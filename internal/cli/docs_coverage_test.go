package cli

import (
	"strings"
	"testing"

	"github.com/nicklecoder/requiem/internal/requiem"
)

// undocumentedCommands are deliberately absent from the agent doc block, with
// the reason recorded here rather than left implicit.
//
// The block competes for context budget with everything else in a startup
// file, so not every command belongs in it. The point of this map is that
// leaving one out becomes a decision someone wrote down, instead of an
// oversight nobody noticed.
var undocumentedCommands = map[string]string{
	"init": "run once by a human before an agent sees the project; the block exists because init ran, so naming it is circular",
}

// The doc block is the only thing telling an agent this tool exists, and it
// has gone stale three times — once for the embedding pipeline, once for
// check --semantic, once for traceability. Each time the marker versioning
// worked correctly and refreshed a block whose content had quietly stopped
// being true. String assertions catch regressions of what someone thought to
// list; only walking the command tree catches a new command shipping
// undocumented.
func TestAgentDocBlock_KeepsPaceWithTheCommandTree(t *testing.T) {
	block := requiem.AgentDocBlock()

	for _, cmd := range NewRootCmd().Commands() {
		name := cmd.Name()
		if reason, exempt := undocumentedCommands[name]; exempt {
			if reason == "" {
				t.Errorf("%q is exempt with no reason recorded", name)
			}
			continue
		}
		if !strings.Contains(block, "requiem "+name) && !strings.Contains(block, "`"+name+"`") {
			t.Errorf("command %q is not mentioned in the agent doc block; document it, or add it to undocumentedCommands with a reason", name)
		}
	}
}

// The inverse: an exemption for a command that no longer exists is a stale
// note that would silently excuse a real gap if the name were ever reused.
func TestUndocumentedCommands_HasNoStaleEntries(t *testing.T) {
	live := map[string]bool{}
	for _, cmd := range NewRootCmd().Commands() {
		live[cmd.Name()] = true
	}
	for name := range undocumentedCommands {
		if !live[name] {
			t.Errorf("%q is exempted but no longer a command", name)
		}
	}
}
