package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/nicklecoder/requiem/internal/index"
	"github.com/nicklecoder/requiem/internal/model"
	"github.com/nicklecoder/requiem/internal/requiem"
	"github.com/nicklecoder/requiem/internal/store"
)

// rejectionView is a rejection as get prints it. source_kind is the field
// check already uses to tell the two record kinds apart, so a caller that
// fetched a candidate can branch on the same key it was handed.
type rejectionView struct {
	Project    string `json:"project,omitempty"`
	SourceKind string `json:"source_kind"`
	FullID     string `json:"full_id"`
	model.Rejection
}

func newGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <namespace/id>",
		Short: "Fetch a full statement or rejection, including resolved relationships and staleness",
		Long: "Fetches one record by its namespace/id. A record from a related project\n" +
			"(configured under related: in .requiem/config.local.yaml) is addressed as\n" +
			"<project>:<namespace>/<id>, the form check --related reports; ids inside\n" +
			"it are that project's own.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := openService()
			if err != nil {
				return err
			}
			id, project := args[0], ""
			// requiem: retrieval/related-ids-prefixed
			if name, rest, ok := requiem.SplitRelatedID(id); ok {
				rp, err := svc.RelatedProjectByName(name)
				if err != nil {
					return err
				}
				svc, id, project = rp.Open(), rest, rp.Name
			}
			st, err := svc.Get(id)
			if err == nil {
				st.Project = project
				return printJSON(st)
			}
			if !errors.Is(err, index.ErrNotFound) {
				return err
			}
			// requiem: cli/get-reads-rejections
			r, rerr := svc.GetRejection(id)
			if rerr != nil {
				if errors.Is(rerr, store.ErrNotFound) {
					if project != "" {
						return fmt.Errorf("%s:%s: not found in related project %s", project, id, project)
					}
					return err
				}
				return rerr
			}
			return printJSON(rejectionView{Project: project, SourceKind: "rejection", FullID: r.FullID(), Rejection: *r})
		},
	}
}
