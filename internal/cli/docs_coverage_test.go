package cli

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"

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
	"init":             "run once by a human before an agent sees the project; the block exists because init ran, so naming it is circular",
	"precommit-notice": "invoked by the pre-commit hook init installs, never by a person or an agent; it is plumbing, not a verb",
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

// Offering a capability that can only fail is worse than not offering it: an
// agent reading --help has no other way to tell whether --semantic will work.
func TestSemanticSurface_HiddenWithoutInference(t *testing.T) {
	find := func(root *cobra.Command, name string) *cobra.Command {
		for _, c := range root.Commands() {
			if c.Name() == name {
				return c
			}
		}
		return nil
	}

	for _, tc := range []struct{ semantic, wantHidden bool }{{false, true}, {true, false}} {
		audit := find(&cobra.Command{}, "audit")
		_ = audit
		root := cobra.Command{}
		root.AddCommand(newAuditCmd(tc.semantic), newCheckCmd(tc.semantic), newReindexCmd(tc.semantic))

		if got := find(&root, "audit").Hidden; got != tc.wantHidden {
			t.Errorf("semantic=%v: audit hidden=%v, want %v", tc.semantic, got, tc.wantHidden)
		}
		check := find(&root, "check")
		if got := check.Flags().Lookup("semantic").Hidden; got != tc.wantHidden {
			t.Errorf("semantic=%v: --semantic hidden=%v, want %v", tc.semantic, got, tc.wantHidden)
		}
		// --vector needs no endpoint and must stay available either way.
		if check.Flags().Lookup("vector").Hidden {
			t.Errorf("semantic=%v: --vector must never be hidden; it needs no endpoint", tc.semantic)
		}
		if got := find(&root, "reindex").Flags().Lookup("embed").Hidden; got != tc.wantHidden {
			t.Errorf("semantic=%v: --embed hidden=%v, want %v", tc.semantic, got, tc.wantHidden)
		}
	}
}

// A hidden command that silently succeeds would be the failure this project
// exists to prevent; it must say why it cannot work and how to enable it.
func TestAudit_ExplainsItselfWhenUnavailable(t *testing.T) {
	cmd := newAuditCmd(false)
	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected an error explaining why audit is unavailable")
	}
	for _, want := range []string{"embedding.endpoint", "config.yaml", "reindex --embed"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should mention %q, got: %v", want, err)
		}
	}
}
