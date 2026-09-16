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
const docBlockVersion = 17

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
	"### Working process",
	"",
	"1. **Before proposing anything non-trivial**, run `check` on the relevant",
	"   namespace. Read the `rejection` results first — those are ideas this",
	"   project already considered and turned down, and re-proposing one is the",
	"   most common way to waste the human's time.",
	"2. If a result conflicts with what you were about to do, say so rather than",
	"   silently picking a side. Requiem surfaces candidates; judging them is",
	"   your job, and reporting the conflict is part of judging it.",
	"3. When a decision is made, `add` it — with `--modality` if it carries",
	"   normative force, `--status proposed` if it is not settled yet.",
	"",
	"   **Never write the open part into the body of an active statement.** A",
	"   body that says \"undecided\", \"open whether\" or \"for now\" is a question",
	"   filed where nothing will look for it: `list --status proposed` is the",
	"   only queue of open decisions, and an active statement is not in it. The",
	"   question then goes stale silently the moment it is answered elsewhere.",
	"   Split it — the settled part `active`, the open part `proposed` — or",
	"   record the whole statement as `proposed`. Requiem's own corpus carried",
	"   four such bodies, every one long since shipped, while the queue read",
	"   zero.",
	"4. `link` it to the principle it refines, so the graph is navigable from",
	"   both ends, and `reject` the alternatives that lost with `--see-instead`",
	"   pointing at what replaced them. A rejection costs one command and saves",
	"   the next agent from re-deriving it.",
	"5. While implementing, write `requiem: <namespace/id>` in a comment above",
	"   the code that carries the decision — an ordinary comment, in that",
	"   file's own syntax, written by you. Requiem has no command for this on",
	"   purpose: you are editing the file and know its language, where requiem",
	"   could only guess from the extension. Labelling a **test** is stronger",
	"   than labelling an implementation: a passing labelled test is evidence",
	"   the statement holds, where a comment only asserts intent.",
	"",
	"   Labels are a shortcut, not an obligation. Nothing enforces them, and",
	"   partial coverage is still useful: a label is an exact answer, and",
	"   `requiem trace <id> --search` answers from the statement's own wording",
	"   wherever a label is missing. Label what you can; do not stall on it.",
	"6. Periodically run `requiem reindex --embed` then `requiem audit`, and",
	"   record a verdict on every pair it surfaces. An adjudicated pair stops",
	"   resurfacing and frees the slot for the next candidate.",
	"7. `requiem commit` when the decision is settled. Nothing is permanent",
	"   until then, so exploring a dead end and running `discard` leaves no trace.",
	"",
	"**Writing a statement that can be found again.** State the decision *and*",
	"why. \"Use Postgres\" is not retrievable; \"Session state lives in Postgres",
	"rather than Redis, because it must survive a restart\" matches a future draft",
	"that shares neither word. Retrieval works on the body, so a body that omits",
	"the reasoning cannot match a draft that arrives at it differently.",
	"",
	"### Commands",
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
	"Each candidate carries a `verdict` — `duplicate`, `related` or `weak` — with",
	"the evidence behind it: the cosine where a vector was compared, and the",
	"identifiers it shares with your draft. A result set that is entirely `weak`",
	"is the answer \"nothing here states this already\"; check always fills its",
	"limit, so the count of results never meant anything on its own.",
	"",
	"`--touches <identifier>` retrieves by identifier — a column, field or symbol",
	"the change touches — which finds a prior decision about",
	"`external_venues.status` even when it was written in words your draft does",
	"not use.",
	"",
	"**Checking a change rather than a draft.** `requiem check --diff` takes the",
	"patch instead: it reports the decisions whose labels sit in an edited hunk,",
	"the ones whose own source range you touched, and any record naming an",
	"identifier your change adds — rejections listed separately, since walking",
	"back into a rejected idea is the thing most worth catching. `--staged` and",
	"`--diff-rev <range>` scope it elsewhere. Failing a build on it is opt-in per",
	"project (`gate.diff` in `.requiem/config.yaml`), and even then it fails only",
	"on facts: a label pointing at a retired or rejected decision, or a covering",
	"statement whose source range has drifted.",
	"",
	"**Starting work in an area.** `requiem brief --namespace <area>` returns the",
	"minimal set in force there: must/must_not rules, the principles they refine,",
	"and what has already been rejected. Small enough to paste into a prompt, and",
	"it counts what the cap left out instead of hiding it.",
	"",
	"**Recording outcomes.** Writes auto-stage in git but never auto-commit, so a",
	"dead end leaves no trace if you back out:",
	"",
	"- `requiem add` / `update` / `link` — decisions that stand. `add` runs the",
	"  duplicate check itself and refuses when the corpus already says this,",
	"  naming what it found; `--duplicate-ok` records it anyway.",
	"- `requiem batch` — many writes as JSON Lines on stdin, one `{\"op\":...}` per",
	"  line, with a per-record result for each. A malformed line writes nothing;",
	"  a refused write is reported against its line while the rest apply.",
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
	"  is the queue of open decisions — and the only one, which is why an open",
	"  question must never be written into an active statement's body instead.",
	"",
	"**Recording an open question.** There is no separate question record and none",
	"is needed: `kind` is a free string and `modality` is optional, so",
	"",
	"```sh",
	"requiem add --namespace <area> --id <slug> --kind question --status proposed \\",
	"  --body \"Open question: ... . What hangs on it: ...\"",
	"```",
	"",
	"is a question, and `list --status proposed` is the queue that holds it.",
	"",
	"Answer it by adding the decision, then:",
	"",
	"```sh",
	"requiem link <decision> <question> --type supersedes",
	"requiem update <question> --status superseded",
	"```",
	"",
	"That keeps the question's history instead of erasing it. Six research agents on",
	"one real project produced about 120 open questions and recorded five, because",
	"writing one looked like it needed a normative force it does not have; two",
	"defects later traced back to questions that never entered the corpus.",
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
	"duplicate each other. Pairs that share an identifier — or come from the same",
	"source file — rank first, because that is evidence about subject matter",
	"rather than a guess from wording; the rest are each statement's nearest",
	"neighbours by embedding. Opposed modality is only a tiebreak: ranking by it",
	"put 48 unrelated pairs in the top 50 of a real audit. The stderr line says",
	"how many unadjudicated pairs remain, and every verdict you record frees a",
	"slot for the next candidate.",
	"",
	"Record what you find:",
	"",
	"- `requiem link <a> <b> --type conflicts_with|duplicates` — a real finding,",
	"  which belongs in the graph.",
	"- `requiem dismiss <a> <b> --note \"...\"` — seen and unrelated. Stored as a",
	"  verdict, not an edge, so dismissals never accumulate in the graph you read",
	"  to understand how decisions fit together. `--restore` takes one back.",
	"",
	"**Linking decisions to code.** Label the code that implements a statement so a",
	"changed decision can report what it affects:",
	"",
	"```go",
	"// requiem: auth/session/no-plaintext-tokens", // requiem:ignore documentation example, and the marker must stay inside the string
	"func storeToken(...) { ... }",
	"```",
	"",
	"Labels travel with the code through refactors, and labelling a *test* is",
	"stronger than labelling an implementation: a passing labelled test is evidence",
	"the statement holds, where a comment only asserts intent.",
	"",
	"- `requiem trace <namespace/id>` — the labelled sites referencing a statement.",
	"  Add `--search` to also find files sharing the statement's vocabulary,",
	"  which works before anything has been labelled at all.",
	"- Write the marker yourself, in the file's own comment syntax. Spacing and",
	"  capitalisation are tolerated — `Requiem : ns/id` is found — but the id",
	"  must be exact. Copy `full_id` from `add` or `check` output rather than",
	"  retyping it.",
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
	"- `--abstract` on a statement no code can implement keeps it out of",
	"  `--unreferenced`. Three kinds qualify: a principle, a decision about",
	"  process rather than software, and a **prohibition** — \"requiem must not",
	"  do X\" is implemented by absence, so there is no site to label. A",
	"  prohibition enforced by a specific guard is the exception: label the",
	"  guard. `audit` flags an abstract statement if code references it anyway.",
	"- `list --unreferenced` reports only live decisions. A superseded or",
	"  deprecated one has no implementation because it was withdrawn, which is",
	"  true and useless.",
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
