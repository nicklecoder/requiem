package index

import (
	"database/sql"
	"fmt"
	"sort"

	"github.com/nicklecoder/requiem/internal/model"
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
	// ModalityConflict marks a pair whose normative directions oppose — an
	// obligation or permission against a prohibition. Set only when both
	// statements declare a modality, since an absent one asserts nothing and
	// can contradict nothing.
	//
	// It is a decidable signal, not a verdict, and it is narrow: contraries
	// defeat it entirely ("must be red" and "must be blue" are both `must`).
	// Its value comes from being combined with similarity — the score filter
	// runs first, so an opposed pair only ever surfaces when the two
	// statements are already close enough to plausibly concern the same
	// subject.
	ModalityConflict bool `json:"modality_conflict,omitempty"`
}

// FindCandidatePairs sweeps active statements (optionally scoped to a
// namespace, matching itself and anything nested under it) for pairs whose
// embeddings are close in cosine-similarity terms, excluding any pair that
// already has a relationship recorded between them in either direction —
// once an agent has judged a pair (conflicts_with, duplicates, not_related,
// ...), it stops resurfacing. Corpus sizes here are small enough that an
// in-memory O(n^2) scan is fine; no ANN index is needed.
func (ix *Index) FindCandidatePairs(namespace string, minScore float64, limit int) ([]PairCandidate, error) {
	query := `SELECT full_id, body, modality FROM statements WHERE ` + searchableStatuses
	args := []interface{}{}
	if namespace != "" {
		query += ` AND (namespace = ? OR namespace LIKE ?)`
		args = append(args, namespace, namespace+"/%")
	}

	rows, err := ix.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	type stmt struct {
		fullID, body string
		modality     model.Modality
	}
	var stmts []stmt
	for rows.Next() {
		var s stmt
		var modality sql.NullString
		if err := rows.Scan(&s.fullID, &s.body, &modality); err != nil {
			rows.Close()
			return nil, err
		}
		if modality.Valid {
			s.modality = model.Modality(modality.String)
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
				A:                stmts[i].fullID,
				B:                stmts[j].fullID,
				Score:            score,
				ExcerptA:         searchExcerpt(stmts[i].body),
				ExcerptB:         searchExcerpt(stmts[j].body),
				ModalityConflict: stmts[i].modality.ConflictsWith(stmts[j].modality),
			})
		}
	}

	// Opposed pairs first, then by similarity. A modality conflict is a
	// qualitatively different finding from a near-duplicate: it says the two
	// statements pull in opposite directions, which is the thing audit exists
	// to catch, where similarity alone is only evidence of shared subject.
	// Ordering rather than boosting keeps the score honest — it still means
	// cosine similarity and nothing else. Note the min-score filter has
	// already run, so this can only reorder pairs that were close enough to
	// surface anyway; it never promotes unrelated statements.
	sort.Slice(out, func(i, j int) bool {
		if out[i].ModalityConflict != out[j].ModalityConflict {
			return out[i].ModalityConflict
		}
		return out[i].Score > out[j].Score
	})
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
