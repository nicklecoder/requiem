package index

import (
	"database/sql"
	"errors"
	"time"
)

// requiem: embedding/background-embedding
// TryStartBackgroundEmbed takes the background-embedding lock, reporting
// whether it did: false while another run holds it, unless that run started
// more than staleAfter ago and so presumably died.
func (ix *Index) TryStartBackgroundEmbed(now time.Time, staleAfter time.Duration) (bool, error) {
	tx, err := ix.db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var started string
	err = tx.QueryRow(`SELECT started_at FROM background_embed WHERE id = 1`).Scan(&started)
	switch {
	case errors.Is(err, sql.ErrNoRows):
	case err != nil:
		return false, err
	default:
		if t, perr := time.Parse(timeFormat, started); perr == nil && now.Sub(t) < staleAfter {
			return false, nil
		}
	}
	if _, err := tx.Exec(`INSERT INTO background_embed (id, started_at) VALUES (1, ?)
		ON CONFLICT(id) DO UPDATE SET started_at = excluded.started_at`, now.UTC().Format(timeFormat)); err != nil {
		return false, err
	}
	return true, tx.Commit()
}

// FinishBackgroundEmbed releases the lock.
func (ix *Index) FinishBackgroundEmbed() error {
	_, err := ix.db.Exec(`DELETE FROM background_embed WHERE id = 1`)
	return err
}
