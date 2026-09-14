package cli

import (
	"encoding/json"
	"fmt"
	"os"

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
		fmt.Fprintf(os.Stderr, "requiem:   %s:%d references %s (%s)\n", r.File, r.Line, r.FullID, r.Class)
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
		fmt.Fprintf(os.Stderr, "requiem:     did you mean %s?\n", m.DidYouMean)
	}
}

// warnRewrittenRefs reports source files mv edited. Source edits are the one
// thing requiem does outside .requiem/, so they are announced rather than
// left to be discovered in a diff.
func warnRewrittenRefs(res *requiem.MoveResult) {
	if len(res.RewrittenRefs) > 0 {
		fmt.Fprintf(os.Stderr, "requiem: rewrote %d labelled site(s) to %s:\n", len(res.RewrittenRefs), res.To)
		for _, r := range res.RewrittenRefs {
			fmt.Fprintf(os.Stderr, "requiem:   %s:%d\n", r.File, r.Line)
		}
		fmt.Fprintln(os.Stderr, "requiem: source edits are UNSTAGED — review with `git diff`")
	}
	if len(res.OrphanedCodeRefs) > 0 {
		fmt.Fprintf(os.Stderr, "requiem: %d labelled site(s) still name %s:\n", len(res.OrphanedCodeRefs), res.From)
		for _, r := range res.OrphanedCodeRefs {
			fmt.Fprintf(os.Stderr, "requiem:   %s:%d\n", r.File, r.Line)
		}
	}
}
