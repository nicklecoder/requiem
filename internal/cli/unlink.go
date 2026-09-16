package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/nicklecoder/requiem/internal/model"
)

// requiem: model/relationships-are-removable
func newUnlinkCmd() *cobra.Command {
	var relType string

	cmd := &cobra.Command{
		Use:   "unlink <from-namespace/id> <to-namespace/id>",
		Short: "Remove a recorded relationship between two statements",
		Long: "Takes back an edge the graph should no longer assert.\n\n" +
			"Without --type every relationship joining the pair is removed; with it,\n" +
			"only that one. A pair can carry two types, which is also why retyping is\n" +
			"`unlink` then `link` rather than a flag: a replacing link would have to\n" +
			"guess which of them you meant to destroy.\n\n" +
			"This is the command for an audit finding that turned out to be wrong.\n" +
			"`audit` skips any pair already carrying a relationship, so a stale\n" +
			"conflicts_with keeps the pair out of the queue for good.",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := openService()
			if err != nil {
				return err
			}
			res, err := svc.Unlink(args[0], args[1], model.RelationshipType(relType))
			if err != nil {
				return err
			}
			for _, t := range res.Removed {
				fmt.Fprintf(os.Stderr, "requiem: removed %s %s -> %s\n", t, args[0], args[1])
			}
			return printJSON(res.Statement)
		},
	}

	cmd.Flags().StringVar(&relType, "type", "", "remove only this relationship type; omit to remove every edge to the target")

	return cmd
}
