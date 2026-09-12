package cli

import (
	"github.com/spf13/cobra"

	"github.com/nicklecoder/requiem/internal/requiem"
)

func newAddCmd() *cobra.Command {
	var id, namespace, kind, body, provenance, source string
	var tags []string

	cmd := &cobra.Command{
		Use:   "add",
		Short: "Create a new statement",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := openService()
			if err != nil {
				return err
			}
			st, err := svc.Add(requiem.AddParams{
				ID:         id,
				Namespace:  namespace,
				Kind:       kind,
				Body:       body,
				Tags:       tags,
				Provenance: provenance,
				Source:     source,
			})
			if err != nil {
				return err
			}
			return printJSON(st)
		},
	}

	cmd.Flags().StringVar(&id, "id", "", "namespace-relative slug, e.g. no-plaintext-tokens (required)")
	cmd.Flags().StringVar(&namespace, "namespace", "", "hierarchical namespace, e.g. auth/session (required)")
	cmd.Flags().StringVar(&kind, "kind", "", "statement kind, e.g. requirement, rule, design (required)")
	cmd.Flags().StringVar(&body, "body", "", "statement body text (required)")
	cmd.Flags().StringSliceVar(&tags, "tags", nil, "comma-separated tags")
	cmd.Flags().StringVar(&provenance, "provenance", "dialogue", "dialogue or code-derived")
	cmd.Flags().StringVar(&source, "source", "", "file:line-line, required when --provenance=code-derived")
	_ = cmd.MarkFlagRequired("id")
	_ = cmd.MarkFlagRequired("namespace")
	_ = cmd.MarkFlagRequired("kind")
	_ = cmd.MarkFlagRequired("body")

	return cmd
}
