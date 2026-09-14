package cli

import (
	"fmt"
	"github.com/spf13/cobra"

	"github.com/nicklecoder/requiem/internal/requiem"
)

func newUpdateCmd() *cobra.Command {
	var body, status, modality string
	var abstract, notAbstract bool

	cmd := &cobra.Command{
		Use:   "update <namespace/id>",
		Short: "Edit an existing statement's body and/or status",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Tri-state: leaving both flags off must not clear an existing
			// declaration, so an update touching only the body is safe.
			params := requiem.UpdateParams{Body: body, Status: status, Modality: modality}
			switch {
			case abstract && notAbstract:
				return fmt.Errorf("--abstract and --no-abstract are mutually exclusive")
			case abstract:
				t := true
				params.Abstract = &t
			case notAbstract:
				f := false
				params.Abstract = &f
			}

			svc, err := openService()
			if err != nil {
				return err
			}
			st, err := svc.Update(args[0], params)
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
	cmd.Flags().BoolVar(&abstract, "abstract", false, "declare that no code can implement this statement")
	cmd.Flags().BoolVar(&notAbstract, "no-abstract", false, "withdraw an abstract declaration")
	cmd.Flags().StringVar(&modality, "modality", "", "set normative strength (must, should, may, must_not, should_not), or \"none\" to clear it")
	cmd.Flags().StringVar(&status, "status", "", "active, superseded, or deprecated")

	return cmd
}
