package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/nicklecoder/requiem/internal/requiem"
)

func newBatchCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "batch",
		Short: "Apply many writes from JSON Lines on stdin, reporting each record's outcome",
		Long: "Reads one JSON object per line from stdin, each carrying an \"op\" of add,\n" +
			"reject, update or link plus that op's fields. Blank lines and # comments\n" +
			"are skipped.\n\n" +
			"  {\"op\":\"add\",\"namespace\":\"auth\",\"id\":\"hashed-tokens\",\"kind\":\"rule\",\"body\":\"...\"}\n" +
			"  {\"op\":\"reject\",\"namespace\":\"auth\",\"id\":\"sliding-expiry\",\"body\":\"...\"}\n" +
			"  {\"op\":\"link\",\"from\":\"auth/hashed-tokens\",\"to\":\"principles/least-privilege\",\"type\":\"refines\"}\n\n" +
			"A line that does not parse aborts the whole batch before anything is\n" +
			"written — that is a defect in the caller, identical on a retry. A write\n" +
			"requiem refuses (a duplicate body, a link to a missing statement) is a\n" +
			"finding about this corpus: it is reported against its own line and the\n" +
			"remaining records still apply. Exits nonzero if any record failed.\n\n" +
			"Writes stage but never commit, so `discard` backs the whole batch out if\n" +
			"you want all-or-nothing, and `review` shows what would be approved.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := openService()
			if err != nil {
				return err
			}
			results, err := svc.BatchApply(os.Stdin)
			if err != nil {
				return err
			}
			if err := printJSON(results); err != nil {
				return err
			}
			// Summary on stderr, payload on stdout, and a nonzero exit when
			// anything failed: a half-applied batch reporting success would
			// be trusted as complete.
			failed := requiem.BatchFailures(results)
			fmt.Fprintf(os.Stderr, "requiem: %d of %d record(s) applied\n", len(results)-failed, len(results))
			if failed > 0 {
				for _, r := range results {
					if !r.Applied {
						fmt.Fprintf(os.Stderr, "requiem:   line %d (%s) %s: %s\n", r.Line, r.Op, r.FullID, r.Error)
					}
				}
				return fmt.Errorf("%d record(s) did not apply", failed)
			}
			return nil
		},
	}
}
