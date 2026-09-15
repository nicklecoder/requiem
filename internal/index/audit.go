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
	A string `json:"a"`
	B string `json:"b"`
	// Score is the CSLS value pairs are ranked by: higher is more unusual.
	// Comparable within one result set, not across corpora.
	Score float64 `json:"score"`
	// Similarity is the raw cosine, kept because it is the number people
	// actually have intuitions about.
	Similarity float64 `json:"similarity"`
	ExcerptA   string  `json:"excerpt_a"`
	ExcerptB   string  `json:"excerpt_b"`
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

// localDensityK is how many neighbours define a statement's local
// neighbourhood for CSLS. Small because requiem's corpora are small; the
// value only has to estimate "how close is this statement to things in
// general", and a handful of neighbours does that stably.
const localDensityK = 5

// FindCandidatePairs sweeps searchable statements (optionally scoped to a
// namespace) for pairs worth a human's attention, excluding any pair that
// already has a relationship recorded between them in either direction —
// once an agent has judged a pair, it stops resurfacing.
//
// Candidates are each statement's `neighbors` nearest others, not every pair
// above a similarity threshold. That choice is empirical. An absolute
// threshold fails on a real corpus because every statement in one project
// shares a vocabulary: measured here, cosine >= 0.5 surfaced 69% of all
// pairs, while 0.85 — the usual near-duplicate cutoff in information
// retrieval — found none of five planted duplicates. The usable window is
// narrow, model-specific, and moves with how topically uniform the corpus
// is. Asking each statement for its nearest neighbours instead needs no
// constant, and produces a candidate count that grows with the number of
// statements rather than their square.
//
// Ranking is by CSLS rather than raw cosine: 2*sim(a,b) - r(a) - r(b), where
// r(x) is x's mean similarity to its own nearest neighbours. High-dimensional
// spaces generically produce hubs — points that are near everything — and
// subtracting each side's local density measures how unusually close a pair
// is *for those two statements*, instead of how close it is on an absolute
// scale that means nothing on its own.
//
// minScore stays available as an optional hard floor but defaults to off:
// it is a blunt instrument here and calibrating it per model is the problem
// this design removes.
//
// Corpus sizes here are small enough that an in-memory O(n^2) similarity
// pass is fine; no ANN index is needed.
func (ix *Index) FindCandidatePairs(namespace string, neighbors, limit int, minScore float64) ([]PairCandidate, error) {
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

	// Only statements that actually carry a vector can be compared.
	var embedded []stmt
	for _, s := range stmts {
		if _, ok := embeddings[StatementKey(s.fullID)]; ok {
			embedded = append(embedded, s)
		}
	}
	if len(embedded) < 2 {
		return nil, nil
	}
	if neighbors <= 0 {
		neighbors = 1
	}

	sims := make([][]float64, len(embedded))
	for i := range embedded {
		sims[i] = make([]float64, len(embedded))
	}
	for i := 0; i < len(embedded); i++ {
		for j := i + 1; j < len(embedded); j++ {
			v := CosineSimilarity(embeddings[StatementKey(embedded[i].fullID)].Vector, embeddings[StatementKey(embedded[j].fullID)].Vector)
			sims[i][j], sims[j][i] = v, v
		}
	}

	// r(x): mean similarity to x's own nearest neighbours — the local density
	// CSLS subtracts out.
	density := make([]float64, len(embedded))
	for i := range embedded {
		row := make([]float64, 0, len(embedded)-1)
		for j := range embedded {
			if i != j {
				row = append(row, sims[i][j])
			}
		}
		sort.Sort(sort.Reverse(sort.Float64Slice(row)))
		k := localDensityK
		if k > len(row) {
			k = len(row)
		}
		var sum float64
		for _, v := range row[:k] {
			sum += v
		}
		density[i] = sum / float64(k)
	}

	seen := map[string]bool{}
	var out []PairCandidate
	for i := range embedded {
		// Rank this statement's partners, skipping ones already judged, so
		// adjudicating a pair lets the next candidate surface rather than
		// leaving the statement with nothing.
		type cand struct {
			j   int
			sim float64
		}
		var ranked []cand
		for j := range embedded {
			if i == j || adjudicated[pairKey(embedded[i].fullID, embedded[j].fullID)] {
				continue
			}
			if minScore > 0 && sims[i][j] < minScore {
				continue
			}
			ranked = append(ranked, cand{j, sims[i][j]})
		}
		sort.Slice(ranked, func(a, b int) bool { return ranked[a].sim > ranked[b].sim })
		if len(ranked) > neighbors {
			ranked = ranked[:neighbors]
		}

		for _, c := range ranked {
			key := pairKey(embedded[i].fullID, embedded[c.j].fullID)
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, PairCandidate{
				A:                embedded[i].fullID,
				B:                embedded[c.j].fullID,
				Score:            2*c.sim - density[i] - density[c.j],
				Similarity:       c.sim,
				ExcerptA:         searchExcerpt(embedded[i].body),
				ExcerptB:         searchExcerpt(embedded[c.j].body),
				ModalityConflict: embedded[i].modality.ConflictsWith(embedded[c.j].modality),
			})
		}
	}

	// Opposed pairs first, then by CSLS. A modality conflict is a
	// qualitatively different finding from a near-duplicate: it says the two
	// statements pull in opposite directions, which is the thing audit exists
	// to catch, where similarity alone is only evidence of shared subject.
	sort.Slice(out, func(i, j int) bool {
		if out[i].ModalityConflict != out[j].ModalityConflict {
			return out[i].ModalityConflict
		}
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].A < out[j].A
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
