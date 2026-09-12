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
	for _, stmt := range schema {
		if _, err := tx.Exec(stmt); err != nil {
			tx.Rollback()
			return fmt.Errorf("schema: %w", err)
		}
	}
	return tx.Commit()
}
