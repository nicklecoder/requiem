package cli

import (
	"github.com/spf13/cobra"

	"github.com/nicklecoder/requiem/internal/index"
)

func newAuditCmd(semantic bool) *cobra.Command {
	var namespace string
	var minScore float64
	var neighbors, limit int

	cmd := &cobra.Command{
		Use:   "audit",
		Short: "Sweep the corpus for candidate conflicting/duplicate statement pairs",
		Long: "Takes each statement's nearest neighbours as candidates and ranks them by\n" +
			"CSLS, skipping any pair that already has a relationship recorded between\n" +
			"them. Requiem surfaces the\n" +
			"candidate only — classifying a pair as a real conflict, a duplicate, or a\n" +
			"false positive is the calling agent's job, recorded afterward via `link\n" +
			"--type conflicts_with|duplicates|not_related`.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := openService()
			if err != nil {
				return err
			}
			candidates, progress, coverage, err := svc.Audit(namespace, neighbors, limit, minScore)
			if err != nil {
				return err
			}
			warnCoverage(coverage)
			warnAuditBacklog(len(candidates), progress)
			contradicting, err := svc.AuditRefs()
			if err != nil {
				return err
			}
			warnContradictingRefs(contradicting)
			misses, err := svc.NearMisses()
			if err != nil {
				return err
			}
			warnNearMisses(misses)
			dangling, err := svc.DanglingPointers()
			if err != nil {
				return err
			}
			warnDanglingPointers(dangling)
			if candidates == nil {
				candidates = []index.PairCandidate{}
			}
			return printJSON(candidates)
		},
	}

	// Audit compares vectors and can do nothing without them.
	cmd.Hidden = !semantic
	if !semantic {
		cmd.Long += unavailableNote
		cmd.RunE = func(*cobra.Command, []string) error {
			return errNoInference("audit")
		}
	}

	cmd.Flags().StringVar(&namespace, "namespace", "", "scope the sweep to this namespace (and anything nested under it)")
	cmd.Flags().IntVar(&neighbors, "neighbors", 2, "candidates to take from each statement's nearest others")
	// Off by default. An absolute cutoff fails on a real corpus: measured on
	// a 36-statement set, 0.5 surfaced 69% of all pairs while 0.85 — the
	// usual near-duplicate cutoff in information retrieval — found none of
	// five planted duplicates. Kept as a blunt floor for anyone who wants one.
	cmd.Flags().Float64Var(&minScore, "min-score", 0, "optional hard floor on raw cosine similarity (0 = no floor)")
	cmd.Flags().IntVar(&limit, "limit", 50, "maximum number of pairs to return (0 = unlimited)")

	return cmd
}
