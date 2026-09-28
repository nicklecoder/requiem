package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/nicklecoder/requiem/internal/index"
	"github.com/nicklecoder/requiem/internal/requiem"
)

func newAuditCmd(semantic bool) *cobra.Command {
	var namespace string
	var minScore float64
	var neighbors, limit int
	var bodies bool

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
			res, err := svc.AuditOrdered(namespace, neighbors, limit, minScore, func(n int, classifier string) {
				fmt.Fprintf(os.Stderr, "requiem: scoring %d pair(s) with %s\n", n, classifier)
			})
			if err != nil {
				return err
			}
			candidates := res.Pairs
			warnCoverage(res.Coverage)
			warnAuditBacklog(len(candidates), res.Progress)
			warnAuditOrdering(res.Ordering)
			if edges, err := svc.UnconfirmedEdges(namespace); err == nil && len(edges) > 0 {
				fmt.Fprintf(os.Stderr, "requiem: %d edge(s) applied through batch are unreviewed; see `requiem list --unconfirmed-edges`\n", len(edges)) // requiem:ignore message text, not a label
			}
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
			if bodies {
				pairs, err := svc.WithBodies(candidates)
				if err != nil {
					return err
				}
				// JSON Lines, the shape batch reads, so the verdicts go
				// straight back as one dismiss or link record per pair.
				enc := json.NewEncoder(os.Stdout)
				for _, p := range pairs {
					if err := enc.Encode(p); err != nil {
						return fmt.Errorf("encode output: %w", err)
					}
				}
				return nil
			}
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
	cmd.Flags().BoolVar(&bodies, "bodies", false, "emit JSON Lines, one pair per line, with both full bodies in place of excerpts")

	return cmd
}

// warnAuditOrdering says on stderr whether a classifier ordered the queue.
// A classifier that failed is reported, never fatal: audit still answers in
// its usual order (principles/models-are-optional).
func warnAuditOrdering(o requiem.AuditOrdering) {
	switch {
	case o.Classifier == "" && o.Error == "":
		return
	case o.Error != "":
		fmt.Fprintf(os.Stderr, "requiem: %s; queue shown in its usual order\n", o.Error)
	default:
		fmt.Fprintf(os.Stderr, "requiem: queue ordered by %s, likeliest contradictions first (%d scored, %d cached)\n", o.Classifier, o.Scored, o.Cached) // requiem:ignore message text, not a label
	}
}
