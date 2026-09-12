package cli

import (
	"github.com/spf13/cobra"

	"github.com/nicklecoder/requiem/internal/requiem"
)

func newUpdateCmd() *cobra.Command {
	var body, status string

	cmd := &cobra.Command{
		Use:   "update <namespace/id>",
		Short: "Edit an existing statement's body and/or status",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := openService()
			if err != nil {
				return err
			}
			st, err := svc.Update(args[0], requiem.UpdateParams{Body: body, Status: status})
			if err != nil {
				return err
			}
			return printJSON(st)
		},
	}

	cmd.Flags().StringVar(&body, "body", "", "new statement body text")
	cmd.Flags().StringVar(&status, "status", "", "active, superseded, or deprecated")

	return cmd
}
