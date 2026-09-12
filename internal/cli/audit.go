package cli

import (
	"github.com/spf13/cobra"

	"github.com/nicklecoder/requiem/internal/index"
)

func newAuditCmd() *cobra.Command {
	var namespace string
	var minScore float64
	var limit int

	cmd := &cobra.Command{
		Use:   "audit",
		Short: "Sweep the corpus for candidate conflicting/duplicate statement pairs",
		Long: "Ranks statement pairs by embedding similarity, skipping any pair that\n" +
			"already has a relationship recorded between them. Requiem surfaces the\n" +
			"candidate only — classifying a pair as a real conflict, a duplicate, or a\n" +
			"false positive is the calling agent's job, recorded afterward via `link\n" +
			"--type conflicts_with|duplicates|not_related`.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := openService()
			if err != nil {
				return err
			}
			candidates, err := svc.Audit(namespace, minScore, limit)
			if err != nil {
				return err
			}
			if candidates == nil {
				candidates = []index.PairCandidate{}
			}
			return printJSON(candidates)
		},
	}

	cmd.Flags().StringVar(&namespace, "namespace", "", "scope the sweep to this namespace (and anything nested under it)")
	// 0.5 is calibrated against a small local model (Ollama's all-minilm,
	// 384-dim): a genuine paraphrase pair scored ~0.69, unrelated pairs
	// ~0.36-0.37 — small models compress cosine similarity into a narrower
	// band than one might assume, so don't reuse thresholds tuned for a
	// different model without recalibrating. Always overridable per call.
	cmd.Flags().Float64Var(&minScore, "min-score", 0.5, "minimum cosine similarity to surface a pair (tune for your embedding model)")
	cmd.Flags().IntVar(&limit, "limit", 50, "maximum number of pairs to return (0 = unlimited)")

	return cmd
}
