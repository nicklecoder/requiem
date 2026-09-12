package cli

import "github.com/spf13/cobra"

func newCommitCmd() *cobra.Command {
	var message string

	cmd := &cobra.Command{
		Use:   "commit",
		Short: "Commit staged .requiem/ changes — this is the approval step",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := openService()
			if err != nil {
				return err
			}
			res, err := svc.Commit(message)
			if err != nil {
				return err
			}
			return printJSON(res)
		},
	}

	cmd.Flags().StringVar(&message, "message", "", "commit message; a structured default is used if omitted")

	return cmd
}
