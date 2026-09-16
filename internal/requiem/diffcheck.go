package requiem

import (
	"fmt"
	"sort"
	"strings"

	"github.com/nicklecoder/requiem/internal/config"
	"github.com/nicklecoder/requiem/internal/git"
	"github.com/nicklecoder/requiem/internal/index"
	"github.com/nicklecoder/requiem/internal/model"
	"github.com/nicklecoder/requiem/internal/trace"
)

// CoverageReason says why a decision is considered to cover a change. Reported
// rather than collapsed into one list, because the three differ in strength: a
// label is an exact claim someone wrote down, a provenance range is where a
// statement was derived from, and an identifier match is inference.
type CoverageReason string

const (
	// ReasonLabel is a `requiem:` label inside an edited hunk.
	ReasonLabel CoverageReason = "label"
	// ReasonProvenance is a code-derived statement whose source range the
	// change touches.
	ReasonProvenance CoverageReason = "provenance"
	// ReasonIdentifier is a decision naming an identifier the patch adds.
	ReasonIdentifier CoverageReason = "identifier"
)

// DiffCoverage is one decision covering one part of a change.
type DiffCoverage struct {
	FullID string         `json:"full_id"`
	Reason CoverageReason `json:"reason"`
	File   string         `json:"file,omitempty"`
	Line   int            `json:"line,omitempty"`
	// Facet names the identifier that matched, for ReasonIdentifier.
	Facet string `json:"facet,omitempty"`
	// Modality and Excerpt let a reader judge without a second call.
	Modality model.Modality `json:"modality,omitempty"`
	Status   model.Status   `json:"status,omitempty"`
	Excerpt  string         `json:"excerpt,omitempty"`
	// Stale marks a covering statement whose source range has drifted since
	// it was written: it may no longer describe the code it claims to.
	Stale bool `json:"stale,omitempty"`
	// Challenged marks a decision a proposal argues against.
	Challenged bool `json:"challenged,omitempty"`
}

// DiffCheck is what `check --diff` answers: given a patch, which recorded
// decisions bear on it.
//
// This is the moment of use the field report identified as the real gap. A
// mature repository does not have a fresh intent document to check against; it
// has a diff. Retrieval that only answers when someone thinks to ask is
// retrieval that gets skipped.
// requiem: traceability/diff-scoped-check
type DiffCheck struct {
	Rev   string   `json:"rev,omitempty"`
	Files []string `json:"files"`
	// Covering are the decisions that bear on this change.
	Covering []DiffCoverage `json:"covering"`
	// Rejections are ideas this project turned down whose identifiers appear
	// in what the patch adds — the most valuable thing in the corpus and the
	// easiest to walk back into.
	Rejections []DiffCoverage `json:"rejections,omitempty"`
	// Contradictions are labelled sites in the change that point at a
	// retired or rejected decision. These are checkable facts, so they are
	// what a gate may fail on.
	Contradictions []ClassifiedRef `json:"contradictions,omitempty"`
	// Gate is the configured mode: off, warn or error.
	Gate string `json:"gate"`
}

// Failing reports whether the gate should fail the caller.
//
// Only on facts: code labelled with a decision that has been retired or
// rejected, or a covering statement whose source range has drifted. Never on
// the mere existence of covering decisions — "this change touches decisions
// you did not read" is a judgment about intent, and a rule approximating it
// would be wrong often enough to be disabled, which is the same reasoning
// that keeps labels unenforced.
// requiem: traceability/diff-gate-is-per-project
func (d *DiffCheck) Failing() bool {
	if d.Gate != config.GateError {
		return false
	}
	if len(d.Contradictions) > 0 {
		return true
	}
	for _, c := range d.Covering {
		if c.Stale {
			return true
		}
	}
	return false
}

// Findings summarizes what a gate would fail on, for stderr.
func (d *DiffCheck) Findings() []string {
	var out []string
	for _, c := range d.Contradictions {
		out = append(out, fmt.Sprintf("%s:%d references %s (%s)", c.File, c.Line, c.FullID, c.Class))
	}
	for _, c := range d.Covering {
		if c.Stale {
			out = append(out, fmt.Sprintf("%s was derived from code that has changed since", c.FullID))
		}
	}
	return out
}

// CheckDiff finds the decisions bearing on a revision's changes. rev is passed
// to git as given; "" means the working tree against HEAD and "--staged" the
// index.
func (s *Service) CheckDiff(rev string) (*DiffCheck, error) {
	diff, err := s.Git.Diff(rev)
	if err != nil {
		return nil, err
	}
	// requiem's own records are excluded: they are not the code the corpus
	// describes, so a patch that records a decision would otherwise report
	// itself as a change that decision covers. `review` is the command for
	// looking at pending corpus edits.
	// requiem: traceability/diff-scoped-check
	var changes []git.Change
	for _, c := range git.ParseDiff(diff) {
		if c.File == requiemDir || strings.HasPrefix(c.File, requiemDir+"/") {
			continue
		}
		changes = append(changes, c)
	}

	cfg, err := config.Load(s.Store.Root)
	if err != nil {
		return nil, err
	}
	out := &DiffCheck{Rev: rev, Gate: cfg.GateDiff(), Files: []string{}, Covering: []DiffCoverage{}}
	for _, c := range changes {
		out.Files = append(out.Files, c.File)
	}
	if len(changes) == 0 {
		return out, nil
	}

	ix, err := s.openIndex()
	if err != nil {
		return nil, err
	}
	defer ix.Close()
	if _, err := ix.Reindex(s.Store); err != nil {
		return nil, fmt.Errorf("reindex before diff check: %w", err)
	}

	byFile := make(map[string]git.Change, len(changes))
	for _, c := range changes {
		byFile[c.File] = c
	}
	challenged, err := ix.ChallengedIDs()
	if err != nil {
		return nil, err
	}
	statements, err := ix.ListStatements(index.ListFilter{})
	if err != nil {
		return nil, err
	}
	byID := make(map[string]model.Statement, len(statements))
	for _, st := range statements {
		byID[st.FullID()] = st
	}

	seen := map[string]bool{}
	add := func(cov DiffCoverage) {
		key := string(cov.Reason) + "\x00" + cov.FullID + "\x00" + cov.File + "\x00" + cov.Facet
		if seen[key] {
			return
		}
		seen[key] = true
		if st, ok := byID[cov.FullID]; ok {
			cov.Modality, cov.Status, cov.Excerpt = st.Modality, st.Status, excerpt(st.Body)
			if st.Provenance.Type == model.ProvenanceCodeDerived && st.Provenance.LineRange != nil {
				cov.Stale = computeStale(s.Root, st.Provenance)
			}
			cov.Challenged = challenged[cov.FullID]
		}
		out.Covering = append(out.Covering, cov)
	}

	// Labels inside an edited hunk: the strongest signal, because someone
	// wrote the connection down at the site.
	refs, err := trace.Scan(s.Root)
	if err != nil {
		return nil, err
	}
	classified, err := classifyRefs(ix, refs)
	if err != nil {
		return nil, err
	}
	for _, ref := range classified {
		change, ok := byFile[ref.File]
		if !ok || !change.Touches(ref.Line, ref.Line) {
			continue
		}
		if ref.Class.Contradiction() && ref.Kind != trace.KindDoc {
			out.Contradictions = append(out.Contradictions, ref)
			continue
		}
		// A label naming nothing is not a decision covering this change. The
		// marker matches anywhere in a tracked file, so prose — or a stderr
		// message whose text happens to take the marker's shape — scans as
		// one, and reporting it here would put a decision that does not exist
		// in front of a reader. Dangling labels stay visible through `trace`,
		// where someone is asking about one specific id and can judge it.
		// requiem: traceability/label-false-positives
		if ref.Class == RefDangling {
			continue
		}
		add(DiffCoverage{FullID: ref.FullID, Reason: ReasonLabel, File: ref.File, Line: ref.Line})
	}

	// Code-derived statements whose own source range the change touches.
	for _, st := range statements {
		if st.Provenance.Type != model.ProvenanceCodeDerived || st.Provenance.LineRange == nil {
			continue
		}
		change, ok := byFile[st.Provenance.File]
		if !ok || !change.Touches(st.Provenance.LineRange.Start, st.Provenance.LineRange.End) {
			continue
		}
		add(DiffCoverage{
			FullID: st.FullID(), Reason: ReasonProvenance,
			File: st.Provenance.File, Line: st.Provenance.LineRange.Start,
		})
	}

	// Identifiers the patch introduces, matched against the facet index.
	// This is what reaches a decision nobody labelled and no provenance
	// points at — including a rejection, which is the record most easily
	// walked back into by accident.
	added := make([]string, 0, len(changes))
	for _, c := range changes {
		added = append(added, c.Added)
	}
	facets := index.ExtractFacets(strings.Join(added, "\n"))
	for _, facet := range facets {
		records, err := ix.RecordsWithFacet(facet)
		if err != nil {
			return nil, err
		}
		for _, r := range records {
			cov := DiffCoverage{FullID: r.FullID, Reason: ReasonIdentifier, Facet: facet}
			if r.SourceKind == index.SourceKindRejection {
				cov.Excerpt = r.Excerpt
				out.Rejections = append(out.Rejections, cov)
				continue
			}
			add(cov)
		}
	}

	sortCoverage(out.Covering)
	sortCoverage(out.Rejections)
	return out, nil
}

// sortCoverage orders by reason strength, then id, so output is stable and
// the strongest evidence reads first.
func sortCoverage(cov []DiffCoverage) {
	strength := map[CoverageReason]int{ReasonLabel: 0, ReasonProvenance: 1, ReasonIdentifier: 2}
	sort.Slice(cov, func(i, j int) bool {
		if strength[cov[i].Reason] != strength[cov[j].Reason] {
			return strength[cov[i].Reason] < strength[cov[j].Reason]
		}
		if cov[i].FullID != cov[j].FullID {
			return cov[i].FullID < cov[j].FullID
		}
		return cov[i].Line < cov[j].Line
	})
}
