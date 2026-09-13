package index

import (
	"fmt"
	"sort"
)

// PairCandidate is a ranked, compact result from FindCandidatePairs — like
// Candidate, excerpts only, never full bodies: the agent classifies the
// pair (conflict, duplicate, or false positive) and records that via a
// follow-up `link`, deferring full detail to `get` on whichever side
// warrants it.
type PairCandidate struct {
	A        string  `json:"a"`
	B        string  `json:"b"`
	Score    float64 `json:"score"`
	ExcerptA string  `json:"excerpt_a"`
	ExcerptB string  `json:"excerpt_b"`
}

// FindCandidatePairs sweeps active statements (optionally scoped to a
// namespace, matching itself and anything nested under it) for pairs whose
// embeddings are close in cosine-similarity terms, excluding any pair that
// already has a relationship recorded between them in either direction —
// once an agent has judged a pair (conflicts_with, duplicates, not_related,
// ...), it stops resurfacing. Corpus sizes here are small enough that an
// in-memory O(n^2) scan is fine; no ANN index is needed.
func (ix *Index) FindCandidatePairs(namespace string, minScore float64, limit int) ([]PairCandidate, error) {
	query := `SELECT full_id, body FROM statements WHERE status = 'active'`
	args := []interface{}{}
	if namespace != "" {
		query += ` AND (namespace = ? OR namespace LIKE ?)`
		args = append(args, namespace, namespace+"/%")
	}

	rows, err := ix.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	type stmt struct{ fullID, body string }
	var stmts []stmt
	for rows.Next() {
		var s stmt
		if err := rows.Scan(&s.fullID, &s.body); err != nil {
			rows.Close()
			return nil, err
		}
		stmts = append(stmts, s)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// An unembedded corpus would otherwise return an empty list — byte for
	// byte what "swept everything, found no candidates" looks like. Audit
	// is only meaningful over embeddings, so say so instead of handing back
	// a false all-clear.
	corpus, err := ix.EmbeddingCorpusInfo()
	if err != nil {
		return nil, err
	}
	if corpus.Count == 0 {
		return nil, fmt.Errorf("no statements are embedded yet: audit compares statements by embedding, so it has nothing to sweep (see `requiem list --needs-embedding`)")
	}

	embeddings, err := ix.AllEmbeddings()
	if err != nil {
		return nil, err
	}

	adjudicated, err := ix.allRelationshipPairs()
	if err != nil {
		return nil, err
	}

	var out []PairCandidate
	for i := 0; i < len(stmts); i++ {
		ei, ok := embeddings[stmts[i].fullID]
		if !ok {
			continue
		}
		for j := i + 1; j < len(stmts); j++ {
			ej, ok := embeddings[stmts[j].fullID]
			if !ok {
				continue
			}
			if adjudicated[pairKey(stmts[i].fullID, stmts[j].fullID)] {
				continue
			}
			score := CosineSimilarity(ei.Vector, ej.Vector)
			if score < minScore {
				continue
			}
			out = append(out, PairCandidate{
				A:        stmts[i].fullID,
				B:        stmts[j].fullID,
				Score:    score,
				ExcerptA: searchExcerpt(stmts[i].body),
				ExcerptB: searchExcerpt(stmts[j].body),
			})
		}
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// allRelationshipPairs loads every relationship as an unordered pair key,
// regardless of which side is from_id/to_id or which relationship type —
// any recorded relationship between two statements counts as "already
// adjudicated" for audit purposes.
func (ix *Index) allRelationshipPairs() (map[string]bool, error) {
	rows, err := ix.db.Query(`SELECT from_id, to_id FROM relationships`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]bool{}
	for rows.Next() {
		var a, b string
		if err := rows.Scan(&a, &b); err != nil {
			return nil, err
		}
		out[pairKey(a, b)] = true
	}
	return out, rows.Err()
}

func pairKey(a, b string) string {
	if a > b {
		a, b = b, a
	}
	return a + "\x00" + b
}
