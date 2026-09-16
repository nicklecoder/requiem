// Package cli defines requiem's cobra command tree. Commands only parse
// flags and format stdout/stderr/exit-code — all real logic lives in
// internal/requiem, so it stays testable without going through cobra or a
// subprocess.
package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// version is set via -ldflags at release build time (see M7); "dev" locally.
var version = "dev"

// NewRootCmd builds the full requiem command tree.
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "requiem",
		Short: "Agent-native requirements, rules, and design-decision tracking",
		Long: `Requiem tracks requirements, rules, and design decisions ("statements") per
project, so an agent can check whether a new idea conflicts with a prior one
without holding the whole specification in context.

Statement files under .requiem/ are canonical and git-tracked. A SQLite index
beside them is disposable and rebuilt from those files on demand, so nothing
is lost if it is deleted.

WORKFLOW

  Before proposing anything non-trivial
    check --text "<the idea>"
      Read the rejections it returns first: they are ideas this project
      already considered and turned down, and re-proposing one is the most
      common way an agent wastes a human's time.
      Searches every namespace; add --namespace <area> to narrow it.

  When a decision is made
    add     the decision, with --modality if it carries normative force
    link    it to the principle it refines, so the graph is navigable
    reject  the alternatives that lost, with --see-instead pointing here

    An open question goes in as "--status proposed", never hedged into the
    body of an active statement. "list --status proposed" is the only queue
    of open decisions; a question written into an active body is invisible
    to it, and goes stale silently once answered elsewhere.

  While implementing it
    Label the code: write "requiem: <namespace/id>" in an ordinary comment
    above the code that carries the decision, using that file's own comment
    syntax. Requiem has no command for this — you are already editing the
    file and know its language, where requiem could only guess from the
    extension and would eventually guess wrong.

    Labelling a test is stronger than labelling an implementation: a passing
    labelled test is evidence the statement holds, where a comment only
    asserts intent.

    Labels are a shortcut, not a requirement. Nothing enforces them, and
    nothing should: deciding whether a change "should" have been labelled is
    a judgment about intent, and a rule approximating it would be wrong often
    enough to get disabled. Partial coverage is useful — a labelled site is
    an exact answer, and "trace --search" still answers from the statement's
    own words where no label exists.

  Starting work in an area
    brief --namespace <area>
      The minimal set in force there: must/must_not rules, the principles they
      refine, and what has already been rejected. Small enough to paste into a
      prompt, and it counts what the cap left out rather than hiding it.

  With a change in flight
    check --diff
      Which recorded decisions bear on this patch — labels inside an edited
      hunk, statements whose own source range you touched, and records naming
      an identifier the change adds. Failing a build on it is opt-in per
      project via "gate.diff" in .requiem/config.yaml.

  Recording many decisions at once
    batch
      JSON Lines on stdin, one {"op": ...} per line, with a result per record.

  Periodically
    reindex --embed   refresh vectors and rescan labels
    audit             candidate pairs worth judging: ones sharing an identifier
                      or a source file first, then nearest neighbours. Record a
                      real finding with "link --type conflicts_with|duplicates";
                      dismiss an unrelated pair with "dismiss <a> <b>", which
                      keeps bookkeeping out of the graph
    list --unreferenced   decisions no code implements
    trace <id> --search   what implements a decision, labelled or not

  Nothing is permanent until "requiem commit". Writes auto-stage, so backing
  out of a dead end with "discard" leaves no trace in history.

CONVENTIONS

  Output is bare JSON on stdout; diagnostics go to stderr, so pipelines stay
  clean while an agent reading combined output still sees them. Failures exit
  nonzero.

  Write a statement body that states the decision AND why. "Use Postgres" is
  not retrievable; "Session state lives in Postgres rather than Redis, because
  it must survive a restart" matches a future draft that shares neither word.
`,
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       version,
	}

	// Capability is computed once: the embedding-dependent surface is hidden
	// where it could only fail. `embed --vector` and `check --vector` are not
	// gated — they take a vector the caller produced and need no endpoint.
	semantic := semanticAvailable()

	root.AddCommand(
		newInitCmd(),
		newAddCmd(),
		newUpdateCmd(),
		newLinkCmd(),
		newRejectCmd(),
		newGetCmd(),
		newListCmd(),
		newCheckCmd(semantic),
		newBriefCmd(),
		newBatchCmd(),
		newEmbedCmd(),
		newAuditCmd(semantic),
		newDismissCmd(),
		newMvCmd(),
		newTraceCmd(),
		newPrecommitCmd(),
		newManCmd(),
		newReindexCmd(semantic),
		newReviewCmd(),
		newCommitCmd(),
		newDiscardCmd(),
	)

	return root
}

// Execute runs the root command and maps errors to a process exit code.
// Bare data goes to stdout on success; errors go to stderr with a nonzero
// exit code — no {ok,data} envelope (see SPEC.md CLI output convention).
func Execute() int {
	cmd := NewRootCmd()
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	return 0
}
