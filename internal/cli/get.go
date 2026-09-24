package cli

import (
	"errors"

	"github.com/spf13/cobra"

	"github.com/nicklecoder/requiem/internal/index"
	"github.com/nicklecoder/requiem/internal/model"
	"github.com/nicklecoder/requiem/internal/store"
)

// rejectionView is a rejection as get prints it. source_kind is the field
// check already uses to tell the two record kinds apart, so a caller that
// fetched a candidate can branch on the same key it was handed.
type rejectionView struct {
	SourceKind string `json:"source_kind"`
	FullID     string `json:"full_id"`
	model.Rejection
}

func newGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <namespace/id>",
		Short: "Fetch a full statement or rejection, including resolved relationships and staleness",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := openService()
			if err != nil {
				return err
			}
			st, err := svc.Get(args[0])
			if err == nil {
				return printJSON(st)
			}
			if !errors.Is(err, index.ErrNotFound) {
				return err
			}
			// requiem: cli/get-reads-rejections
			r, rerr := svc.GetRejection(args[0])
			if rerr != nil {
				if errors.Is(rerr, store.ErrNotFound) {
					return err
				}
				return rerr
			}
			return printJSON(rejectionView{SourceKind: "rejection", FullID: r.FullID(), Rejection: *r})
		},
	}
}
