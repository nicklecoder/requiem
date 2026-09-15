package cli

import (
	"fmt"
	"github.com/spf13/cobra"

	"github.com/nicklecoder/requiem/internal/requiem"
)

func newUpdateCmd() *cobra.Command {
	var body, status, modality, seeInstead string
	var abstract, notAbstract, rejection, clearSeeInstead bool

	cmd := &cobra.Command{
		Use:   "update <namespace/id>",
		Short: "Edit an existing statement's body and/or status",
		Long: "With --rejection, edits a rejection instead: its body, or where its\n" +
			"see_instead points. A rejection has no status or modality, so those flags\n" +
			"do not apply to one.\n\n" +
			"A rejection still stored in a legacy per-namespace _rejected.md is moved\n" +
			"to its own file as part of the edit, since rewriting one entry of a shared\n" +
			"file is the hazard the current layout removes.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if rejection {
				for _, bad := range []struct {
					name string
					set  bool
				}{
					{"--status", status != ""},
					{"--modality", modality != ""},
					{"--abstract", abstract},
					{"--no-abstract", notAbstract},
				} {
					if bad.set {
						return fmt.Errorf("%s does not apply to a rejection: a rejection records that an idea lost, and carries no lifecycle of its own", bad.name)
					}
				}
				if clearSeeInstead && seeInstead != "" {
					return fmt.Errorf("--see-instead and --clear-see-instead are mutually exclusive")
				}
				svc, err := openService()
				if err != nil {
					return err
				}
				r, err := svc.UpdateRejection(args[0], requiem.UpdateRejectionParams{
					Body:            body,
					SeeInstead:      seeInstead,
					ClearSeeInstead: clearSeeInstead,
				})
				if err != nil {
					return err
				}
				return printJSON(r)
			}
			if seeInstead != "" || clearSeeInstead {
				return fmt.Errorf("--see-instead applies to a rejection: pass --rejection with it")
			}
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
	cmd.Flags().BoolVar(&rejection, "rejection", false, "edit the rejection with this id rather than the statement")
	cmd.Flags().StringVar(&seeInstead, "see-instead", "", "with --rejection, re-point it at the statement adopted instead")
	cmd.Flags().BoolVar(&clearSeeInstead, "clear-see-instead", false, "with --rejection, remove its see_instead pointer")

	return cmd
}
