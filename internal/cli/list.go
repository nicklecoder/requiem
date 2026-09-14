package cli

import (
	"github.com/spf13/cobra"

	"github.com/nicklecoder/requiem/internal/requiem"
)

func newListCmd() *cobra.Command {
	var namespace, kind, status, tag string
	var needsEmbedding, unreferenced, direct bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List compact statement summaries matching filters",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := openService()
			if err != nil {
				return err
			}
			summaries, err := svc.List(requiem.ListFilter{
				Namespace:      namespace,
				Kind:           kind,
				Status:         status,
				Tag:            tag,
				NeedsEmbedding: needsEmbedding,
				Unreferenced:   unreferenced,
				Direct:         direct,
			})
			if err != nil {
				return err
			}
			if summaries == nil {
				summaries = []requiem.StatementSummary{}
			}
			return printJSON(summaries)
		},
	}

	cmd.Flags().StringVar(&namespace, "namespace", "", "filter by namespace")
	cmd.Flags().StringVar(&kind, "kind", "", "filter by kind")
	cmd.Flags().StringVar(&status, "status", "", "filter by status")
	cmd.Flags().StringVar(&tag, "tag", "", "filter by tag")
	cmd.Flags().BoolVar(&needsEmbedding, "needs-embedding", false, "only statements with a missing or stale embedding")
	cmd.Flags().BoolVar(&unreferenced, "unreferenced", false, "only statements no labelled code site references (empty where labelling is unused)")
	cmd.Flags().BoolVar(&direct, "direct", false, "with --unreferenced, ignore coverage inherited from refining statements")

	return cmd
}
