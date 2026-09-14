package requiem

import (
	"sort"

	"github.com/nicklecoder/requiem/internal/index"
	"github.com/nicklecoder/requiem/internal/model"
	"github.com/nicklecoder/requiem/internal/trace"
)

// nearMissDistance is the edit distance within which a dangling label is
// treated as a typo rather than an unrelated string.
//
// Two, matching the threshold cobra already uses for "Did you mean this?" in
// this same binary — a convention to reuse rather than a number to invent.
//
// The threshold is what makes this reportable at all. Dangling labels are
// otherwise ignored, because prose explaining the label format reads as a
// label and documentation would generate findings by existing. A typo sits
// one or two edits from a real id; a documentation example sits nowhere near
// anything. Distance separates the two cleanly.
const nearMissDistance = 2

// NearMiss is a label that resolves to nothing but closely resembles a real
// statement id.
type NearMiss struct {
	FullID     string     `json:"full_id"`
	File       string     `json:"file"`
	Line       int        `json:"line"`
	Kind       trace.Kind `json:"kind"`
	DidYouMean string     `json:"did_you_mean"`
}

// findNearMisses pairs each dangling label with the closest real id within
// nearMissDistance, if any.
func findNearMisses(refs []ClassifiedRef, knownIDs []string) []NearMiss {
	var out []NearMiss
	for _, r := range refs {
		if r.Class != RefDangling {
			continue
		}
		best, dist := "", nearMissDistance+1
		for _, id := range knownIDs {
			if d := levenshtein(r.FullID, id); d < dist {
				best, dist = id, d
			}
		}
		if best == "" || dist > nearMissDistance {
			continue
		}
		out = append(out, NearMiss{
			FullID: r.FullID, File: r.File, Line: r.Line, Kind: r.Kind, DidYouMean: best,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].File != out[j].File {
			return out[i].File < out[j].File
		}
		return out[i].Line < out[j].Line
	})
	return out
}

// levenshtein is the standard edit distance, two rows rather than a full
// matrix since only the previous row is ever needed.
func levenshtein(a, b string) int {
	if a == b {
		return 0
	}
	ar, br := []rune(a), []rune(b)
	prev := make([]int, len(br)+1)
	curr := make([]int, len(br)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ar); i++ {
		curr[0] = i
		for j := 1; j <= len(br); j++ {
			cost := 1
			if ar[i-1] == br[j-1] {
				cost = 0
			}
			curr[j] = min3(curr[j-1]+1, prev[j]+1, prev[j-1]+cost)
		}
		prev, curr = curr, prev
	}
	return prev[len(br)]
}

func min3(a, b, c int) int {
	if b < a {
		a = b
	}
	if c < a {
		a = c
	}
	return a
}

// NearMisses reports labels that look like typos of a real statement id.
// Scans live, like every other direct question about the tree.
func (s *Service) NearMisses() ([]NearMiss, error) {
	ix, err := s.openIndex()
	if err != nil {
		return nil, err
	}
	defer ix.Close()
	if _, err := ix.Reindex(s.Store); err != nil {
		return nil, err
	}

	refs, err := trace.Scan(s.Root)
	if err != nil {
		return nil, err
	}
	classified, err := classifyRefs(ix, refs)
	if err != nil {
		return nil, err
	}

	statements, err := ix.ListStatements(index.ListFilter{})
	if err != nil {
		return nil, err
	}
	rejections, err := ix.AllRejectionIDs()
	if err != nil {
		return nil, err
	}
	known := make([]string, 0, len(statements)+len(rejections))
	for _, st := range statements {
		known = append(known, st.FullID())
	}
	known = append(known, rejections...)

	return findNearMisses(classified, known), nil
}

// transitiveCoverage maps each statement to the refining statements that
// carry its implementation.
//
// A principle is implemented through the rules that refine it, not at any
// single code site, so without this `--unreferenced` reports every principle
// forever and the real finding — implemented but unlabelled — is buried.
//
// Only *active* refiners count. A proposal refining a principle is not built
// yet, so counting it would hold the principle permanently uncovered; this
// corpus has exactly that case. A statement is covered only when every active
// refiner carries a label, so partial implementation does not read as
// complete.
func transitiveCoverage(ix *index.Index, counts map[string]int) (map[string][]string, error) {
	statements, err := ix.ListStatements(index.ListFilter{})
	if err != nil {
		return nil, err
	}
	status := make(map[string]model.Status, len(statements))
	for _, st := range statements {
		status[st.FullID()] = st.Status
	}

	out := map[string][]string{}
	for _, st := range statements {
		refiners, err := ix.InboundRelationships(st.FullID())
		if err != nil {
			return nil, err
		}
		var active, labelled []string
		for _, r := range refiners {
			if r.Type != model.RelRefines || status[r.From] != model.StatusActive {
				continue
			}
			active = append(active, r.From)
			if counts[r.From] > 0 {
				labelled = append(labelled, r.From)
			}
		}
		if len(active) > 0 && len(labelled) == len(active) {
			out[st.FullID()] = labelled
		}
	}
	return out, nil
}
