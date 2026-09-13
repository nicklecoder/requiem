package index

// schema is executed once, in a single transaction, whenever the index is
// opened. It's idempotent (IF NOT EXISTS everywhere) since the index is a
// disposable, rebuildable cache — see SPEC.md: files are canonical, this
// database only exists to make queries fast and can be deleted and rebuilt
// via `reindex` at any time.
var schema = []string{
	`CREATE TABLE IF NOT EXISTS manifest (
		file_path    TEXT PRIMARY KEY,
		mtime        INTEGER NOT NULL,
		size         INTEGER NOT NULL,
		content_hash TEXT
	)`,

	`CREATE TABLE IF NOT EXISTS statements (
		full_id           TEXT PRIMARY KEY,
		id                TEXT NOT NULL,
		namespace         TEXT NOT NULL,
		kind              TEXT NOT NULL,
		modality          TEXT,
		body              TEXT NOT NULL,
		status            TEXT NOT NULL,
		provenance_type   TEXT NOT NULL,
		source_file       TEXT,
		source_line_start INTEGER,
		source_line_end   INTEGER,
		source_hash       TEXT,
		created_at        TEXT NOT NULL,
		file_path         TEXT NOT NULL REFERENCES manifest(file_path)
	)`,
	`CREATE INDEX IF NOT EXISTS idx_statements_namespace ON statements(namespace)`,
	`CREATE INDEX IF NOT EXISTS idx_statements_kind ON statements(kind)`,
	`CREATE INDEX IF NOT EXISTS idx_statements_status ON statements(status)`,

	`CREATE TABLE IF NOT EXISTS statement_tags (
		statement_id TEXT NOT NULL REFERENCES statements(full_id) ON DELETE CASCADE,
		tag          TEXT NOT NULL,
		PRIMARY KEY (statement_id, tag)
	)`,
	`CREATE INDEX IF NOT EXISTS idx_statement_tags_tag ON statement_tags(tag)`,

	`CREATE TABLE IF NOT EXISTS relationships (
		from_id TEXT NOT NULL REFERENCES statements(full_id) ON DELETE CASCADE,
		to_id   TEXT NOT NULL,
		type    TEXT NOT NULL,
		note    TEXT,
		PRIMARY KEY (from_id, to_id, type)
	)`,
	`CREATE INDEX IF NOT EXISTS idx_relationships_to ON relationships(to_id)`,

	// Rejections are a lighter-weight companion to statements (see
	// model.Rejection): body already carries both what-was-proposed and
	// why-it-was-rejected as one piece of prose, matching the on-disk
	// _rejected.md format — no separate reason column.
	`CREATE TABLE IF NOT EXISTS rejections (
		full_id     TEXT PRIMARY KEY,
		namespace   TEXT NOT NULL,
		body        TEXT NOT NULL,
		see_instead TEXT,
		rejected_at TEXT NOT NULL,
		file_path   TEXT NOT NULL REFERENCES manifest(file_path)
	)`,
	`CREATE INDEX IF NOT EXISTS idx_rejections_namespace ON rejections(namespace)`,

	// embeddings is a sidecar cache of agent-supplied vectors, deliberately
	// separate from the disposability guarantee the rest of this schema
	// relies on: unlike every other table here, reindex cannot regenerate a
	// row's content by reparsing the statement file — only a fresh `embed`
	// call can. So no ON DELETE CASCADE from statements(full_id) (same
	// precedent as relationships.to_id going unconstrained): an ordinary
	// edit-and-reinsert of a statement's row during reindex must never wipe
	// its embedding, only reindex's genuine file-removal path does that
	// (see reindex.go). Staleness (body changed since source_hash was
	// captured) is a read-time comparison, computed in the service layer —
	// never written back here, matching the code-derived staleness pattern.
	`CREATE TABLE IF NOT EXISTS embeddings (
		full_id     TEXT PRIMARY KEY,
		model       TEXT NOT NULL,
		dims        INTEGER NOT NULL,
		vector      BLOB NOT NULL,
		source_hash TEXT NOT NULL,
		computed_at TEXT NOT NULL
	)`,

	// embedding_meta pins the whole corpus to a single model/dims: cosine
	// similarity between vectors from two different embedding models is a
	// number that looks plausible but means nothing, so this is a hard
	// guard (see UpsertEmbedding), not just bookkeeping.
	`CREATE TABLE IF NOT EXISTS embedding_meta (
		id    INTEGER PRIMARY KEY CHECK (id = 1),
		model TEXT NOT NULL,
		dims  INTEGER NOT NULL
	)`,

	// Standalone FTS5 tables (not external-content): full_id is TEXT, and
	// external-content FTS5 wants an INTEGER rowid matching content_rowid
	// plus sync triggers for no real benefit here, since Reindex already
	// fully controls every write to these — see SPEC.md. Populated/cleared
	// in reindex.go alongside the relational tables, in the same transaction.
	`CREATE VIRTUAL TABLE IF NOT EXISTS statements_fts USING fts5(full_id UNINDEXED, namespace, body, tags)`,
	`CREATE VIRTUAL TABLE IF NOT EXISTS rejections_fts USING fts5(full_id UNINDEXED, namespace, body)`,

	// code_refs caches the last source scan so `get`/`list`/`check` can show
	// a reference count without touching the working tree. It is a
	// materialized scan result, not a second index: nothing tracks source
	// file mtimes, there is no manifest, and each scan replaces the table
	// wholesale, so it cannot fall out of sync in the way an incremental
	// index could — it can only lag, which is acceptable for a hint.
	//
	// An empty table means no labels were found, which is indistinguishable
	// from never having scanned, and both correctly read as "labelling is
	// not in use here" (see codeRefsAdopted).
	`CREATE TABLE IF NOT EXISTS code_refs (
		full_id TEXT NOT NULL,
		file    TEXT NOT NULL,
		line    INTEGER NOT NULL,
		PRIMARY KEY (full_id, file, line)
	)`,
	`CREATE INDEX IF NOT EXISTS idx_code_refs_full_id ON code_refs(full_id)`,

	// fts5vocab exposes each FTS index's term -> document-frequency table.
	// It is a view over data FTS5 already maintains, not a second index:
	// nothing writes to it, reindex never touches it, and it cannot fall out
	// of sync with the table it reads. Used by buildMatchQuery to drop query
	// terms that occur in more than half the corpus — precisely the terms
	// FTS5's own bm25 scores as carrying no information (see fuse). Declared
	// after the FTS tables because a vocab table names an existing one.
	`CREATE VIRTUAL TABLE IF NOT EXISTS statements_vocab USING fts5vocab(statements_fts, row)`,
	`CREATE VIRTUAL TABLE IF NOT EXISTS rejections_vocab USING fts5vocab(rejections_fts, row)`,
}
