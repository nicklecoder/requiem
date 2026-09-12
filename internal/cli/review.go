package cli

import "github.com/spf13/cobra"

func newReviewCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "review",
		Short: "Describe currently staged, uncommitted changes under .requiem/",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := openService()
			if err != nil {
				return err
			}
			res, err := svc.Review()
			if err != nil {
				return err
			}
			return printJSON(res)
		},
	}
}
