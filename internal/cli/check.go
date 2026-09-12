package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/nicklecoder/requiem/internal/index"
)

func newCheckCmd() *cobra.Command {
	var namespace, text, vectorJSON string
	var tags []string

	cmd := &cobra.Command{
		Use:   "check",
		Short: "Surface compact candidate statements/rejections relevant to a draft idea",
		Long: "Lexical (FTS) matching alone misses a prior statement worded completely\n" +
			"differently. Pass --vector (a query embedding the agent computed itself —\n" +
			"requiem never computes one) to also search by meaning; results from either\n" +
			"path are merged and marked via match_kind.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			var vec []float32
			if vectorJSON != "" {
				if err := json.Unmarshal([]byte(vectorJSON), &vec); err != nil {
					return fmt.Errorf("--vector: expected a JSON array of numbers: %w", err)
				}
			}

			svc, err := openService()
			if err != nil {
				return err
			}
			candidates, err := svc.Check(namespace, text, tags, vec)
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
	_ = cmd.MarkFlagRequired("namespace")
	_ = cmd.MarkFlagRequired("text")

	return cmd
}
