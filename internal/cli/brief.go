package cli

import (
	"github.com/spf13/cobra"

	"github.com/nicklecoder/requiem/internal/requiem"
)

func newBriefCmd() *cobra.Command {
	var namespace string
	var limit int

	cmd := &cobra.Command{
		Use:   "brief",
		Short: "The minimal set of decisions in force for a namespace — small enough to paste into a prompt",
		Long: "Returns the binding rules (must and must_not, prohibitions first), the\n" +
			"principles they refine, and what this project has already rejected.\n\n" +
			"A corpus of hundreds of statements has no way to say which ten matter\n" +
			"here, so an agent either loads everything or loads nothing. Each section\n" +
			"is capped, and whatever the cap left out is counted in `omitted` rather\n" +
			"than silently dropped.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := openService()
			if err != nil {
				return err
			}
			brief, err := svc.BriefFor(namespace, limit)
			if err != nil {
				return err
			}
			return printJSON(brief)
		},
	}

	cmd.Flags().StringVar(&namespace, "namespace", "", "scope the brief to this namespace (and anything nested under it)")
	cmd.Flags().IntVar(&limit, "limit", requiem.DefaultBriefLimit, "maximum records per section")

	return cmd
}
