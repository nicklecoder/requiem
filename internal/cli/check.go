package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/nicklecoder/requiem/internal/index"
	"github.com/nicklecoder/requiem/internal/requiem"
)

// checkLong describes only what is actually available here: advertising
// --semantic where no endpoint is configured tells a reader to use something
// that can only fail.
func checkLong(semantic bool) string {
	base := "Lexical (FTS) matching alone misses a prior statement worded completely\n" +
		"differently.\n\n"
	if semantic {
		return base +
			"--semantic also searches by meaning, embedding the query text via the\n" +
			"endpoint in .requiem/config.yaml. Pass --vector with --model instead to\n" +
			"supply a query embedding you computed yourself. Results from either path\n" +
			"are merged and marked via match_kind.\n\n" +
			"--semantic is opt-in, not the default: check is the most-used command here\n" +
			"and stays fast and offline unless you ask for the network call."
	}
	return base +
		"Semantic matching is unavailable here: no embedding endpoint is configured\n" +
		"in .requiem/config.yaml and no vectors are stored. You can still pass\n" +
		"--vector with --model if you have an embedding from elsewhere, or configure\n" +
		"an endpoint (any OpenAI-compatible /v1/embeddings, including a local Ollama)\n" +
		"and re-run to enable --semantic."
}

func newCheckCmd(semantic bool) *cobra.Command {
	var namespace, text, vectorJSON, model string
	var tags []string
	var limit int
	var useSemantic bool

	cmd := &cobra.Command{
		Use:   "check",
		Short: "Surface compact candidate statements/rejections relevant to a draft idea",
		Long:  checkLong(semantic),
		Args:  cobra.NoArgs,
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
			candidates, coverage, err := svc.Check(requiem.CheckParams{
				Namespace: namespace,
				Text:      text,
				Tags:      tags,
				Limit:     limit,
				Vector:    vec,
				Model:     model,
				Semantic:  useSemantic,
			})
			if err != nil {
				return err
			}
			warnCoverage(coverage)
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
	cmd.Flags().BoolVar(&useSemantic, "semantic", false, "also match by meaning, embedding --text via the configured endpoint")
	cmd.Flags().IntVar(&limit, "limit", index.DefaultCheckLimit, "maximum candidates to return (0 = unlimited)")
	// --semantic needs an endpoint to embed the query with; --vector does not
	// and stays available regardless.
	if !semantic {
		_ = cmd.Flags().MarkHidden("semantic")
	}
	_ = cmd.MarkFlagRequired("namespace")
	_ = cmd.MarkFlagRequired("text")

	return cmd
}
