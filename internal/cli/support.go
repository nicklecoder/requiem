package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/nicklecoder/requiem/internal/requiem"
)

// openService builds a Service rooted at the current working directory —
// requiem is run from the project root, the same convention as most git
// subcommands.
func openService() (*requiem.Service, error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	return requiem.Open(wd), nil
}

// printJSON writes v as indented JSON to stdout — the bare-data half of the
// CLI's output convention (see SPEC.md): success writes data to stdout,
// failure writes an error to stderr with a nonzero exit code, no wrapping
// {ok,data} envelope either way.
func printJSON(v interface{}) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return fmt.Errorf("encode output: %w", err)
	}
	return nil
}
