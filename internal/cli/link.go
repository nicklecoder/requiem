package cli

import (
	"github.com/spf13/cobra"

	"github.com/nicklecoder/requiem/internal/model"
)

func newLinkCmd() *cobra.Command {
	var relType, note string

	cmd := &cobra.Command{
		Use:   "link <from-namespace/id> <to-namespace/id>",
		Short: "Add a typed relationship from one statement to another, or update its note",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := openService()
			if err != nil {
				return err
			}
			res, err := svc.Link(args[0], args[1], model.RelationshipType(relType), note)
			if err != nil {
				return err
			}
			// Recording that A supersedes B leaves B active on purpose, so
			// the one thing the caller must not do is miss that it did.
			// requiem: model/supersedes-does-not-retire
			warnLink(res)
			return printJSON(res.Statement)
		},
	}

	cmd.Flags().StringVar(&relType, "type", "", "conflicts_with, supersedes, depends_on, refines, duplicates, or moved_to (required); dismiss an unrelated pair with `requiem dismiss`")
	cmd.Flags().StringVar(&note, "note", "", "optional note explaining the relationship")
	_ = cmd.MarkFlagRequired("type")

	return cmd
}
