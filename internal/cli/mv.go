package cli

import (
	"github.com/spf13/cobra"
)

func newMvCmd() *cobra.Command {
	var leaveLink bool

	cmd := &cobra.Command{
		Use:   "mv <from-namespace/id> <to-namespace/id>",
		Short: "Relocate a statement to a new namespace/id, rewriting inbound references",
		Long: "Every statement that held a relationship pointing at <from> has that\n" +
			"reference rewritten to <to>, so the graph doesn't silently break. With\n" +
			"--leave-link, a deprecated stub with a moved_to relationship is left at\n" +
			"the old location instead of deleting it outright.",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := openService()
			if err != nil {
				return err
			}
			res, err := svc.Move(args[0], args[1], leaveLink)
			if err != nil {
				return err
			}
			return printJSON(res)
		},
	}

	cmd.Flags().BoolVar(&leaveLink, "leave-link", false, "leave a deprecated stub at the old location, linked to the new one")

	return cmd
}
