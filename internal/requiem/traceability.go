package requiem

import (
	"fmt"
	"sort"

	"github.com/nicklecoder/requiem/internal/index"
	"github.com/nicklecoder/requiem/internal/model"
	"github.com/nicklecoder/requiem/internal/trace"
)

// RefClass is what a scanned label resolves to. Two of these are worth
// raising unprompted; the rest are ordinary.
type RefClass string

const (
	// RefActive is code referencing a statement in force. Nothing to say.
	RefActive RefClass = "active"
	// RefProposed is code referencing a decision still under consideration —
	// someone built ahead of the decision. Worth knowing, not alarming.
	RefProposed RefClass = "proposed"
	// RefRetired is code referencing a superseded or deprecated statement:
	// the decision changed and the code was never brought along.
	RefRetired RefClass = "retired"
	// RefRejected is code referencing an idea this project explicitly turned
	// down.
	RefRejected RefClass = "rejected"
	// RefDangling is a label naming nothing that exists — the reference
	// rotted, or the id was mistyped.
	RefDangling RefClass = "dangling"
	// RefAbstract is code referencing a statement whose author declared that
	// no code can implement it. The declaration excludes the statement from
	// --unreferenced forever, so a wrong one hides a real gap permanently —
	// and this is the one case where evidence can contradict it.
	RefAbstract RefClass = "abstract"
)

// Contradiction reports whether this class describes code disagreeing with a
// decision, as opposed to code that is merely out of date with a label or
// ahead of a decision. These are the classes audit raises on its own.
func (c RefClass) Contradiction() bool {
	return c == RefRetired || c == RefRejected || c == RefAbstract
}

// ClassifiedRef is one labelled site with the verdict on what it points at.
type ClassifiedRef struct {
	FullID string     `json:"full_id"`
	File   string     `json:"file"`
	Line   int        `json:"line"`
	Kind   trace.Kind `json:"kind"`
	Class  RefClass   `json:"class"`
	// Status is the referenced statement's status, absent for rejections and
	// dangling labels.
	Status model.Status `json:"status,omitempty"`
}

// classifyRefs resolves every scanned label against both statements and
// rejections.
//
// Consulting both is not a completeness nicety. Resolve against statements
// alone and a rejection id found in source matches nothing, so it reports as
// a dangling label — "points at something that no longer exists" — when the
// truthful report is nearly the opposite: this code implements an idea the
// project explicitly rejected. Same input, inverted meaning, one lookup
// between them.
func classifyRefs(ix *index.Index, refs []trace.Ref) ([]ClassifiedRef, error) {
	statements, err := ix.ListStatements(index.ListFilter{})
	if err != nil {
		return nil, err
	}
	status := make(map[string]model.Status, len(statements))
	abstract := make(map[string]bool, len(statements))
	for _, st := range statements {
		status[st.FullID()] = st.Status
		abstract[st.FullID()] = st.Abstract
	}

	rejections, err := ix.AllRejectionIDs()
	if err != nil {
		return nil, err
	}
	rejected := make(map[string]bool, len(rejections))
	for _, id := range rejections {
		rejected[id] = true
	}

	out := make([]ClassifiedRef, 0, len(refs))
	for _, r := range refs {
		c := ClassifiedRef{FullID: r.FullID, File: r.File, Line: r.Line, Kind: r.Kind}
		switch st, ok := status[r.FullID]; {
		case ok:
			c.Status = st
			switch {
			case abstract[r.FullID]:
				c.Class = RefAbstract
			case st == model.StatusActive:
				c.Class = RefActive
			case st == model.StatusProposed:
				c.Class = RefProposed
			default:
				c.Class = RefRetired
			}
		case rejected[r.FullID]:
			c.Class = RefRejected
		default:
			c.Class = RefDangling
		}
		out = append(out, c)
	}
	return out, nil
}

// TraceResult is `trace`'s output for one statement.
type TraceResult struct {
	FullID string   `json:"full_id"`
	Class  RefClass `json:"class"`
	// CodeRefs and DocMentions are counted apart: only the first says
	// anything about whether the decision is implemented.
	CodeRefs    int             `json:"code_refs"`
	DocMentions int             `json:"doc_mentions"`
	Refs        []ClassifiedRef `json:"refs"`
	// SearchHits are files whose text overlaps this statement's vocabulary,
	// populated only when asked for. Weaker evidence than a label — the words
	// appear, which is not the same as the code implementing the decision —
	// so they are reported apart from Refs rather than mixed in.
	SearchHits []trace.SearchHit `json:"search_hits,omitempty"`
	// Commits carry Requiem-Id trailers naming this statement. Labels answer
	// where a decision lives now; trailers answer when it was implemented and
	// by what change, which labels structurally cannot.
	Commits []trace.Commit `json:"commits,omitempty"`
}

// Trace reports the labelled source sites referencing one statement.
//
// Scans live rather than reading the counts stored by reindex: this is a
// direct question about the tree as it is now, and a stale answer to "what
// implements this" is worse than a slow one. The stored counts exist for the
// read path, where a hint may lag.
func (s *Service) Trace(fullID string, search bool, searchLimit int) (*TraceResult, error) {
	ix, err := s.openIndex()
	if err != nil {
		return nil, err
	}
	defer ix.Close()
	if _, err := ix.Reindex(s.Store); err != nil {
		return nil, fmt.Errorf("reindex before trace: %w", err)
	}

	refs, err := trace.Scan(s.Root)
	if err != nil {
		return nil, err
	}
	classified, err := classifyRefs(ix, refs)
	if err != nil {
		return nil, err
	}

	out := &TraceResult{FullID: fullID, Refs: []ClassifiedRef{}}
	for _, c := range classified {
		if c.FullID != fullID {
			continue
		}
		out.Class = c.Class
		if c.Kind == trace.KindDoc {
			out.DocMentions++
		} else {
			out.CodeRefs++
		}
		out.Refs = append(out.Refs, c)
	}
	if search {
		st, err := ix.GetStatement(fullID)
		if err == nil {
			hits, err := trace.Search(s.Root, st.Body, searchLimit)
			if err != nil {
				return nil, err
			}
			// A file already carrying a label needs no weaker evidence for
			// the same statement.
			labelled := map[string]bool{}
			for _, r := range out.Refs {
				labelled[r.File] = true
			}
			for _, h := range hits {
				if !labelled[h.File] {
					out.SearchHits = append(out.SearchHits, h)
				}
			}
		}
	}

	commits, err := trace.Commits(s.Root, fullID)
	if err != nil {
		return nil, err
	}
	out.Commits = commits

	if out.Class == "" {
		// No labels point here, so the class describes the statement itself
		// rather than any reference to it.
		out.Class = classOfTarget(ix, fullID)
	}
	return out, nil
}

func classOfTarget(ix *index.Index, fullID string) RefClass {
	st, err := ix.GetStatement(fullID)
	if err == nil {
		switch st.Status {
		case model.StatusActive:
			return RefActive
		case model.StatusProposed:
			return RefProposed
		default:
			return RefRetired
		}
	}
	return RefDangling
}

// ContradictingRefs returns the labelled sites that disagree with a decision:
// code referencing a retired statement or a rejected idea. Sorted so output
// is stable between runs.
//
// Code only. A document explaining why an idea was rejected cites that
// rejection legitimately and contradicts nothing — treating it as a finding
// would make writing about a decision an offence against it.
func ContradictingRefs(refs []ClassifiedRef) []ClassifiedRef {
	var out []ClassifiedRef
	for _, r := range refs {
		if r.Kind == trace.KindDoc {
			continue
		}
		if r.Class.Contradiction() {
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].FullID != out[j].FullID {
			return out[i].FullID < out[j].FullID
		}
		if out[i].File != out[j].File {
			return out[i].File < out[j].File
		}
		return out[i].Line < out[j].Line
	})
	return out
}

// LabelResult is Label's output.
type LabelResult struct {
	FullID string `json:"full_id"`
	File   string `json:"file"`
	Line   int    `json:"line"`
}

// Label inserts a marker comment tying a code location to a statement.
//
// The id is resolved before anything is written: labelling is meant to reduce
// the friction that makes labels get skipped, and a command that cheerfully
// writes a typo would replace that friction with a dangling reference found
// weeks later. Rejections resolve too — code referencing a rejected idea is a
// finding audit reports, not something to refuse at the point of writing.
func (s *Service) Label(fullID, file string, line int, comment string) (*LabelResult, error) {
	ix, err := s.openIndex()
	if err != nil {
		return nil, err
	}
	defer ix.Close()
	if _, err := ix.Reindex(s.Store); err != nil {
		return nil, err
	}

	known := false
	if _, err := ix.GetStatement(fullID); err == nil {
		known = true
	} else {
		rejections, err := ix.AllRejectionIDs()
		if err != nil {
			return nil, err
		}
		for _, id := range rejections {
			if id == fullID {
				known = true
				break
			}
		}
	}
	if !known {
		return nil, fmt.Errorf("no statement or rejection %q: labelling an id that does not exist would leave a dangling reference", fullID)
	}

	written, err := trace.InsertLabel(s.Root, file, line, fullID, comment)
	if err != nil {
		return nil, err
	}
	return &LabelResult{FullID: fullID, File: file, Line: written}, nil
}
