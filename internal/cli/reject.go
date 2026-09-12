package cli

import (
	"github.com/spf13/cobra"

	"github.com/nicklecoder/requiem/internal/requiem"
)

func newRejectCmd() *cobra.Command {
	var id, namespace, body, seeInstead string

	cmd := &cobra.Command{
		Use:   "reject",
		Short: "Record an idea that was explicitly considered and rejected, so it isn't re-proposed later",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := openService()
			if err != nil {
				return err
			}
			r, err := svc.Reject(requiem.RejectParams{
				ID:         id,
				Namespace:  namespace,
				Body:       body,
				SeeInstead: seeInstead,
			})
			if err != nil {
				return err
			}
			return printJSON(r)
		},
	}

	cmd.Flags().StringVar(&id, "id", "", "namespace-relative slug (required)")
	cmd.Flags().StringVar(&namespace, "namespace", "", "hierarchical namespace, e.g. auth/session (required)")
	cmd.Flags().StringVar(&body, "body", "", "what was proposed and why it was rejected (required)")
	cmd.Flags().StringVar(&seeInstead, "see-instead", "", "namespace/id of the active statement that addresses the concern instead")
	_ = cmd.MarkFlagRequired("id")
	_ = cmd.MarkFlagRequired("namespace")
	_ = cmd.MarkFlagRequired("body")

	return cmd
}
