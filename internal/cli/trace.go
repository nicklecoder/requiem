package cli

import (
	"github.com/spf13/cobra"
)

func newTraceCmd() *cobra.Command {
	var search bool
	var searchLimit int

	cmd := &cobra.Command{
		Use:   "trace <namespace/id>",
		Short: "Show labelled source sites referencing a statement",
		Long: "Finds `requiem: <namespace/id>` marker comments in the working tree, so a\n" +
			"changed decision can report the code it affects. Labels travel with code\n" +
			"through refactors, which a stored line range cannot.\n\n" +
			"Scans the tree live rather than reading cached counts: this is a direct\n" +
			"question about the code as it is now, and a stale answer is worse than a\n" +
			"slow one. Untracked files count, gitignored paths do not.\n\n" +
			"--search additionally looks for the statement's own distinctive words in\n" +
			"the code, ranked by how many of them each file contains. That is weaker\n" +
			"evidence than a label and is reported separately, but it means the\n" +
			"question is answerable before anything has been labelled at all.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := openService()
			if err != nil {
				return err
			}
			res, err := svc.Trace(args[0], search, searchLimit)
			if err != nil {
				return err
			}
			return printJSON(res)
		},
	}
	cmd.Flags().BoolVar(&search, "search", false, "also search code for the statement's own vocabulary, for statements with no label yet")
	cmd.Flags().IntVar(&searchLimit, "search-limit", 10, "maximum files to return from --search (0 = unlimited)")
	return cmd
}
