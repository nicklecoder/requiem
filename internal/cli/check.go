package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/nicklecoder/requiem/internal/config"
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
	var namespace, text, vectorJSON, model, diffRev string
	var tags, touches []string
	var limit int
	var useSemantic, diff, staged bool

	cmd := &cobra.Command{
		Use:   "check",
		Short: "Surface compact candidate statements/rejections relevant to a draft idea",
		Long:  checkLong(semantic),
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			// --diff asks a different question — "what decisions bear on this
			// patch" — so it takes no draft text. A mature repository has a
			// diff where a new project has an intent document, and that is
			// the artifact worth checking against.
			// requiem: traceability/diff-scoped-check
			if diff || staged || diffRev != "" {
				if text != "" {
					return fmt.Errorf("--diff and --text ask different questions: one scopes to a patch, the other to a draft")
				}
				rev := diffRev
				if staged {
					rev = "--staged"
				}
				svc, err := openService()
				if err != nil {
					return err
				}
				res, err := svc.CheckDiff(rev)
				if err != nil {
					return err
				}
				if err := printJSON(res); err != nil {
					return err
				}
				return reportDiffGate(res)
			}
			if namespace == "" || text == "" {
				return fmt.Errorf("--namespace and --text are required (or use --diff to scope to a patch instead)")
			}

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
				Touches:   touches,
				Vector:    vec,
				Model:     model,
				Semantic:  useSemantic,
			})
			if err != nil {
				return err
			}
			warnCoverage(coverage)
			warnNothingMatched(candidates)
			if candidates == nil {
				candidates = []index.Candidate{}
			}
			return printJSON(candidates)
		},
	}

	cmd.Flags().StringVar(&namespace, "namespace", "", "namespace to check within (required)")
	cmd.Flags().StringVar(&text, "text", "", "draft text to check for related/conflicting statements (required)")
	cmd.Flags().StringSliceVar(&tags, "tags", nil, "comma-separated tags to narrow the search")
	cmd.Flags().StringSliceVar(&touches, "touches", nil, "comma-separated identifiers the change touches, e.g. external_venues.status")
	cmd.Flags().StringVar(&vectorJSON, "vector", "", "JSON array of floats: an embedding of --text, for semantic matching alongside lexical")
	cmd.Flags().StringVar(&model, "model", "", "name of the embedding model that produced --vector (required with it)")
	cmd.Flags().BoolVar(&useSemantic, "semantic", false, "also match by meaning, embedding --text via the configured endpoint")
	cmd.Flags().IntVar(&limit, "limit", index.DefaultCheckLimit, "maximum candidates to return (0 = unlimited)")
	cmd.Flags().BoolVar(&diff, "diff", false, "scope to the working tree's changes instead of a draft: which recorded decisions cover this patch")
	cmd.Flags().BoolVar(&staged, "staged", false, "like --diff, over the staged changes")
	cmd.Flags().StringVar(&diffRev, "diff-rev", "", "like --diff, over a revision or range (e.g. HEAD~3, main...HEAD)")
	// --semantic needs an endpoint to embed the query with; --vector does not
	// and stays available regardless.
	if !semantic {
		_ = cmd.Flags().MarkHidden("semantic")
	}
	// Not marked required at the cobra level any more: --diff is a valid
	// invocation with neither, and RunE reports the combination that is not.
	return cmd
}

// reportDiffGate writes the human-readable half of a --diff run and returns a
// nonzero-exit error when the configured gate says to fail.
//
// What it fails on is deliberately narrow: code labelled with a retired or
// rejected decision, and covering statements whose source range has drifted.
// Both are checkable facts. Failing because a change touches decisions the
// author may not have read would be a judgment about intent, which this tool
// refuses to make.
// requiem: traceability/diff-gate-is-per-project
func reportDiffGate(res *requiem.DiffCheck) error {
	findings := res.Findings()
	if len(res.Covering) > 0 || len(res.Rejections) > 0 {
		fmt.Fprintf(os.Stderr, "requiem: %d decision(s) and %d rejection(s) bear on this change\n",
			len(res.Covering), len(res.Rejections))
	}
	for _, f := range findings {
		fmt.Fprintf(os.Stderr, "requiem:   %s\n", f)
	}
	if res.Gate == config.GateOff || len(findings) == 0 {
		return nil
	}
	if res.Failing() {
		return fmt.Errorf("%d finding(s) contradict a recorded decision (gate: error)", len(findings))
	}
	fmt.Fprintln(os.Stderr, "requiem: reporting only (gate: warn)") // requiem:ignore message text, not a label
	return nil
}
