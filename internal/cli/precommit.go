package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// commitEnvMarker is set by `requiem commit` so the pre-commit hook stays
// quiet for the one commit that is already an explicit approval.
const commitEnvMarker = "REQUIEM_COMMIT"

func newPrecommitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "precommit-notice",
		Short: "Report pending statements a commit is about to make permanent",
		Long: "Installed as a pre-commit hook by `requiem init`. Requiem auto-stages every\n" +
			"mutation and commit is approval, so a plain `git commit -a` can approve\n" +
			"statements nobody reviewed. This names them at the moment it happens.\n\n" +
			"It never blocks: it always exits zero. A blocking hook would be one\n" +
			"--no-verify away from useless while making requiem a gate on every commit\n" +
			"in the repository, which is not what this tool is for.",
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Silent when the commit came from `requiem commit`, which is
			// already a deliberate approval.
			if os.Getenv(commitEnvMarker) != "" {
				return nil
			}
			svc, err := openService()
			if err != nil {
				return nil
			}
			pending, err := svc.PendingApproval()
			if err != nil || len(pending) == 0 {
				return nil
			}
			fmt.Fprintf(os.Stderr, "requiem: this commit also approves %d pending statement change(s):\n", len(pending)) // requiem:ignore message text, not a label
			for _, p := range pending {
				fmt.Fprintf(os.Stderr, "requiem:   %s %s\n", p.Change, p.FullID)
			}
			fmt.Fprintln(os.Stderr, "requiem: `requiem discard` backs them out if unintended")
			return nil
		},
	}
}
