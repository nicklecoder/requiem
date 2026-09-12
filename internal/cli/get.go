package cli

import "github.com/spf13/cobra"

func newGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <namespace/id>",
		Short: "Fetch a full statement, including resolved relationships and staleness",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := openService()
			if err != nil {
				return err
			}
			st, err := svc.Get(args[0])
			if err != nil {
				return err
			}
			return printJSON(st)
		},
	}
}
