package cli

import (
	"github.com/spf13/cobra"

	"github.com/nicklecoder/requiem/internal/model"
)

func newLinkCmd() *cobra.Command {
	var relType, note string

	cmd := &cobra.Command{
		Use:   "link <from-namespace/id> <to-namespace/id>",
		Short: "Add a typed relationship from one statement to another",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := openService()
			if err != nil {
				return err
			}
			st, err := svc.Link(args[0], args[1], model.RelationshipType(relType), note)
			if err != nil {
				return err
			}
			return printJSON(st)
		},
	}

	cmd.Flags().StringVar(&relType, "type", "", "conflicts_with, supersedes, depends_on, refines, duplicates, moved_to, or not_related (required)")
	cmd.Flags().StringVar(&note, "note", "", "optional note explaining the relationship")
	_ = cmd.MarkFlagRequired("type")

	return cmd
}
