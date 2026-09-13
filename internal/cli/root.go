// Package cli defines requiem's cobra command tree. Commands only parse
// flags and format stdout/stderr/exit-code — all real logic lives in
// internal/requiem, so it stays testable without going through cobra or a
// subprocess.
package cli

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// version is set via -ldflags at release build time (see M7); "dev" locally.
var version = "dev"

// errNotImplemented is returned by verbs not yet built for the current
// milestone. Distinct from a normal usage/runtime error so it's obvious in
// output during incremental development.
var errNotImplemented = errors.New("not yet implemented")

// NewRootCmd builds the full requiem command tree.
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "requiem",
		Short:         "Agent-native requirements, rules, and design-decision tracking",
		Long:          "Requiem tracks requirements, rules, and design decisions per project so an AI agent can cheaply check whether a new idea conflicts with a prior decision, without loading the whole spec into context.",
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       version,
	}

	root.AddCommand(
		newInitCmd(),
		newAddCmd(),
		newUpdateCmd(),
		newLinkCmd(),
		newRejectCmd(),
		newGetCmd(),
		newListCmd(),
		newCheckCmd(),
		newEmbedCmd(),
		newAuditCmd(),
		newMvCmd(),
		newTraceCmd(),
		newReindexCmd(),
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
