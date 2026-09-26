package cli

import (
	"errors"

	"github.com/nicklecoder/requiem/internal/requiem"
	"github.com/spf13/cobra"
)

func newInitCmd() *cobra.Command {
	var local, shared bool
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Set up .requiem/ in the current project",
		Long: "Set up .requiem/ in the current project, or finish setting it up in a\n" +
			"fresh clone. Safe to re-run: it adds what is missing and leaves the rest.\n\n" +
			"The index and .requiem/config.local.yaml, the per-machine config that\n" +
			"holds the embedding endpoint, are gitignored. A config.yaml from an older\n" +
			"requiem that still carries an endpoint has it moved into the local file.\n\n" +
			"Agent instructions go to CLAUDE.local.md by default, excluded from git\n" +
			"through .git/info/exclude, because one contributor adopting requiem is no\n" +
			"reason to change every contributor's agent instructions. A block already\n" +
			"committed to AGENTS.md or CLAUDE.md is refreshed where it is; one that\n" +
			"was never committed is moved to CLAUDE.local.md.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if local && shared {
				return errors.New("--local and --shared are mutually exclusive")
			}
			opts := requiem.InitOptions{Docs: requiem.DocsAuto}
			switch {
			case local:
				opts.Docs = requiem.DocsLocal
			case shared:
				opts.Docs = requiem.DocsShared
			}
			svc, err := openService()
			if err != nil {
				return err
			}
			res, err := svc.Init(opts)
			if err != nil {
				return err
			}
			return printJSON(res)
		},
	}
	cmd.Flags().BoolVar(&local, "local", false,
		"move the agent instructions out of AGENTS.md/CLAUDE.md into CLAUDE.local.md")
	cmd.Flags().BoolVar(&shared, "shared", false,
		"write the agent instructions to AGENTS.md and CLAUDE.md, for a team that has adopted requiem")
	return cmd
}
