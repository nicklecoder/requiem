package index

import (
	"database/sql"
	"errors"
	"fmt"
)

// ScoreKey identifies what a classifier scored: the hashes of the bodies it
// read. B is empty for a question about one body.
type ScoreKey struct{ A, B string }

// PairScoreKey keys a pair of bodies in either order, since a pair's
// contradiction score is read both ways and stored once.
func PairScoreKey(a, b string) ScoreKey {
	if b < a {
		a, b = b, a
	}
	return ScoreKey{a, b}
}

// ClassifierScores returns the cached scores identity gave for keys, omitting
// any it has not scored.
func (ix *Index) ClassifierScores(identity string, keys []ScoreKey) (map[ScoreKey]float64, error) {
	out := map[ScoreKey]float64{}
	stmt, err := ix.db.Prepare(`SELECT score FROM classifier_scores WHERE identity = ? AND a_hash = ? AND b_hash = ?`)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()
	for _, k := range keys {
		var s float64
		switch err := stmt.QueryRow(identity, k.A, k.B).Scan(&s); {
		case err == nil:
			out[k] = s
		case errors.Is(err, sql.ErrNoRows):
		default:
			return nil, fmt.Errorf("read classifier score: %w", err)
		}
	}
	return out, nil
}

// StoreClassifierScores caches scores identity gave.
func (ix *Index) StoreClassifierScores(identity string, scores map[ScoreKey]float64) error {
	tx, err := ix.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for k, s := range scores {
		if _, err := tx.Exec(`INSERT INTO classifier_scores (identity, a_hash, b_hash, score) VALUES (?, ?, ?, ?)
			ON CONFLICT(identity, a_hash, b_hash) DO UPDATE SET score = excluded.score`, identity, k.A, k.B, s); err != nil {
			return fmt.Errorf("cache classifier score: %w", err)
		}
	}
	return tx.Commit()
}
