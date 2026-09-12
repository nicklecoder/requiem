package cli

import "github.com/spf13/cobra"

func newReindexCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "reindex",
		Short: "Rebuild the SQLite index from statement files on disk",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := openService()
			if err != nil {
				return err
			}
			stats, err := svc.Reindex()
			if err != nil {
				return err
			}
			return printJSON(stats)
		},
	}
}
