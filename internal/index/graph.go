package index

import (
	"database/sql"
	"errors"
	"sort"
)

// MatchGraph marks a candidate reached by following a recorded edge from a
// direct hit, rather than by matching the draft at all.
const MatchGraph = "graph"

// graphSeeds is how many of the top direct hits are expanded from, and
// maxGraphNeighbours caps what expansion may add. Both are small on purpose:
// check exists to keep an agent's context small, and a neighbour of the
// fifth-ranked hit is already far from the draft.
const (
	graphSeeds         = 3
	maxGraphNeighbours = 5
)

// graphTypePriority orders neighbours: the edges most likely to lead to a
// decision the draft contradicts come first.
var graphTypePriority = map[string]int{
	"conflicts_with": 0,
	"supersedes":     1,
	"duplicates":     2,
	"see_instead":    3,
	"refines":        4,
	"depends_on":     5,
}

// GraphVia names the recorded edge a graph candidate was reached through,
// in the direction it was written: from and to are the two ends, and one of
// them is the direct hit it was expanded from. A rejection reached through
// its see_instead reports type "see_instead".
type GraphVia struct {
	From string `json:"from"`
	To   string `json:"to"`
	Type string `json:"type"`
}

// ExpandGraph returns the records one recorded edge away from the top direct
// hits, for appending after them. Each carries match_kind "graph", the edge
// it was reached through, and rank 0 — after every fused rank, which is
// always negative — with its verdict computed from its own evidence against
// the draft.
//
// Every other retriever measures resemblance to the draft's wording, and a
// rule the draft contradicts is usually worded from the other side, so it
// can miss both the lexical and the semantic list while sitting one edge
// from something that made it into them. The graph is the one path that
// does not depend on wording at all.
//
// Kept out of Check itself: add's duplicate gate runs Check, and a record
// reached only through an edge is not evidence that a draft is a duplicate.
// requiem: retrieval/graph-expansion
func (ix *Index) ExpandGraph(direct []Candidate, namespace, text string, touches []string) ([]Candidate, error) {
	if len(direct) == 0 {
		return nil, nil
	}
	seen := make(map[EmbKey]bool, len(direct))
	for _, c := range direct {
		seen[EmbKey{c.SourceKind, c.FullID}] = true
	}
	seeds := direct
	if len(seeds) > graphSeeds {
		seeds = seeds[:graphSeeds]
	}

	type edge struct {
		key  EmbKey
		via  GraphVia
		seed int
	}
	var edges []edge
	for i, seed := range seeds {
		if seed.SourceKind == SourceKindRejection {
			// A rejection's only edge is its see_instead: the decision
			// taken in its place.
			var to string
			err := ix.db.QueryRow(`SELECT COALESCE(see_instead, '') FROM rejections WHERE full_id = ?`, seed.FullID).Scan(&to)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return nil, err
			}
			if to != "" {
				edges = append(edges, edge{EmbKey{SourceKindStatement, to}, GraphVia{seed.FullID, to, "see_instead"}, i})
			}
			continue
		}
		rows, err := ix.db.Query(`
			SELECT from_id, to_id, type FROM relationships WHERE from_id = ? OR to_id = ?`,
			seed.FullID, seed.FullID)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var v GraphVia
			if err := rows.Scan(&v.From, &v.To, &v.Type); err != nil {
				rows.Close()
				return nil, err
			}
			other := v.To
			if other == seed.FullID {
				other = v.From
			}
			edges = append(edges, edge{EmbKey{SourceKindStatement, other}, v, i})
		}
		if err := rows.Close(); err != nil {
			return nil, err
		}
		rej, err := ix.db.Query(`SELECT full_id FROM rejections WHERE see_instead = ?`, seed.FullID)
		if err != nil {
			return nil, err
		}
		for rej.Next() {
			var id string
			if err := rej.Scan(&id); err != nil {
				rej.Close()
				return nil, err
			}
			edges = append(edges, edge{EmbKey{SourceKindRejection, id}, GraphVia{id, seed.FullID, "see_instead"}, i})
		}
		if err := rej.Close(); err != nil {
			return nil, err
		}
	}
	if len(edges) == 0 {
		return nil, nil
	}

	// Edge type before seed rank: a conflicts_with edge from the third hit
	// is worth more than a refines edge from the first, which usually
	// leads up to a principle half the corpus refines.
	sort.SliceStable(edges, func(a, b int) bool {
		pa, pb := graphTypePriority[edges[a].via.Type], graphTypePriority[edges[b].via.Type]
		if pa != pb {
			return pa < pb
		}
		if edges[a].seed != edges[b].seed {
			return edges[a].seed < edges[b].seed
		}
		return edges[a].key.FullID < edges[b].key.FullID
	})

	// Resolved through the same enumeration the other paths use, so a
	// neighbour obeys the same status and namespace scoping: a superseded
	// statement one edge away is withdrawn, not a candidate.
	records, err := ix.embeddableRecords(namespace)
	if err != nil {
		return nil, err
	}
	byKey := make(map[EmbKey]Candidate, len(records))
	for _, c := range records {
		byKey[EmbKey{c.SourceKind, c.FullID}] = c
	}

	var out []Candidate
	for _, e := range edges {
		if len(out) == maxGraphNeighbours {
			break
		}
		if seen[e.key] {
			continue
		}
		c, ok := byKey[e.key]
		if !ok {
			continue
		}
		seen[e.key] = true
		via := e.via
		c.MatchKind = MatchGraph
		c.Via = &via
		c.Rank = 0
		out = append(out, c)
	}
	if err := ix.annotateEvidence(out, text, touches); err != nil {
		return nil, err
	}
	return out, nil
}
