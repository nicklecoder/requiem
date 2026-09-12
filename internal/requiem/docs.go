package requiem

import (
	"os"
	"path/filepath"
	"strings"
)

// agentDocFiles are written/updated by Init so any agent whose harness
// loads one of these at session start learns requiem exists and how to use
// it without a human explaining it each time. AGENTS.md is the emerging
// cross-harness convention; CLAUDE.md is Claude Code's own.
var agentDocFiles = []string{"AGENTS.md", "CLAUDE.md"}

const (
	docMarkerBegin = "<!-- >>> requiem >>> -->"
	docMarkerEnd   = "<!-- <<< requiem <<< -->"
)

// agentDocBlock is deliberately short: it competes for context budget with
// everything else in a startup file, so it states the workflow, not the
// full reference — `requiem --help` is the reference.
const agentDocBlock = docMarkerBegin + `
## requiem

This project uses requiem to track requirements, rules, and design
decisions ("statements") under ` + "`.requiem/`" + ` (git-tracked; a disposable
SQLite index makes them queryable). Purpose: catch conflicts with prior
decisions before they're built, without needing the whole spec in context.

Before proposing something non-trivial, run ` + "`requiem check --namespace <area> --text \"<the idea>\"`" + ` —
it surfaces related prior statements *and* previously-rejected ideas,
ranked, excerpt-only. Record real decisions with ` + "`requiem add`" + `/` + "`update`" + `/` + "`link`" + `;
record ideas that were explicitly considered and rejected (not just
abandoned mid-thought) with ` + "`requiem reject`" + `, so they aren't re-proposed later.

Requiem never computes embeddings itself — if you have one available, pass
it to ` + "`requiem embed <namespace/id> --vector <json> --model <name>`" + ` so
` + "`check --vector`" + ` and ` + "`audit`" + ` can also match by meaning, not just
shared vocabulary (` + "`requiem list --needs-embedding`" + ` finds what's missing
or stale). Run ` + "`requiem audit`" + ` periodically to sweep for statements that
may conflict or duplicate each other; record the verdict with ` + "`link --type" + `
` + "`conflicts_with`" + `/` + "`duplicates`" + `/` + "`not_related`" + ` so the same pair doesn't resurface.
Relocate a statement (e.g. factoring a duplicate into a shared namespace)
with ` + "`requiem mv <from> <to> [--leave-link]`" + ` — it rewrites every inbound
reference so the graph doesn't silently break.

Every write auto-stages in git but never auto-commits — nothing is
permanent until ` + "`requiem commit`" + ` runs, so exploring dead ends leaves no
trace if you back out. Full command reference: ` + "`requiem --help`" + `.
` + docMarkerEnd + "\n"

// ensureAgentDocs writes agentDocBlock into each of agentDocFiles at root,
// appending after any existing content (never overwriting it) and skipping
// files that already carry the block — the same idempotent-chaining
// approach as git.Client.InstallHook, applied to plain text instead of
// shell scripts. Returns the files actually created or modified.
func ensureAgentDocs(root string) ([]string, error) {
	var touched []string
	for _, name := range agentDocFiles {
		path := filepath.Join(root, name)
		changed, err := appendMarkedBlock(path, agentDocBlock)
		if err != nil {
			return nil, err
		}
		if changed {
			touched = append(touched, name)
		}
	}
	return touched, nil
}

// appendMarkedBlock ensures block's content exists in the file at path,
// creating the file if needed or appending after existing content if not
// — never touching anything already there. No-op (returns changed=false)
// if the file already contains docMarkerBegin.
func appendMarkedBlock(path, block string) (changed bool, err error) {
	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}
	if strings.Contains(string(existing), docMarkerBegin) {
		return false, nil
	}

	var out []byte
	if len(existing) == 0 {
		out = []byte(block)
	} else {
		content := string(existing)
		if !strings.HasSuffix(content, "\n") {
			content += "\n"
		}
		out = []byte(content + "\n" + block)
	}
	if err := os.WriteFile(path, out, 0o644); err != nil {
		return false, err
	}
	return true, nil
}
