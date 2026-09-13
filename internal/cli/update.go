package cli

import (
	"github.com/spf13/cobra"

	"github.com/nicklecoder/requiem/internal/requiem"
)

func newUpdateCmd() *cobra.Command {
	var body, status, modality string

	cmd := &cobra.Command{
		Use:   "update <namespace/id>",
		Short: "Edit an existing statement's body and/or status",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := openService()
			if err != nil {
				return err
			}
			st, err := svc.Update(args[0], requiem.UpdateParams{Body: body, Status: status, Modality: modality})
			if err != nil {
				return err
			}
			// Reported on stderr so stdout stays bare JSON, and only when the
			// body changed: a status or modality edit does not invalidate
			// the code that implements the statement.
			if body != "" {
				refs, err := svc.UpdateBlastRadius(args[0])
				if err != nil {
					return err
				}
				warnBlastRadius(args[0], refs)
			}
			return printJSON(st)
		},
	}

	cmd.Flags().StringVar(&body, "body", "", "new statement body text")
	cmd.Flags().StringVar(&modality, "modality", "", "set normative strength (must, should, may, must_not, should_not), or \"none\" to clear it")
	cmd.Flags().StringVar(&status, "status", "", "active, superseded, or deprecated")

	return cmd
}
