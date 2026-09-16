package cli

import (
	"github.com/spf13/cobra"
)

func newDismissCmd() *cobra.Command {
	var note string
	var restore bool

	cmd := &cobra.Command{
		Use:   "dismiss <namespace/id> <namespace/id>",
		Short: "Record that an audit candidate pair was judged unrelated, so it stops resurfacing",
		Long: "A dismissal says only that somebody looked at the pair. It is stored as a\n" +
			"verdict under .requiem/verdicts/, not as a relationship, so dismissals do\n" +
			"not accumulate in the graph people read to understand how decisions fit\n" +
			"together — one real corpus collected 75 such edges.\n\n" +
			"Real findings still belong in the graph: use `link --type conflicts_with`\n" +
			"or `--type duplicates` for those.\n\n" +
			"--restore removes a dismissal, returning the pair to the audit queue.",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := openService()
			if err != nil {
				return err
			}
			if restore {
				if err := svc.RestorePair(args[0], args[1]); err != nil {
					return err
				}
				return printJSON(map[string]string{"restored_a": args[0], "restored_b": args[1]})
			}
			v, err := svc.DismissPair(args[0], args[1], note)
			if err != nil {
				return err
			}
			return printJSON(v)
		},
	}

	cmd.Flags().StringVar(&note, "note", "", "why the pair is unrelated")
	cmd.Flags().BoolVar(&restore, "restore", false, "remove the dismissal instead, returning the pair to the queue")

	return cmd
}
