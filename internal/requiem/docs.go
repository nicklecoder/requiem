package requiem

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// agentDocFiles are written/updated by Init so any agent whose harness
// loads one of these at session start learns requiem exists and how to use
// it without a human explaining it each time. AGENTS.md is the emerging
// cross-harness convention; CLAUDE.md is Claude Code's own, and Claude Code
// reads only the latter (it has no AGENTS.md fallback), so both are written
// rather than relying on one to cover the other.
var agentDocFiles = []string{"AGENTS.md", "CLAUDE.md"}

// docBlockVersion is bumped whenever agentDocBlock's content changes, so
// Init can refresh a block written by an older requiem instead of leaving
// it frozen forever. The previous scheme keyed purely on an unversioned
// marker and returned early whenever it was present, which meant a project
// initialized once could never pick up a correction to this text.
const docBlockVersion = 2

const (
	// docMarkerPrefix matches the opening marker of *any* version, including
	// the original unversioned "<!-- >>> requiem >>> -->", so upgrades from
	// before docBlockVersion existed are still recognized and replaced.
	docMarkerPrefix = "<!-- >>> requiem"
	docMarkerEnd    = "<!-- <<< requiem <<< -->"
)

var docMarkerBegin = fmt.Sprintf("<!-- >>> requiem v%d >>> -->", docBlockVersion)

// agentDocBlock is deliberately short: it competes for context budget with
// everything else in a startup file, so it states the workflow, not the full
// reference — `requiem --help` is that. It is built from interpreted string
// literals rather than raw literals spliced around backticks; the latter is
// what previously produced unbalanced code spans in the emitted markdown.
//
// Structure is bullets and headers rather than prose paragraphs on purpose:
// agent startup files are scanned, not read, and organized sections are
// followed more reliably than dense text.
var agentDocBlock = docMarkerBegin + "\n" + strings.Join([]string{
	"## requiem",
	"",
	"This project tracks requirements, rules, and design decisions (\"statements\")",
	"in `.requiem/` using requiem, so you can check a new idea against prior",
	"decisions without loading the whole spec into context.",
	"",
	"**Before proposing anything non-trivial**, look for prior decisions and",
	"previously-rejected ideas:",
	"",
	"```sh",
	"requiem check --namespace <area> --text \"<the idea>\"",
	"```",
	"",
	"Returns ranked, excerpt-only candidates (top 10; `--limit` to change), each",
	"tagged `statement` or `rejection`. Call `requiem get <namespace/id>` for the",
	"full body of one that actually matters.",
	"",
	"**Recording outcomes.** Writes auto-stage in git but never auto-commit, so a",
	"dead end leaves no trace if you back out:",
	"",
	"- `requiem add` / `update` / `link` — decisions that stand",
	"- `requiem reject` — ideas explicitly considered and rejected, so they aren't re-proposed",
	"- `requiem mv <from> <to>` — relocate a statement, rewriting inbound references",
	"- `requiem commit` — the approval step; nothing is permanent until this runs",
	"",
	"**Semantic matching.** Requiem computes no embeddings itself; it stores",
	"vectors you supply and does the cosine math. Without them `check` matches",
	"shared vocabulary only, and `audit` cannot run at all. Three rules:",
	"",
	"- Every vector in a project must come from the **same model**, including the",
	"  query vector passed to `check --vector` — requiem rejects a mismatch.",
	"- Vectors live in the gitignored index and **do not survive a clone**. Run",
	"  `requiem list --needs-embedding` before trusting an `audit` result.",
	"- Pass vectors via shell substitution so they never enter your context —",
	"  one vector is 1k-8k tokens depending on the model.",
	"",
	"Embedding one statement, against a local Ollama:",
	"",
	"```sh",
	"ID=auth/session/no-plaintext-tokens",
	"BODY=\"$(requiem get \"$ID\" | jq -r .body)\"",
	"VEC=\"$(jq -n --arg t \"$BODY\" '{model:\"all-minilm\",input:$t}' \\",
	"  | curl -s http://localhost:11434/api/embed -d @- | jq -c '.embeddings[0]')\"",
	"requiem embed \"$ID\" --model all-minilm --vector \"$VEC\"",
	"```",
	"",
	"`requiem audit` then sweeps the whole corpus for pairs that may conflict or",
	"duplicate each other. Record every verdict with",
	"`requiem link <a> <b> --type conflicts_with|duplicates|not_related` so the",
	"pair stops resurfacing.",
	"",
	"Full reference: `requiem --help`.",
}, "\n") + "\n" + docMarkerEnd + "\n"

// ensureAgentDocs writes agentDocBlock into each of agentDocFiles at root,
// appending after any existing content (never overwriting it), replacing an
// older version of the block in place if one is there, and skipping files
// already carrying the current version — the same idempotent-chaining
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
// creating the file if needed, replacing a stale earlier version of the
// block where it sits, or otherwise appending after existing content —
// never touching anything outside the markers. No-op (changed=false) if the
// current version's block is already present.
func appendMarkedBlock(path, block string) (changed bool, err error) {
	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}
	content := string(existing)

	// Must precede the prefix check: docMarkerBegin contains docMarkerPrefix.
	if strings.Contains(content, docMarkerBegin) {
		return false, nil
	}

	if start := strings.Index(content, docMarkerPrefix); start >= 0 {
		if rel := strings.Index(content[start:], docMarkerEnd); rel >= 0 {
			end := start + rel + len(docMarkerEnd)
			// Absorb the block's own trailing newline so repeated upgrades
			// don't accrete a blank line each time.
			if end < len(content) && content[end] == '\n' {
				end++
			}
			if err := os.WriteFile(path, []byte(content[:start]+block+content[end:]), 0o644); err != nil {
				return false, err
			}
			return true, nil
		}
		// Opening marker with no close: the block was hand-edited or
		// truncated. Appending a clean copy is safer than guessing where
		// the damaged one ends and deleting the caller's content with it.
	}

	var out []byte
	if len(content) == 0 {
		out = []byte(block)
	} else {
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
