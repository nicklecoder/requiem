// Package index is the SQLite layer: a disposable, rebuildable cache over
// the canonical statement files (internal/store). It exists purely to make
// queries fast (filtering now, FTS5 full-text search from M4) — if the
// database file is deleted, Reindex rebuilds it from files with no data
// loss, since files are the real source of truth.
package index

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// ErrNotFound is returned when a requested statement isn't in the index.
var ErrNotFound = errors.New("not found")

// timeFormat is used for every timestamp column — RFC3339Nano round-trips
// through time.Parse without losing the sub-second precision Go's
// time.Now() produces.
const timeFormat = time.RFC3339Nano

// Index wraps the SQLite connection backing one project's .requiem/index.sqlite.
type Index struct {
	db *sql.DB
}

// Open opens (creating if necessary) the SQLite file at path and ensures
// its schema exists.
func Open(path string) (*Index, error) {
	// Pragmas set via the DSN (rather than a post-open Exec) are applied by
	// the driver to every connection it opens internally, not just the one
	// the first Exec happens to land on — that distinction mattered in
	// practice: a post-open `PRAGMA busy_timeout` still produced spurious
	// SQLITE_BUSY errors under real concurrent-process load (see the
	// internal/requiem concurrency stress test), because database/sql can
	// open a connection the pragma-Exec never reached despite
	// SetMaxOpenConns(1). _busy_timeout=15000 (not SQLite's small default):
	// several `requiem` invocations reindexing the same project at once
	// (e.g. multiple agents) can queue a full incremental-reindex
	// transaction behind others longer than a short timeout comfortably covers.
	dsn := "file:" + path + "?_busy_timeout=15000&_foreign_keys=1"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// A single connection also matches reality: one CLI invocation does one
	// operation and exits, so there's no concurrency within it to pool for.
	db.SetMaxOpenConns(1)
	ix := &Index{db: db}
	if err := ix.ensureSchema(); err != nil {
		db.Close()
		return nil, err
	}
	return ix, nil
}

func (ix *Index) Close() error {
	return ix.db.Close()
}

func (ix *Index) ensureSchema() error {
	tx, err := ix.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, stmt := range schema {
		if _, err := tx.Exec(stmt); err != nil {
			return fmt.Errorf("schema: %w", err)
		}
	}
	if err := migrate(tx); err != nil {
		return fmt.Errorf("migrate index: %w", err)
	}
	if err := ensureDerivation(tx); err != nil {
		return fmt.Errorf("refresh derived data: %w", err)
	}
	return tx.Commit()
}

// derivationVersion identifies the shape of the data reindex *derives* from
// statement files — the facet index today, anything of the same character
// later. Bump it whenever that shape changes.
//
// Without it, adding derived data is invisible on an existing index and stays
// that way: reindex is incremental, so every unmodified file reports
// unchanged, the code that would populate the new table never runs, and the
// feature silently does nothing until someone happens to edit each file. That
// is exactly what happened when facets were added — the whole corpus indexed
// clean and `check --touches` matched nothing.
const derivationVersion = "2"

// requiem: model/derived-data-is-versioned
// ensureDerivation rebuilds everything reindex derives from files when the
// derivation shape has changed, by clearing the manifest and the derived
// tables so the next reindex reparses the corpus.
//
// Embeddings and code_refs are deliberately left alone. Vectors cannot be
// recovered by reparsing a file — only by a network call — which is the same
// reason schema changes here migrate rather than wipe.
func ensureDerivation(tx *sql.Tx) error {
	var stored string
	err := tx.QueryRow(`SELECT value FROM index_meta WHERE key = 'derivation_version'`).Scan(&stored)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if stored == derivationVersion {
		return nil
	}

	// Dependents first, manifest last: statements.file_path and
	// rejections.file_path are foreign keys into it, so clearing the
	// manifest while its rows are still referenced fails outright.
	for _, stmt := range []string{
		`DELETE FROM statements_fts`,
		`DELETE FROM statement_tags`,
		`DELETE FROM relationships`,
		`DELETE FROM statements`,
		`DELETE FROM rejections_fts`,
		`DELETE FROM rejections`,
		`DELETE FROM facets`,
		`DELETE FROM manifest`,
	} {
		if _, err := tx.Exec(stmt); err != nil {
			return fmt.Errorf("%s: %w", stmt, err)
		}
	}
	_, err = tx.Exec(
		`INSERT INTO index_meta (key, value) VALUES ('derivation_version', ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`, derivationVersion)
	return err
}

// migrate brings an index created by an older build up to the current shape.
//
// CREATE TABLE IF NOT EXISTS silently does nothing when a table already
// exists, so adding a column to the schema leaves every existing index
// broken — queries against the new column fail until the file is deleted.
//
// Deleting it is not an acceptable answer. Every other table can be rebuilt
// by reparsing statement files, but embeddings cannot: recovering those needs
// network access and an endpoint that may not be reachable. So schema changes
// are migrated rather than resolved by wiping.
//
// Additive only, and each step checks for its own column rather than relying
// on a version counter — that way a fresh index built from the full schema
// and an old one being upgraded both end up in the same state, with no
// bookkeeping to get out of step.
func migrate(tx *sql.Tx) error {
	migrations := []struct {
		table, column, ddl string
	}{
		// code_refs.kind distinguishes a reference in code from a mention in
		// documentation (see internal/trace.Kind).
		{"code_refs", "kind", `ALTER TABLE code_refs ADD COLUMN kind TEXT NOT NULL DEFAULT 'code'`},
		// statements.abstract marks a statement no code can implement.
		{"statements", "abstract", `ALTER TABLE statements ADD COLUMN abstract INTEGER NOT NULL DEFAULT 0`},
	}
	for _, m := range migrations {
		has, err := hasColumn(tx, m.table, m.column)
		if err != nil {
			return err
		}
		if has {
			continue
		}
		if _, err := tx.Exec(m.ddl); err != nil {
			return fmt.Errorf("add %s.%s: %w", m.table, m.column, err)
		}
	}
	return migrateRebuilds(tx)
}

// migrateRebuilds handles the changes ALTER TABLE cannot express — a new
// primary key, in practice. Each step is still guarded by a missing column,
// so it is skipped on a fresh index built from the full schema, and each
// copies the old rows forward: the whole reason to migrate rather than wipe
// is that embeddings cannot be rebuilt from files.
func migrateRebuilds(tx *sql.Tx) error {
	rebuilds := []struct {
		table, column string
		steps         []string
	}{
		// embeddings gains source_kind and is re-keyed on
		// (source_kind, full_id) so rejections can carry vectors too.
		// Existing rows are all statements, by construction: nothing else
		// could be embedded before.
		{"embeddings", "source_kind", []string{
			`CREATE TABLE embeddings_migrated (
				source_kind TEXT NOT NULL DEFAULT 'statement',
				full_id     TEXT NOT NULL,
				model       TEXT NOT NULL,
				dims        INTEGER NOT NULL,
				vector      BLOB NOT NULL,
				source_hash TEXT NOT NULL,
				computed_at TEXT NOT NULL,
				PRIMARY KEY (source_kind, full_id)
			)`,
			`INSERT INTO embeddings_migrated
				(source_kind, full_id, model, dims, vector, source_hash, computed_at)
			 SELECT 'statement', full_id, model, dims, vector, source_hash, computed_at
			 FROM embeddings`,
			`DROP TABLE embeddings`,
			`ALTER TABLE embeddings_migrated RENAME TO embeddings`,
		}},
	}
	for _, r := range rebuilds {
		has, err := hasColumn(tx, r.table, r.column)
		if err != nil {
			return err
		}
		if has {
			continue
		}
		for _, step := range r.steps {
			if _, err := tx.Exec(step); err != nil {
				return fmt.Errorf("rebuild %s for %s: %w", r.table, r.column, err)
			}
		}
	}
	return nil
}

func hasColumn(tx *sql.Tx, table, column string) (bool, error) {
	rows, err := tx.Query(`SELECT name FROM pragma_table_info(?)`, table)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
}
