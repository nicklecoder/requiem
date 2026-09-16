package index

// schema is executed once, in a single transaction, whenever the index is
// opened. It's idempotent (IF NOT EXISTS everywhere) since the index is a
// disposable, rebuildable cache — see SPEC.md: files are canonical, this
// database only exists to make queries fast and can be deleted and rebuilt
// via `reindex` at any time.
var schema = []string{
	// index_meta records facts about the index itself, notably which shape
	// of derived data it was built with — see derivationVersion.
	`CREATE TABLE IF NOT EXISTS index_meta (
		key   TEXT PRIMARY KEY,
		value TEXT NOT NULL
	)`,

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
		abstract          INTEGER NOT NULL DEFAULT 0,
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
	//
	// Keyed by (source_kind, full_id), not full_id alone: rejections are
	// embedded too, and a statement and a rejection may legitimately share
	// a full_id (see fuse in search.go), so one key would fuse two
	// different records' vectors into one row.
	// requiem: retrieval/rejections-embedded
	`CREATE TABLE IF NOT EXISTS embeddings (
		source_kind TEXT NOT NULL DEFAULT 'statement',
		full_id     TEXT NOT NULL,
		model       TEXT NOT NULL,
		dims        INTEGER NOT NULL,
		vector      BLOB NOT NULL,
		source_hash TEXT NOT NULL,
		computed_at TEXT NOT NULL,
		PRIMARY KEY (source_kind, full_id)
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
		kind    TEXT NOT NULL DEFAULT 'code',
		PRIMARY KEY (full_id, file, line)
	)`,
	`CREATE INDEX IF NOT EXISTS idx_code_refs_full_id ON code_refs(full_id)`,

	// verdicts holds audit dismissals — pairs an agent has judged unrelated.
	//
	// Kept out of the relationships table, and out of the statement files,
	// because a dismissal is not a semantic relationship: one real corpus
	// accumulated 75 not_related edges, permanent noise in a graph people
	// read to understand how decisions fit together. Derived from files under
	// .requiem/verdicts/ exactly as every other table here is derived from
	// files, so nothing about it is less recoverable.
	// requiem: model/verdicts-are-not-edges
	`CREATE TABLE IF NOT EXISTS verdicts (
		a          TEXT NOT NULL,
		b          TEXT NOT NULL,
		verdict    TEXT NOT NULL,
		note       TEXT,
		decided_at TEXT NOT NULL,
		file_path  TEXT NOT NULL REFERENCES manifest(file_path),
		PRIMARY KEY (a, b)
	)`,

	// facets holds the concrete identifiers a record names — snake_case
	// names, dotted paths, symbols, camelCase — extracted from its body.
	//
	// This is not a second index in the sense SPEC's Boundary section
	// forbids: it is derived from the statement files this index already
	// parses, rebuilt in the same transaction as the row it belongs to, and
	// it never looks at the source tree. Nothing about it can fall out of
	// sync that the surrounding row could not.
	//
	// It exists because prose similarity cannot pair two records that share
	// an identifier and nothing else. Measured on a real 255-statement
	// corpus: three genuine conflicts were never surfaced by nearest-
	// neighbour search over 1,936 candidate pairs, and every one of them
	// shared an identifier while sharing almost no wording.
	// requiem: retrieval/identifier-facets
	`CREATE TABLE IF NOT EXISTS facets (
		source_kind TEXT NOT NULL,
		full_id     TEXT NOT NULL,
		facet       TEXT NOT NULL,
		PRIMARY KEY (source_kind, full_id, facet)
	)`,
	`CREATE INDEX IF NOT EXISTS idx_facets_facet ON facets(facet)`,

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
