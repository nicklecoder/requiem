package cli

import "github.com/spf13/cobra"

func newDiscardCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "discard [namespace/id]",
		Short: "Unstage and revert pending changes; all pending if no id given",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var fullID string
			if len(args) == 1 {
				fullID = args[0]
			}
			svc, err := openService()
			if err != nil {
				return err
			}
			res, err := svc.Discard(fullID)
			if err != nil {
				return err
			}
			return printJSON(res)
		},
	}
}
