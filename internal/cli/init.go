package cli

import "github.com/spf13/cobra"

func newInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Set up .requiem/ in the current project",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := openService()
			if err != nil {
				return err
			}
			res, err := svc.Init()
			if err != nil {
				return err
			}
			return printJSON(res)
		},
	}
}
