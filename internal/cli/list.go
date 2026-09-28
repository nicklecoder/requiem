package cli

import (
	"github.com/spf13/cobra"

	"github.com/nicklecoder/requiem/internal/requiem"
)

func newListCmd() *cobra.Command {
	var namespace, kind, status, tag string
	var needsEmbedding, unreferenced, direct, abstract, openWording, bodies, unconfirmedEdges bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List compact statement summaries matching filters",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := openService()
			if err != nil {
				return err
			}
			if unconfirmedEdges {
				edges, err := svc.UnconfirmedEdges(namespace)
				if err != nil {
					return err
				}
				if edges == nil {
					edges = []requiem.Edge{}
				}
				return printJSON(edges)
			}
			if openWording {
				items, err := svc.OpenWordingQueue(namespace, bodies)
				if err != nil {
					return err
				}
				if items == nil {
					items = []requiem.OpenWordingItem{}
				}
				return printJSON(items)
			}
			summaries, err := svc.List(requiem.ListFilter{
				Namespace:      namespace,
				Kind:           kind,
				Status:         status,
				Tag:            tag,
				NeedsEmbedding: needsEmbedding,
				Unreferenced:   unreferenced,
				Direct:         direct,
				Abstract:       abstract,
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
	cmd.Flags().BoolVar(&openWording, "open-wording", false,
		"rank active statements by how likely their body leaves the decision open, skipping cleared ones (needs a classifier)")
	cmd.Flags().BoolVar(&bodies, "bodies", false, "with --open-wording, carry each full body instead of an excerpt")
	cmd.Flags().BoolVar(&unconfirmedEdges, "unconfirmed-edges", false,
		"list edges applied through batch that no one has reviewed; confirm one by linking it again, or remove it with unlink")
	cmd.Flags().BoolVar(&needsEmbedding, "needs-embedding", false, "only active or proposed statements with a missing or stale embedding")
	cmd.Flags().BoolVar(&unreferenced, "unreferenced", false, "only statements no labelled code site references (empty where labelling is unused)")
	cmd.Flags().BoolVar(&direct, "direct", false, "with --unreferenced, ignore coverage inherited from refining statements")
	// The declaration is an unverifiable author assertion that quiets
	// --unreferenced, so it has to be reviewable in bulk.
	// requiem: traceability/abstract-declarations-are-reviewable
	cmd.Flags().BoolVar(&abstract, "abstract", false, "only statements declared unimplementable by their author")

	return cmd
}
