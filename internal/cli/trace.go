package cli

import (
	"github.com/spf13/cobra"
)

func newTraceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "trace <namespace/id>",
		Short: "Show labelled source sites referencing a statement",
		Long: "Finds `requiem: <namespace/id>` marker comments in the working tree, so a\n" +
			"changed decision can report the code it affects. Labels travel with code\n" +
			"through refactors, which a stored line range cannot.\n\n" +
			"Scans the tree live rather than reading cached counts: this is a direct\n" +
			"question about the code as it is now, and a stale answer is worse than a\n" +
			"slow one. Untracked files count, gitignored paths do not.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := openService()
			if err != nil {
				return err
			}
			res, err := svc.Trace(args[0])
			if err != nil {
				return err
			}
			return printJSON(res)
		},
	}
	return cmd
}
