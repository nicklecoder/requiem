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
const docBlockVersion = 7

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
	"- `requiem review` — describe what is staged but not yet committed",
	"- `requiem discard [<id>]` — unstage and revert; everything pending if no id given",
	"- `requiem commit` — the approval step; nothing is permanent until this runs",
	"",
	"Two optional fields on `add`/`update`:",
	"",
	"- `--modality must|should|may|must_not|should_not` — normative force. The one",
	"  field requiem reasons with: `audit` flags a pair whose modalities oppose.",
	"- `--status proposed` — a decision under consideration rather than in force.",
	"  Searched and audited like a settled one, so you find out whether a proposal",
	"  conflicts with something before committing to it. `list --status proposed`",
	"  is the queue of open decisions.",
	"",
	"`requiem get` also reports what points *at* a statement: `referenced_by` for",
	"statements refining or depending on it, and `rejected_alternatives` for ideas",
	"turned down in its favour — read those before re-proposing something.",
	"",
	"**Semantic matching.** Lexical search cannot find a prior decision worded in",
	"vocabulary your draft doesn't share. Embeddings close that gap. If",
	"`.requiem/config.yaml` names an embedding endpoint, requiem fetches vectors",
	"itself:",
	"",
	"```sh",
	"requiem reindex --embed      # fill every missing or stale vector",
	"```",
	"",
	"- Vectors live in the gitignored index and **do not survive a clone**. Run",
	"  `requiem reindex --embed` after cloning; `requiem list --needs-embedding`",
	"  shows what is missing.",
	"- A partial run exits nonzero and keeps whatever succeeded — re-running",
	"  retries only the failures.",
	"- `audit` and `check` warn on stderr when part of the corpus is unembedded,",
	"  because a short result list would otherwise be indistinguishable from a",
	"  thorough one.",
	"",
	"Add `--semantic` to search by meaning as well as vocabulary — requiem embeds",
	"the query text for you:",
	"",
	"```sh",
	"requiem check --namespace <area> --text \"<the idea>\" --semantic",
	"```",
	"",
	"It is opt-in because a plain `check` stays offline and fast. Without an",
	"endpoint, supply your own with `--model <name> --vector <json>`; requiem",
	"refuses a vector from a different model, since cosine similarity across two",
	"models is meaningless.",
	"",
	"`requiem audit` sweeps the whole corpus for pairs that may conflict or",
	"duplicate each other. Record every verdict with",
	"`requiem link <a> <b> --type conflicts_with|duplicates|not_related` so the",
	"pair stops resurfacing.",
	"",
	"**Linking decisions to code.** Label the code that implements a statement so a",
	"changed decision can report what it affects:",
	"",
	"```go",
	"// requiem: auth/session/no-plaintext-tokens",
	"func storeToken(...) { ... }",
	"```",
	"",
	"Labels travel with the code through refactors, and labelling a *test* is",
	"stronger than labelling an implementation: a passing labelled test is evidence",
	"the statement holds, where a comment only asserts intent.",
	"",
	"- `requiem trace <namespace/id>` — the labelled sites referencing a statement.",
	"- `requiem update` prints which sites a body change affects.",
	"- `requiem audit` flags code still referencing a superseded or rejected",
	"  statement — the decision changed and the code was never brought along.",
	"- `get`/`list` show `code_refs`, a count, absent where labelling is unused.",
	"- `requiem list --unreferenced` finds statements no code implements. A",
	"  statement counts as covered when the statements refining it are labelled;",
	"  `--direct` suppresses that inference.",
	"- A label in a `.md` or `.txt` file is a *mention*, not a reference: it does",
	"  not count toward `code_refs`, but a changed decision still flags it.",
	"- A mistyped id is reported by `audit` and by `reindex`, which exits nonzero.",
	"- Writing *about* a label — a test fixture, a tutorial snippet — would",
	"  otherwise scan as a real one. Put `requiem:ignore` on the line, with an",
	"  optional reason after it.",
	"- `--abstract` on a statement no code can implement (a principle, a process",
	"  decision) keeps it out of `--unreferenced`. `audit` flags it if code turns",
	"  up referencing it anyway.",
	"",
	"`requiem mv` rewrites labels to the new id and leaves those edits unstaged,",
	"so review them with `git diff`. `--no-rewrite-refs` reports them instead.",
	"",
	"A code commit can also name the decision it implements, which records *when*",
	"something was built rather than where it lives now:",
	"",
	"```",
	"Enforce single-model embedding",
	"",
	"Requiem-Id: embedding/model-pinning",
	"```",
	"",
	"`requiem trace` reports those commits. Requiem only reads them — you write",
	"them on your own commits.",
	"",
	"Full reference: `requiem --help`.",
}, "\n") + "\n" + docMarkerEnd + "\n"

// AgentDocBlock returns the block init writes into AGENTS.md/CLAUDE.md.
// Exported so internal/cli can assert the block keeps pace with the command
// tree — the block has gone stale three times, each time because a feature
// shipped and nothing noticed the documentation had stopped being true.
func AgentDocBlock() string { return agentDocBlock }

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
