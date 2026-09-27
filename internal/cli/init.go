package cli

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/nicklecoder/requiem/internal/requiem"
	"github.com/spf13/cobra"
)

func newInitCmd() *cobra.Command {
	var local, shared, skipModels, saveLocal bool
	var endpoint, model string
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
			"was never committed is moved to CLAUDE.local.md.\n\n" +
			"It then connects the embedder, so requiem is fully working when init\n" +
			"returns. The model is the project's, committed: the one already named, else\n" +
			"--embedding-model, else the machine config's default. The endpoint is the\n" +
			"first that answers a real request with that model: --embedding-endpoint,\n" +
			"the configured one, the server OLLAMA_HOST names, then Ollama on localhost.\n" +
			"It never scans the network. A newly found endpoint is saved to the machine\n" +
			"config, so every project on this machine finds it (--save-local keeps it in\n" +
			"this project's overlay instead), and the corpus is embedded. At a terminal\n" +
			"it asks for what it cannot find; otherwise it never waits for input and\n" +
			"reports what is missing.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if local && shared {
				return errors.New("--local and --shared are mutually exclusive")
			}
			opts := requiem.InitOptions{Docs: requiem.DocsAuto, Models: requiem.ModelSetupOptions{
				Enabled:   !skipModels,
				Endpoint:  endpoint,
				Model:     model,
				SaveLocal: saveLocal,
				Prompt:    terminalPrompt(),
			}}
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
	cmd.Flags().StringVar(&endpoint, "embedding-endpoint", "",
		"embedding endpoint to use (a /v1/embeddings URL, a base URL, or host:port)")
	cmd.Flags().StringVar(&model, "embedding-model", "",
		"embedding model for a project that names none yet; committed to .requiem/config.yaml")
	cmd.Flags().BoolVar(&skipModels, "skip-models", false,
		"leave model setup alone; search stays lexical-only until an endpoint is configured")
	cmd.Flags().BoolVar(&saveLocal, "save-local", false,
		"save a newly found endpoint to this project's config.local.yaml instead of the machine config")
	return cmd
}

// terminalPrompt asks on stderr and reads a line from stdin, but only when
// both are terminals: an agent running init must never be left waiting on
// input nobody will type. Nil otherwise.
// requiem: cli/init-sets-up-models
func terminalPrompt() func(question, suggestion string) (string, error) {
	if !isTerminal(os.Stdin) || !isTerminal(os.Stderr) {
		return nil
	}
	in := bufio.NewReader(os.Stdin)
	return func(question, suggestion string) (string, error) {
		if suggestion != "" {
			fmt.Fprintf(os.Stderr, "%s [%s]: ", question, suggestion)
		} else {
			fmt.Fprintf(os.Stderr, "%s: ", question)
		}
		line, err := in.ReadString('\n')
		if err != nil && line == "" {
			return "", nil // end of input reads as a blank answer
		}
		if answer := strings.TrimSpace(line); answer != "" {
			return answer, nil
		}
		return suggestion, nil
	}
}

func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
