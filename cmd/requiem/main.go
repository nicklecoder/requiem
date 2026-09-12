// Command requiem is an agent-native CLI for tracking requirements, rules,
// and design decisions on a per-project basis. See SPEC.md for the design.
package main

import (
	"os"

	"github.com/nicklecoder/requiem/internal/cli"
)

func main() {
	os.Exit(cli.Execute())
}
