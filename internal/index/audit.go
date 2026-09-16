package index

import (
	"database/sql"
	"fmt"
	"sort"

	"github.com/nicklecoder/requiem/internal/model"
)

// PairCandidate is a ranked, compact result from FindCandidatePairs — like
// Candidate, excerpts only, never full bodies: the agent classifies the pair
// (conflict, duplicate, or false positive) and records that afterwards,
// deferring full detail to `get` on whichever side warrants it.
type PairCandidate struct {
	A string `json:"a"`
	B string `json:"b"`
	// Score ranks the pair: higher is more worth a look. It is CSLS where
	// both sides are embedded, promoted by shared evidence — see
	// FindCandidatePairs. Comparable within one result set, not across
	// corpora.
	Score float64 `json:"score"`
	// Similarity is the raw cosine, kept because it is the number people
	// actually have intuitions about. Absent when either side has no vector:
	// a pair surfaced by a shared identifier is a real candidate, and
	// reporting 0.0 for it would read as "measured, and unrelated".
	Similarity *float64 `json:"similarity,omitempty"`
	ExcerptA   string   `json:"excerpt_a"`
	ExcerptB   string   `json:"excerpt_b"`
	// SharedFacets are the identifiers both statements name. This is the
	// strongest pairing signal in a real corpus and the reason this pair is
	// ranked where it is.
	// requiem: retrieval/audit-pairs-share-identifiers
	SharedFacets []string `json:"shared_facets,omitempty"`
	// SameSource marks two statements derived from the same source file —
	// two decisions about one piece of code.
	SameSource bool `json:"same_source,omitempty"`
	// ModalityConflict marks a pair whose normative directions oppose. It is
	// a tiebreak, not a ranking: on a real corpus, ranking by modality
	// opposition pushed 48 unrelated pairs into the top 50, because a must
	// set against a must_not on unrelated topics is ordinary.
	// requiem: model/modality-is-not-conflict-detection
	ModalityConflict bool `json:"modality_conflict,omitempty"`
}

// AuditProgress reports how much of the corpus has actually been judged, so a
// caller can tell progress from motion.
//
// Remaining alone was not enough, and measurably so: fifteen verdicts recorded
// against this project's own corpus moved the outstanding count from 93 to 92,
// because the old sweep offered each statement its k nearest *unadjudicated*
// partners and every verdict freed a slot the next-nearest neighbour filled.
// The field report saw the same shape at scale — 415 to 425 after 97 verdicts.
// Swept counts statements whose whole window has been judged, which is a
// number that rises as work is done.
// requiem: retrieval/audit-queue-drains
type AuditProgress struct {
	// Remaining is the unadjudicated candidate pairs in this sweep, including
	// those beyond the returned page.
	Remaining int `json:"remaining"`
	// Swept is how many in-scope statements have no unjudged pair left in
	// their window. A statement with nothing comparable counts as swept: it
	// has nothing outstanding.
	Swept int `json:"swept"`
	// Statements is the in-scope total, the denominator for Swept.
	Statements int `json:"statements"`
	// Depth is the neighbour count this sweep used — the dial that decides
	// how deep a window goes.
	Depth int `json:"depth"`
}

// localDensityK is how many neighbours define a statement's local
// neighbourhood for CSLS. Small because requiem's corpora are small; the
// value only has to estimate "how close is this statement to things in
// general", and a handful of neighbours does that stably.
const localDensityK = 5

// maxRecordsPerFacet bounds which identifiers can pair statements.
//
// An identifier named by most of the corpus is behaving like a common word,
// not a key: `full_id` appears throughout requiem's own corpus, and pairing
// every statement that mentions it would bury the pairs that share something
// specific. This is the same hub problem CSLS corrects for in embedding
// space, handled here by declining to build the pair at all.
const maxRecordsPerFacet = 8

// facetPairBoost and sourcePairBoost lift pairs that share hard evidence
// above pairs that merely sit near each other in embedding space.
//
// They are large relative to CSLS (which lands in roughly [-0.3, 0.7] on a
// real corpus) because the ordering they produce is not a matter of degree:
// the three genuine conflicts a real 255-statement corpus never surfaced
// shared identifiers while sharing almost no prose, and no amount of
// similarity-based ranking would have found them. Similarity still orders
// pairs within each band.
const (
	facetPairBoost  = 10.0
	sourcePairBoost = 5.0
)

// FindCandidatePairs sweeps statements (optionally scoped to a namespace) for
// pairs worth a human's attention, excluding any pair already adjudicated —
// by a recorded relationship in either direction, or by a dismissal in the
// verdict store.
//
// Candidates come from three sources, in decreasing order of how much the
// pairing itself asserts:
//
//   - Statements naming the same identifier. An identifier is an exact key,
//     so this is evidence about subject matter rather than a guess from
//     wording. It is also what embeddings structurally cannot find: measured
//     on a real 255-statement corpus, nearest-neighbour search over 1,936
//     candidate pairs missed three genuine conflicts, and all three shared an
//     identifier while sharing almost no prose.
//   - Statements derived from the same source file — two decisions about one
//     piece of code.
//   - Each statement's `neighbors` nearest others by embedding. That choice
//     is empirical: an absolute similarity threshold fails on a real corpus
//     because every statement in one project shares a vocabulary. Measured
//     here, cosine >= 0.5 surfaced 69% of all pairs while 0.85 — the usual
//     near-duplicate cutoff in information retrieval — found none of five
//     planted duplicates.
//
// Ranking is by CSLS, 2*sim(a,b) - r(a) - r(b), where r(x) is x's mean
// similarity to its own nearest neighbours, promoted by the boosts above.
// High-dimensional spaces generically produce hubs — points near everything —
// and subtracting each side's local density measures how unusually close a
// pair is for those two statements rather than on an absolute scale that
// means nothing alone.
//
// Modality opposition is a tiebreak only. Ranking by it put 48 unrelated
// pairs in the top 50 of a real audit, because a must against a must_not on
// different subjects is common and says nothing.
//
// **Each statement's window is fixed before adjudication is considered, not
// after.** This is what makes the queue finite. The sweep used to rank a
// statement's *unadjudicated* partners and take the top k, so judging one pair
// promoted partner k+1 into the window and the outstanding count never fell:
// fifteen verdicts against this corpus moved it from 93 to 92. Selecting the
// top k first and then dropping the judged ones means a statement stops
// contributing once its window is clear, so verdicts strictly drain the queue
// and `Swept` rises.
//
// The cost is deliberate and worth stating: a pair at depth k+1 is no longer
// offered just because a nearer pair was judged. Depth is a dial instead —
// sweep at `--neighbors 2`, then again at 5 to go deeper — which is a decision
// the reader makes rather than a dribble the tool decides. That ordering is
// also the one model-independent choice available: a similarity floor would
// have to name an absolute cosine, and what counts as close is a property of
// the embedding model, not of the corpus.
// requiem: retrieval/audit-queue-drains
func (ix *Index) FindCandidatePairs(namespace string, neighbors, limit int, minScore float64) ([]PairCandidate, AuditProgress, error) {
	if neighbors <= 0 {
		neighbors = 1
	}
	progress := AuditProgress{Depth: neighbors}

	stmts, err := ix.auditStatements(namespace)
	if err != nil {
		return nil, progress, err
	}
	progress.Statements = len(stmts)
	if len(stmts) < 2 {
		progress.Swept = len(stmts)
		return nil, progress, nil
	}

	embeddings, err := ix.AllEmbeddings()
	if err != nil {
		return nil, progress, err
	}
	facets, err := ix.AllFacets()
	if err != nil {
		return nil, progress, err
	}
	adjudicated, err := ix.AdjudicatedPairs()
	if err != nil {
		return nil, progress, err
	}

	embedded := make([]bool, len(stmts))
	var embeddedCount int
	for i, s := range stmts {
		if _, ok := embeddings[StatementKey(s.fullID)]; ok {
			embedded[i] = true
			embeddedCount++
		}
	}
	// Facet pairing needs no vectors, so an unembedded corpus is no longer
	// nothing to sweep — but a corpus with neither vectors nor identifiers
	// genuinely cannot be swept, and saying so is better than returning an
	// empty list that reads as "nothing to worry about".
	if embeddedCount == 0 && len(facets) == 0 {
		return nil, progress, fmt.Errorf("nothing to compare: no statement is embedded and no identifiers were found in any body (see `requiem list --needs-embedding`)")
	}

	sims, density := ix.similarityAndDensity(stmts, embedded, embeddings)

	type pairInfo struct {
		i, j         int
		sharedFacets []string
		sameSource   bool
	}
	pairs := map[string]*pairInfo{}

	// window records which pairs each statement is responsible for, judged or
	// not. It is what "swept" is measured against, so it has to include the
	// pairs pairAt drops: a statement whose whole window has been judged is
	// finished, and that is only knowable if the judged pairs were counted as
	// part of the window in the first place.
	// requiem: retrieval/audit-queue-drains
	window := make([]map[string]bool, len(stmts))
	note := func(i, j int) {
		key := pairKey(stmts[i].fullID, stmts[j].fullID)
		for _, side := range [2]int{i, j} {
			if window[side] == nil {
				window[side] = map[string]bool{}
			}
			window[side][key] = true
		}
	}

	pairAt := func(i, j int) *pairInfo {
		if i > j {
			i, j = j, i
		}
		note(i, j)
		key := pairKey(stmts[i].fullID, stmts[j].fullID)
		if adjudicated[key] {
			return nil
		}
		p, ok := pairs[key]
		if !ok {
			p = &pairInfo{i: i, j: j}
			pairs[key] = p
		}
		return p
	}

	// Identifier pairs.
	byFacet := map[string][]int{}
	for i, s := range stmts {
		for _, f := range facets[StatementKey(s.fullID)] {
			byFacet[f] = append(byFacet[f], i)
		}
	}
	for facet, members := range byFacet {
		if len(members) < 2 || len(members) > maxRecordsPerFacet {
			continue
		}
		for a := 0; a < len(members); a++ {
			for b := a + 1; b < len(members); b++ {
				if p := pairAt(members[a], members[b]); p != nil {
					p.sharedFacets = append(p.sharedFacets, facet)
				}
			}
		}
	}

	// Same-source pairs.
	bySource := map[string][]int{}
	for i, s := range stmts {
		if s.sourceFile != "" {
			bySource[s.sourceFile] = append(bySource[s.sourceFile], i)
		}
	}
	for _, members := range bySource {
		for a := 0; a < len(members); a++ {
			for b := a + 1; b < len(members); b++ {
				if p := pairAt(members[a], members[b]); p != nil {
					p.sameSource = true
				}
			}
		}
	}

	// Nearest-neighbour pairs, among the embedded statements only.
	//
	// Adjudication is deliberately *not* consulted here. Ranking only the
	// unjudged partners made the window slide forward with every verdict, so
	// the queue refilled as fast as it was worked. The top k are chosen on
	// similarity alone; pairAt drops the ones already judged afterwards.
	// requiem: retrieval/audit-queue-drains
	for i := range stmts {
		if !embedded[i] {
			continue
		}
		type cand struct {
			j   int
			sim float64
		}
		var ranked []cand
		for j := range stmts {
			if i == j || !embedded[j] {
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
			pairAt(i, c.j)
		}
	}

	out := make([]PairCandidate, 0, len(pairs))
	for _, p := range pairs {
		a, b := stmts[p.i], stmts[p.j]
		score := 0.0
		var similarity *float64
		if embedded[p.i] && embedded[p.j] {
			sim := sims[p.i][p.j]
			similarity = &sim
			score = 2*sim - density[p.i] - density[p.j]
		}
		sort.Strings(p.sharedFacets)
		if n := len(p.sharedFacets); n > 0 {
			score += facetPairBoost * float64(n)
		}
		if p.sameSource {
			score += sourcePairBoost
		}
		out = append(out, PairCandidate{
			A:                a.fullID,
			B:                b.fullID,
			Score:            score,
			Similarity:       similarity,
			ExcerptA:         searchExcerpt(a.body),
			ExcerptB:         searchExcerpt(b.body),
			SharedFacets:     p.sharedFacets,
			SameSource:       p.sameSource,
			ModalityConflict: a.modality.ConflictsWith(b.modality),
		})
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		// Modality only breaks a tie — see the type's own comment for the
		// measurement that demoted it.
		if out[i].ModalityConflict != out[j].ModalityConflict {
			return out[i].ModalityConflict
		}
		if out[i].A != out[j].A {
			return out[i].A < out[j].A
		}
		return out[i].B < out[j].B
	})

	remaining := len(out)
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	progress.Remaining = remaining
	// A statement with nothing left unjudged in its window is swept, and one
	// with an empty window is swept too: it has nothing outstanding, and
	// reporting it as unfinished would make the denominator unreachable.
	for i := range stmts {
		done := true
		for key := range window[i] {
			if !adjudicated[key] {
				done = false
				break
			}
		}
		if done {
			progress.Swept++
		}
	}
	return out, progress, nil
}

// auditStatement is one row the sweep compares.
type auditStatement struct {
	fullID     string
	body       string
	sourceFile string
	modality   model.Modality
}

func (ix *Index) auditStatements(namespace string) ([]auditStatement, error) {
	query := `SELECT full_id, body, modality, source_file FROM statements WHERE ` + searchableStatuses
	args := []interface{}{}
	if namespace != "" {
		query += ` AND (namespace = ? OR namespace LIKE ?)`
		args = append(args, namespace, namespace+"/%")
	}
	query += ` ORDER BY full_id`

	rows, err := ix.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []auditStatement
	for rows.Next() {
		var s auditStatement
		var modality, sourceFile sql.NullString
		if err := rows.Scan(&s.fullID, &s.body, &modality, &sourceFile); err != nil {
			return nil, err
		}
		s.modality = model.Modality(modality.String)
		s.sourceFile = sourceFile.String
		out = append(out, s)
	}
	return out, rows.Err()
}

// similarityAndDensity builds the pairwise cosine matrix and each embedded
// statement's local density. Corpus sizes here are small enough that an
// in-memory O(n^2) pass is fine; no ANN index is warranted.
func (ix *Index) similarityAndDensity(stmts []auditStatement, embedded []bool, embeddings map[EmbKey]Embedding) ([][]float64, []float64) {
	sims := make([][]float64, len(stmts))
	for i := range sims {
		sims[i] = make([]float64, len(stmts))
	}
	for i := 0; i < len(stmts); i++ {
		if !embedded[i] {
			continue
		}
		for j := i + 1; j < len(stmts); j++ {
			if !embedded[j] {
				continue
			}
			v := CosineSimilarity(
				embeddings[StatementKey(stmts[i].fullID)].Vector,
				embeddings[StatementKey(stmts[j].fullID)].Vector)
			sims[i][j], sims[j][i] = v, v
		}
	}

	density := make([]float64, len(stmts))
	for i := range stmts {
		if !embedded[i] {
			continue
		}
		row := make([]float64, 0, len(stmts))
		for j := range stmts {
			if i != j && embedded[j] {
				row = append(row, sims[i][j])
			}
		}
		if len(row) == 0 {
			continue
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
	return sims, density
}

// AdjudicatedPairs loads every pair an agent has already judged: any recorded
// relationship, in either direction and of any type, plus every dismissal in
// the verdict store. Both mean the same thing to a sweep — somebody has
// looked at this pair — even though only one of them says anything about the
// decisions themselves.
func (ix *Index) AdjudicatedPairs() (map[string]bool, error) {
	out := map[string]bool{}
	for _, q := range []string{
		`SELECT from_id, to_id FROM relationships`,
		`SELECT a, b FROM verdicts`,
	} {
		rows, err := ix.db.Query(q)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var a, b string
			if err := rows.Scan(&a, &b); err != nil {
				rows.Close()
				return nil, err
			}
			out[pairKey(a, b)] = true
		}
		if err := rows.Close(); err != nil {
			return nil, err
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func pairKey(a, b string) string {
	if a > b {
		a, b = b, a
	}
	return a + "\x00" + b
}
