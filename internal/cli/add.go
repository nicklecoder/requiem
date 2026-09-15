package cli

import (
	"github.com/spf13/cobra"

	"github.com/nicklecoder/requiem/internal/requiem"
)

func newAddCmd() *cobra.Command {
	var id, namespace, kind, modality, status, body, provenance, source string
	var abstract, duplicateOk bool
	var tags []string

	cmd := &cobra.Command{
		Use:   "add",
		Short: "Create a new statement",
		Long: "Runs check against the body first and refuses to write when the corpus\n" +
			"already carries a record that reads as a duplicate, naming what it found.\n" +
			"--duplicate-ok records it anyway.\n\n" +
			"The check is lexical and identifier-based, never a network call, so a\n" +
			"duplicate worded in vocabulary this draft does not share can still get\n" +
			"through; `check --semantic` is how to find that one.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := openService()
			if err != nil {
				return err
			}
			st, err := svc.Add(requiem.AddParams{
				ID:          id,
				Namespace:   namespace,
				Kind:        kind,
				Modality:    modality,
				Status:      status,
				Abstract:    abstract,
				Body:        body,
				Tags:        tags,
				Provenance:  provenance,
				Source:      source,
				DuplicateOk: duplicateOk,
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
	cmd.Flags().BoolVar(&abstract, "abstract", false, "no code can implement this statement (a principle, a process decision); excludes it from list --unreferenced")
	cmd.Flags().StringVar(&status, "status", "", "proposed for a decision under consideration (default: active)")
	cmd.Flags().StringVar(&modality, "modality", "", "normative strength: must, should, may, must_not, should_not (optional)")
	cmd.Flags().StringVar(&body, "body", "", "statement body text (required)")
	cmd.Flags().StringSliceVar(&tags, "tags", nil, "comma-separated tags")
	cmd.Flags().StringVar(&provenance, "provenance", "dialogue", "dialogue or code-derived")
	cmd.Flags().StringVar(&source, "source", "", "file:line-line, required when --provenance=code-derived")
	cmd.Flags().BoolVar(&duplicateOk, "duplicate-ok", false, "record this even though an existing record reads as a duplicate")
	_ = cmd.MarkFlagRequired("id")
	_ = cmd.MarkFlagRequired("namespace")
	_ = cmd.MarkFlagRequired("kind")
	_ = cmd.MarkFlagRequired("body")

	return cmd
}
