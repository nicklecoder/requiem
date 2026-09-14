package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// newManCmd writes troff man pages generated from the command tree.
//
// The troff is emitted directly rather than through cobra/doc, which would
// pull go-md2man into the shipped binary purely to support a generator most
// users never run. Reading cobra's own metadata keeps the pages and
// `--help` in sync by construction: there is no second copy of the text.
func newManCmd() *cobra.Command {
	var dir string

	cmd := &cobra.Command{
		Use:   "man",
		Short: "Generate man pages for requiem and its subcommands",
		Long: "Writes one troff page per command into --dir, ready to install anywhere on\n" +
			"MANPATH. Generated from the same help text `requiem --help` shows, so the\n" +
			"two cannot drift apart.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return err
			}
			// Built from a fresh tree, not cmd.Root(), so the pages describe
			// every command rather than only those available in whatever
			// directory this happened to run in.
			root := NewRootCmd()
			written := []string{}
			for _, c := range append([]*cobra.Command{root}, root.Commands()...) {
				if c != root && (c.Hidden || c.Name() == "completion" || c.Name() == "help") {
					continue
				}
				name := "requiem"
				if c != root {
					name = "requiem-" + c.Name()
				}
				path := filepath.Join(dir, name+".1")
				if err := os.WriteFile(path, []byte(troff(c, name)), 0o644); err != nil {
					return err
				}
				written = append(written, filepath.Base(path))
			}
			return printJSON(map[string]interface{}{"dir": dir, "pages": written})
		},
	}
	cmd.Flags().StringVar(&dir, "dir", "man", "directory to write man pages into")
	return cmd
}

// troff renders one command. Deliberately small: a heading, the synopsis, the
// long description, flags, and for the root a command list.
func troff(c *cobra.Command, name string) string {
	var b strings.Builder
	fmt.Fprintf(&b, ".TH %s 1 %q \"requiem\" \"Requiem Manual\"\n",
		strings.ToUpper(strings.ReplaceAll(name, "-", " ")), time.Now().UTC().Format("2006-01-02"))

	fmt.Fprintf(&b, ".SH NAME\n%s \\- %s\n", name, escape(c.Short))

	fmt.Fprintf(&b, ".SH SYNOPSIS\n.B %s\n", escape(c.UseLine()))

	if body := strings.TrimSpace(c.Long); body != "" {
		b.WriteString(".SH DESCRIPTION\n")
		// A blank line becomes a paragraph break; everything else is emitted
		// verbatim in a no-fill block, so the indentation and short lines the
		// help text relies on survive.
		for _, para := range strings.Split(body, "\n\n") {
			b.WriteString(".PP\n.nf\n" + escape(strings.TrimRight(para, "\n")) + "\n.fi\n")
		}
	}

	if c.HasAvailableLocalFlags() {
		b.WriteString(".SH OPTIONS\n")
		c.LocalFlags().VisitAll(func(f *pflag.Flag) {
			if f.Hidden {
				return
			}
			spec := "\\-\\-" + f.Name
			if f.Shorthand != "" {
				spec = "\\-" + f.Shorthand + ", " + spec
			}
			if f.Value.Type() != "bool" {
				spec += " \\fI" + f.Value.Type() + "\\fR"
			}
			fmt.Fprintf(&b, ".TP\n.B %s\n%s\n", spec, escape(f.Usage))
		})
	}

	if c.HasAvailableSubCommands() {
		b.WriteString(".SH COMMANDS\n")
		for _, sub := range c.Commands() {
			if sub.Hidden || !sub.IsAvailableCommand() {
				continue
			}
			fmt.Fprintf(&b, ".TP\n.B %s\n%s\n", escape(sub.Name()), escape(sub.Short))
		}
		b.WriteString(".PP\nSee \\fBrequiem\\-<command>\\fR(1) for each.\n")
	}

	b.WriteString(".SH FILES\n.TP\n.B .requiem/statements/\nStatement files, canonical and git-tracked.\n" +
		".TP\n.B .requiem/config.yaml\nEmbedding endpoint and model; committed, because the index is only disposable if the pipeline that rebuilds it is tracked.\n" +
		".TP\n.B .requiem/index.sqlite\nDisposable cache, gitignored; rebuilt by \\fBrequiem reindex\\fR.\n")
	return b.String()
}

// escape neutralises the two characters troff treats specially at the start
// of a line, plus the escape character itself.
func escape(s string) string {
	s = strings.ReplaceAll(s, `\`, `\e`)
	out := make([]string, 0, 8)
	for _, line := range strings.Split(s, "\n") {
		if strings.HasPrefix(line, ".") || strings.HasPrefix(line, "'") {
			line = `\&` + line
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}
