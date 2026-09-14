package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/nicklecoder/requiem/internal/requiem"
)

func newReindexCmd() *cobra.Command {
	var withEmbed, force bool

	cmd := &cobra.Command{
		Use:   "reindex",
		Short: "Rebuild the SQLite index from statement files on disk",
		Long: "With --embed, also fills in every missing or stale embedding vector by\n" +
			"calling the endpoint configured in .requiem/config.yaml. That is kept\n" +
			"separate from a plain reindex because it is the one indexing operation\n" +
			"that reaches the network: a bare reindex stays fast, offline, and safe to\n" +
			"run from a git hook.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if force && !withEmbed {
				return fmt.Errorf("--force only applies with --embed")
			}

			svc, err := openService()
			if err != nil {
				return err
			}
			stats, err := svc.Reindex()
			if err != nil {
				return err
			}

			// Reported here because reindex is what scans the tree, and its
			// nonzero exit lets CI catch a typo on every push rather than
			// whenever someone remembers to run audit.
			misses, err := svc.NearMisses()
			if err != nil {
				return err
			}
			warnNearMisses(misses)

			var embedErr error
			if withEmbed {
				res, err := svc.EmbedAll(force)
				if err != nil {
					return err
				}
				embedErr = reportEmbedResult(res)
			}

			if err := printJSON(stats); err != nil {
				return err
			}
			if len(misses) > 0 && embedErr == nil {
				return fmt.Errorf("%d label(s) do not resolve to a statement", len(misses))
			}
			// Reported after stdout so the machine-readable payload is never
			// withheld by a partial failure — the caller gets the index
			// stats either way, and learns about the shortfall from the exit
			// code and the stderr summary above.
			return embedErr
		},
	}

	cmd.Flags().BoolVar(&withEmbed, "embed", false, "also compute missing/stale embeddings via the configured endpoint")
	cmd.Flags().BoolVar(&force, "force", false, "re-embed the whole corpus under the configured model, discarding every existing vector")

	return cmd
}

// reportEmbedResult writes the human-readable summary to stderr, keeping
// stdout bare JSON per the CLI output convention, and returns a non-nil
// error when anything failed so the process exits nonzero. A partial run
// that exited 0 would let a caller believe the corpus is fully embedded and
// then trust an audit built on part of it.
func reportEmbedResult(res *requiem.EmbedAllResult) error {
	if res.Repinned {
		fmt.Fprintln(os.Stderr, "requiem: re-pinned the corpus to a new model; every previous vector was discarded")
	}
	fmt.Fprintf(os.Stderr, "requiem: embedded %d, skipped %d already fresh", res.Embedded, res.Skipped)
	if res.Failed == 0 {
		fmt.Fprintln(os.Stderr)
		return nil
	}
	fmt.Fprintf(os.Stderr, ", FAILED %d\n", res.Failed)
	for _, line := range res.GroupedFailures() {
		fmt.Fprintf(os.Stderr, "requiem:   %s\n", line)
	}
	fmt.Fprintln(os.Stderr, "requiem: re-run to retry only the failures — vectors already written are kept")
	return fmt.Errorf("%d statement(s) could not be embedded", res.Failed)
}
