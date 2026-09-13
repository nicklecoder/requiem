package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/nicklecoder/requiem/internal/requiem"
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
	fmt.Fprintf(os.Stderr, "requiem: %s changed; %d labelled code site(s) reference it:\n", fullID, len(refs))
	for _, r := range refs {
		fmt.Fprintf(os.Stderr, "requiem:   %s:%d\n", r.File, r.Line)
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
