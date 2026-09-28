package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newDismissCmd() *cobra.Command {
	var note string
	var restore, openWording bool

	cmd := &cobra.Command{
		Use:   "dismiss <namespace/id> <namespace/id> | dismiss <namespace/id> --open-wording",
		Short: "Record that an audit candidate pair was judged unrelated, so it stops resurfacing",
		Long: "A dismissal says only that somebody looked at the pair. It is stored as a\n" +
			"verdict under .requiem/verdicts/, not as a relationship, so dismissals do\n" +
			"not accumulate in the graph people read to understand how decisions fit\n" +
			"together — one real corpus collected 75 such edges.\n\n" +
			"Real findings still belong in the graph: use `link --type conflicts_with`\n" +
			"or `--type duplicates` for those.\n\n" +
			"--restore removes a dismissal, returning the pair to the audit queue.\n\n" +
			"With --open-wording and one id, clears a statement from the open-wording\n" +
			"review list (list --open-wording): its current body has been read and is\n" +
			"settled. The clearance is tied to that body and lapses if it changes.",
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := openService()
			if err != nil {
				return err
			}
			if openWording {
				if len(args) != 1 {
					return fmt.Errorf("--open-wording clears one statement: give exactly one id")
				}
				if restore {
					if err := svc.RestoreOpenWording(args[0]); err != nil {
						return err
					}
					return printJSON(map[string]string{"restored": args[0]})
				}
				c, err := svc.ClearOpenWording(args[0], note)
				if err != nil {
					return err
				}
				return printJSON(c)
			}
			if len(args) != 2 {
				return fmt.Errorf("dismiss takes two ids, or one with --open-wording")
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
	cmd.Flags().BoolVar(&openWording, "open-wording", false, "clear one statement from the open-wording review list")

	return cmd
}
