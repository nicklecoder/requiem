package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/nicklecoder/requiem/internal/index"
)

func newCheckCmd() *cobra.Command {
	var namespace, text, vectorJSON, model string
	var tags []string
	var limit int

	cmd := &cobra.Command{
		Use:   "check",
		Short: "Surface compact candidate statements/rejections relevant to a draft idea",
		Long: "Lexical (FTS) matching alone misses a prior statement worded completely\n" +
			"differently. Pass --vector (a query embedding the agent computed itself —\n" +
			"requiem never computes one) together with --model to also search by\n" +
			"meaning; results from either path are merged and marked via match_kind.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			var vec []float32
			if vectorJSON != "" {
				if err := json.Unmarshal([]byte(vectorJSON), &vec); err != nil {
					return fmt.Errorf("--vector: expected a JSON array of numbers: %w", err)
				}
				// Caught here rather than deeper down so the message names
				// the flag the caller has to add, not the corpus state.
				if model == "" {
					return fmt.Errorf("--model is required with --vector: it names the model that produced the query vector, which must match the one the corpus is embedded with")
				}
			}

			svc, err := openService()
			if err != nil {
				return err
			}
			candidates, err := svc.Check(namespace, text, tags, vec, model, limit)
			if err != nil {
				return err
			}
			if candidates == nil {
				candidates = []index.Candidate{}
			}
			return printJSON(candidates)
		},
	}

	cmd.Flags().StringVar(&namespace, "namespace", "", "namespace to check within (required)")
	cmd.Flags().StringVar(&text, "text", "", "draft text to check for related/conflicting statements (required)")
	cmd.Flags().StringSliceVar(&tags, "tags", nil, "comma-separated tags to narrow the search")
	cmd.Flags().StringVar(&vectorJSON, "vector", "", "JSON array of floats: an embedding of --text, for semantic matching alongside lexical")
	cmd.Flags().StringVar(&model, "model", "", "name of the embedding model that produced --vector (required with it)")
	cmd.Flags().IntVar(&limit, "limit", index.DefaultCheckLimit, "maximum candidates to return (0 = unlimited)")
	_ = cmd.MarkFlagRequired("namespace")
	_ = cmd.MarkFlagRequired("text")

	return cmd
}
