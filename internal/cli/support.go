package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/nicklecoder/requiem/internal/index"
	"github.com/nicklecoder/requiem/internal/requiem"
	"github.com/nicklecoder/requiem/internal/trace"
)

// openService builds a Service rooted at the current working directory —
// requiem is run from the project root, the same convention as most git
// subcommands.
func openService() (*requiem.Service, error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	return requiem.Open(wd), nil
}

// printJSON writes v as indented JSON to stdout — the bare-data half of the
// CLI's output convention (see SPEC.md): success writes data to stdout,
// failure writes an error to stderr with a nonzero exit code, no wrapping
// {ok,data} envelope either way.
// requiem: cli/bare-stdout
func printJSON(v interface{}) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return fmt.Errorf("encode output: %w", err)
	}
	return nil
}

// warnCoverage reports incomplete embedding coverage on stderr, leaving
// stdout as bare JSON per the CLI output convention. Exit status is
// deliberately unaffected: the results are real, just partial, and failing
// the command would break pipelines over a condition the caller may already
// know about. stderr is the right channel because agent harnesses surface
// combined output, so the warning reaches the reader that needs it while
// `jq` never sees it.
// requiem: cli/diagnostics-stderr
func warnCoverage(c requiem.Coverage) {
	if w := c.Warning(); w != "" {
		fmt.Fprintln(os.Stderr, w)
	}
}

// warnBlastRadius reports the labelled code sites affected by a body change.
// This is the payoff of traceability, and it goes to stderr rather than
// requiring a separate `trace` call: an agent that has to know to ask will
// not ask at the moment it matters most.
func warnBlastRadius(fullID string, refs []requiem.ClassifiedRef) {
	if len(refs) == 0 {
		return
	}
	var code, docs int
	for _, r := range refs {
		if r.Kind == trace.KindDoc {
			docs++
		} else {
			code++
		}
	}
	fmt.Fprintf(os.Stderr, "requiem: %s changed; %d code site(s) and %d document(s) reference it:\n", fullID, code, docs)
	for _, r := range refs {
		fmt.Fprintf(os.Stderr, "requiem:   %s:%d (%s)\n", r.File, r.Line, r.Kind)
	}
}

// warnContradictingRefs reports code that disagrees with a decision: sites
// referencing a retired statement or a rejected idea. These describe the same
// hazard — a decision was made and the code was never brought along, so the
// corpus reads as settled while the source still asserts the old position.
//
// A report, never a gate. Code referencing a superseded statement is often a
// legitimate in-progress state: the decision landed Tuesday, the migration
// ships Friday.
func warnContradictingRefs(refs []requiem.ClassifiedRef) {
	if len(refs) == 0 {
		return
	}
	fmt.Fprintf(os.Stderr, "requiem: %d labelled code site(s) contradict a recorded decision:\n", len(refs))
	for _, r := range refs {
		reason := string(r.Class)
		if r.Class == requiem.RefAbstract {
			reason = "declared abstract, but code implements it"
		}
		fmt.Fprintf(os.Stderr, "requiem:   %s:%d references %s (%s)\n", r.File, r.Line, r.FullID, reason)
	}
}

// warnNearMisses reports labels that look like typos of a real statement id.
//
// Dangling labels are otherwise ignored, because documentation explaining the
// label format generates them by existing. A near miss is different: it sits
// within an edit or two of a real id, which a doc example never does, so it
// can be reported without bringing that noise back.
func warnNearMisses(misses []requiem.NearMiss) {
	if len(misses) == 0 {
		return
	}
	fmt.Fprintf(os.Stderr, "requiem: %d label(s) look like typos:\n", len(misses))
	for _, m := range misses {
		fmt.Fprintf(os.Stderr, "requiem:   %s:%d  requiem: %s\n", m.File, m.Line, m.FullID)
		fmt.Fprintf(os.Stderr, "requiem:     did you mean %s?\n", m.DidYouMean) // requiem:ignore message text, not a label
	}
}

// warnAuditBacklog reports how much of the queue this page covers.
//
// A backlog that refills as it is worked is not a defect — adjudicating a pair
// frees its slot for the next candidate — but without a total it looks like
// one: a real corpus went from 415 to 425 outstanding pairs after 97 verdicts,
// and a queue that appears to grow as you work it gets abandoned.
// requiem: retrieval/audit-pairs-share-identifiers
func warnAuditBacklog(shown int, p index.AuditProgress) {
	if p.Remaining == 0 && p.Statements == 0 {
		return
	}
	// Both numbers, because one of them moves as work is done and the other
	// is the size of the job. Remaining alone was the misleading half:
	// fifteen verdicts once took it from 93 to 92.
	// requiem: retrieval/audit-queue-drains
	fmt.Fprintf(os.Stderr,
		"requiem: showing %d of %d unadjudicated pair(s); %d of %d statement(s) swept at depth %d\n", // requiem:ignore message text, not a label
		shown, p.Remaining, p.Swept, p.Statements, p.Depth)
	if p.Swept < p.Statements {
		fmt.Fprintln(os.Stderr, "requiem: every verdict now drains the queue; raise --neighbors to sweep deeper once it is clear") // requiem:ignore message text, not a label
	}
}

// warnNothingMatched says so when no candidate rose above a weak match.
//
// check always fills its limit, so a page of results is not evidence that any
// of them is relevant — and an agent had no way to conclude "this idea is
// new". The verdict field carries this per candidate; this line states the
// conclusion once, on stderr, where a reader skimming combined output will
// see it without parsing the payload.
// requiem: retrieval/calibrated-verdict
func warnNothingMatched(candidates []index.Candidate) {
	if len(candidates) == 0 {
		fmt.Fprintln(os.Stderr, "requiem: no candidates matched — nothing in this namespace resembles the draft") // requiem:ignore message text, not a label
		return
	}
	if index.StrongestVerdict(candidates) == index.VerdictWeak {
		fmt.Fprintf(os.Stderr,
			"requiem: %d candidate(s) returned, all weak matches (vocabulary overlap only) — nothing here appears to state this already\n",
			len(candidates))
	}
}

// warnDanglingPointers reports rejections whose see_instead names no
// statement. A rejection exists to answer "what was done instead", so a
// pointer resolving to nothing is the one part of it that can rot — and it
// rotted silently: `mv` rewrote statement relationships but not these, and
// nothing reported the break.
// requiem: model/see-instead-is-checked
func warnDanglingPointers(dangling []index.DanglingPointer) {
	if len(dangling) == 0 {
		return
	}
	fmt.Fprintf(os.Stderr, "requiem: %d rejection(s) point at a statement that does not exist:\n", len(dangling))
	for _, d := range dangling {
		fmt.Fprintf(os.Stderr, "requiem:   %s: see_instead %s\n", d.Rejection, d.SeeInstead)
	}
	fmt.Fprintln(os.Stderr, "requiem: re-point each with `requiem update <id> --rejection --see-instead <statement>`") // requiem:ignore message text, not a label
}

// warnIndexDiff names the records a reindex added, updated or removed.
//
// The counts alone could not be acted on: "removed: 1" with no id left the
// reader unable to tell what had left the index, and only requiem knows
// which file path held which record.
// requiem: cli/index-diffs-name-ids
func warnIndexDiff(stats index.ReindexStats) {
	if len(stats.RemovedIDs) == 0 {
		return
	}
	fmt.Fprintf(os.Stderr, "requiem: %d record(s) removed from the index: %s\n",
		len(stats.RemovedIDs), strings.Join(stats.RemovedIDs, ", "))
}

// warnRewrittenRefs reports source files mv edited. Source edits are the one
// thing requiem does outside .requiem/, so they are announced rather than
// left to be discovered in a diff.
func warnRewrittenRefs(res *requiem.MoveResult) {
	if len(res.RewrittenRefs) > 0 {
		fmt.Fprintf(os.Stderr, "requiem: rewrote %d labelled site(s) to %s:\n", len(res.RewrittenRefs), res.To) // requiem:ignore message text, not a label
		for _, r := range res.RewrittenRefs {
			fmt.Fprintf(os.Stderr, "requiem:   %s:%d\n", r.File, r.Line)
		}
		fmt.Fprintln(os.Stderr, "requiem: source edits are UNSTAGED — review with `git diff`") // requiem:ignore message text, not a label
	}
	if len(res.OrphanedCodeRefs) > 0 {
		fmt.Fprintf(os.Stderr, "requiem: %d labelled site(s) still name %s:\n", len(res.OrphanedCodeRefs), res.From)
		for _, r := range res.OrphanedCodeRefs {
			fmt.Fprintf(os.Stderr, "requiem:   %s:%d\n", r.File, r.Line)
		}
	}
}
