package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

func newEmbedCmd() *cobra.Command {
	var vectorJSON, model string
	var force, rejection bool

	cmd := &cobra.Command{
		Use:   "embed <namespace/id>",
		Short: "Store an agent-supplied embedding vector for a statement",
		Long: "Requiem never computes embeddings itself — it accepts a vector the agent\n" +
			"already produced (from whatever embedding model/provider it has access to)\n" +
			"and does cosine-similarity math over stored vectors for `audit`/`check`.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var vec []float32
			if err := json.Unmarshal([]byte(vectorJSON), &vec); err != nil {
				return fmt.Errorf("--vector: expected a JSON array of numbers: %w", err)
			}

			svc, err := openService()
			if err != nil {
				return err
			}
			res, err := svc.Embed(args[0], model, vec, force, rejection)
			if err != nil {
				return err
			}
			return printJSON(res)
		},
	}

	cmd.Flags().StringVar(&vectorJSON, "vector", "", "JSON array of floats, e.g. [0.01,-0.23,...] (required)")
	cmd.Flags().StringVar(&model, "model", "", "name of the embedding model that produced the vector (required)")
	cmd.Flags().BoolVar(&force, "force", false, "re-pin the whole corpus to this model/dims, wiping every existing embedding")
	cmd.Flags().BoolVar(&rejection, "rejection", false, "store the vector for the rejection with this id rather than the statement")
	_ = cmd.MarkFlagRequired("vector")
	_ = cmd.MarkFlagRequired("model")

	return cmd
}
