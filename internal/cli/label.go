package cli

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

func newLabelCmd() *cobra.Command {
	var comment string

	cmd := &cobra.Command{
		Use:   "label <namespace/id> <file>:<line>",
		Short: "Insert a marker comment linking code to a statement",
		Long: "Writes `<comment> requiem: <namespace/id>` on its own line above the given\n" +
			"line, matching its indentation and using the comment syntax for that file\n" +
			"type — including block syntax where a language has no line comment, so a\n" +
			"stylesheet or Markdown file is not given an invalid `//`.\n\n" +
			"The id is checked against the corpus first, so a typo is refused here\n" +
			"rather than surfacing later as a dangling reference. Running it twice on\n" +
			"the same line is a no-op.\n\n" +
			"The edit is left unstaged, like every other change requiem makes outside\n" +
			".requiem/, so it appears in `git diff` before anything is committed.",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			file, lineStr, ok := strings.Cut(args[1], ":")
			if !ok {
				return fmt.Errorf("expected <file>:<line>, got %q", args[1])
			}
			line, err := strconv.Atoi(lineStr)
			if err != nil {
				return fmt.Errorf("%q: line must be a number", args[1])
			}

			svc, err := openService()
			if err != nil {
				return err
			}
			res, err := svc.Label(args[0], file, line, comment)
			if err != nil {
				return err
			}
			fmt.Fprintf(os.Stderr, "requiem: labelled %s:%d — edit is UNSTAGED, review with `git diff`\n", res.File, res.Line)
			return printJSON(res)
		},
	}
	cmd.Flags().StringVar(&comment, "comment", "", "line-comment marker to use, for a file type requiem does not recognise")
	return cmd
}
