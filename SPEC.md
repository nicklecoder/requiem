# Requiem — Agent-Native Requirements & Design Tracking

## Problem

Spec-first development (fully specify, then implement) works well for small projects, but breaks down once a project crosses a complexity threshold — too large, existing code with architectural issues, or a spec too unwieldy to stay internally consistent. The symptom isn't "the spec is too long," it's that nothing enforces consistency as it grows:

- **Working-memory ceiling.** A spec that fits in one head (human or agent) stays coherent because every edit implicitly cross-references everything else. Past that size, edits happen locally and contradictions creep in unnoticed.
- **Undeclared prior constraints.** Existing code encodes rules nobody wrote down. A spec can be internally consistent on paper and still conflict with reality — discovered mid-implementation, sending agents down rabbit holes.
- **Vocabulary drift.** The same concept gets named differently in different places, so a text search for contradictions misses it.
- **No relationship model.** Prose asserts things but doesn't declare how they relate (supersedes, conflicts with, depends on), so "does A contradict B" has to be re-derived by rereading instead of looked up.

**Signals you've hit the limit** (reasons to reach for this system):
- Rereading more of the spec to make an edit than writing it.
- Contradictions surface during implementation, not spec review.
- Restating a rule because it's unclear whether it already exists elsewhere.
- The agent asks for clarification on something that should've been settled, or visibly reconciles a conflict you didn't know existed.
- Existing code constraints keep surprising you after the spec was declared done.

## What this system is (and isn't)

Reasoning about contradictions still requires reasoning — this system doesn't replace that. What it provides is **context economy**: the ability for an agent to retrieve the narrow slice of prior decisions relevant to what it's about to write, instead of needing the whole spec in context to notice a conflict. It is infrastructure and retrieval, not a judge.

## Design Principles

- **Agent-native.** The primary consumer is an AI agent, not a human. Humans interact directly only if they really want to.
- **Per-project isolation.** State lives inside the project; no cross-project bleed, no global registry.
- **Integrate with git, don't replace it.** History, diffing, blame, and revert are already solved problems — don't rebuild them. Only add what git doesn't provide (structured relational queries, fast retrieval).
- **Two ingestion paths feed one graph.** Statements come from human↔agent dialogue and from the agent exploring existing code. Both are first-class, so a code-derived constraint can be checked against a dialogue-derived requirement symmetrically.

## Architecture

### Interface

CLI, JSON in/out. No long-running server or per-project MCP wiring required — the agent invokes it like any other CLI tool via its shell/Bash access, and it works regardless of which agent harness is in use. Can be wrapped as an MCP server later without changing the storage layer.

### Storage

- **Files are canonical and git-tracked.** One file per statement, under a path that mirrors its namespace, e.g. `.requiem/statements/auth/session/<id>.md`. Deterministic frontmatter + body format so diffs are meaningful.
- **SQLite is a disposable, rebuildable index** — gitignored, not committed. Exists purely to make queries (`get`, `list`, `check`) fast: full-text search (SQLite FTS5) plus namespace/tag filtering and relationship lookups. If deleted or corrupted, nothing is lost — rebuild from files via `reindex`.

This direction (files canonical, DB derived) was chosen deliberately over the reverse, because it lets git carry all history/diff/blame responsibility, avoids a two-sources-of-truth consistency problem, and means cloning the project doesn't lose anything.

### Indexing

- A manifest table (`file_path`, `mtime`, `size`) lets reindexing be **incremental**: on read or trigger, stat the tree, skip anything unchanged, and only reparse files that differ. Relationships are stored on the *owning* file's frontmatter (not a global edges file), so updating one statement never requires touching the files it references — avoids a merge-conflict hotspot and keeps reindex genuinely per-file.
- **Trigger 1 — lazy staleness check on read.** Before `get`/`list`/`check` answer a query, compare the manifest to disk and reindex anything stale first. Correct regardless of what caused the change (hand edit, git operation) and needs no extra infrastructure.
- **Trigger 2 — git hooks** (`post-checkout`, `post-merge`, `post-rewrite`), installed automatically by `requiem init`, chaining after any pre-existing hook rather than clobbering it. These fire on bulk-change operations and run a plain `reindex`, keeping the index eagerly warm without a background process. A targeted `git diff --name-only`-scoped scan was considered instead of a full tree walk, but dropped: the manifest-diff skip logic (see above) already makes a full walk cheap when little changed, and `post-rewrite` doesn't cleanly offer a before/after ref pair the way `post-checkout`/`post-merge` do, so it would've needed a separate code path anyway.
- No filesystem watcher / daemon — would require a long-running process per project, which fights the "no server, no lifecycle" goal, and isn't needed given the above two triggers.

### History & Approval

- The CLI **auto-stages** (`git add`, scoped to the specific file that changed) on every mutation (`add`/`update`/`link`) — it never auto-commits. Because staging overwrites the staged blob for a path, a statement edited repeatedly during exploration (including dead ends) collapses to whatever it looks like when someone actually reviews it. Nothing about abandoned intermediate states becomes part of visible history.
- **Commit is approval** — there is no separate "approved" status to track. `git diff --staged -- .requiem/` already shows pending changes at any time.
- `requiem review` — optional, not a gate. Turns staged changes into a human-readable description of what changed, for whoever wants to look before committing.
- `requiem commit` — thin, path-scoped wrapper around `git commit -- .requiem/...` with a structured message, so it can't accidentally sweep in unrelated staged code changes elsewhere in the working tree.
- `requiem discard` — unstage and revert to last commit, for the rejection path.
- Commits stay in the project's one real repo (no nested/separate `.git`), correlated on the same timeline and branch structure as code, and survive a normal clone.

### Code-Derived Staleness

A code-derived statement's provenance stores a hash of the referenced `line_range` captured when it was written, alongside the `file:line` pointer. On reindex, that range is rehashed and compared; a mismatch flags the statement as stale — "the code this was derived from has changed, re-verify this claim." This is computed live by the SQLite index (rehash vs. stored hash), not written back into the statement file — staleness is a derived fact, not a content edit, so it doesn't go through the stage/commit flow. `stale: true/false` is exposed as part of `get`/`check` output. Reformatting-only changes can false-positive; cheap for an agent to glance at and dismiss versus the cost of a silently wrong statement.

### Agent Discoverability

Being agent-native only helps if an agent actually knows requiem exists in a given project and how to use it, without a human re-explaining it every session. `requiem init` writes (or appends to, chaining after any existing content rather than overwriting it — same idempotent marker-block approach as git hook installation) a concise workflow summary into `AGENTS.md` and `CLAUDE.md` at the project root: what requiem is for, when to reach for `check`, how `add`/`update`/`link`/`reject` map to real vs. rejected decisions, and that commit is the approval step. Any agent whose harness loads one of those files at session start picks this up automatically. These two files are the project's own, not requiem's data, so — unlike everything else `init`/`add`/`update`/`link`/`reject` touch — they're deliberately left unstaged, following the project's normal commit workflow rather than requiem's spec-approval one. `requiem --help` remains the full command reference; the doc block is intentionally short since it competes for context budget with everything else in those files.

## Data Model

**Statement**
| field | notes |
|---|---|
| `id` | namespace-relative slug, doubles as the filename; stable once other statements may reference it — renaming is a deliberate operation |
| `namespace` | hierarchical path, mirrors directory structure |
| `kind` | requirement / rule / design — plain string, not a hard-constrained enum, so new kinds can be added without a migration |
| `body` | |
| `status` | active / superseded / deprecated |
| `tags` | |
| `provenance` | `dialogue` or `code-derived`. For `code-derived`: `file`, `line_range`, and a hash of that line range captured at write time — see Staleness below. |
| `created_at` | |

**Relationship**: `from`, `to`, `type` (`conflicts_with`, `supersedes`, `depends_on`, `refines`), `note`. (`scoped_to` was considered and dropped — namespace already expresses what a statement applies to; a redundant statement-to-statement edge for the same thing wasn't worth the extra relationship type.)

**Rejection** — a lighter-weight companion to Statement, for ideas explicitly considered and rejected (not abandoned mid-thought — those are just `discard`ed and leave no trace). Recorded so a future agent doesn't re-propose the same rejected idea.
- Lives in a sister file per namespace: `.requiem/statements/<namespace>/_rejected.md`, an append-only list of entries alongside that namespace's statement files.
- Each entry: the body of what was proposed, the reason it was rejected, and an optional pointer to the active statement that addresses the concern instead.
- Indexed into SQLite alongside statements but tagged distinctly (`source_kind: rejection`), so `check` can surface them clearly labeled "previously rejected" rather than mixed in with active statements.
- Follows the same stage → commit model as everything else — not permanent until committed.

## CLI

**Output convention:** bare data on stdout on success; nonzero exit code + error on stderr on failure. No wrapper envelope to unwrap on the common path. Every statement payload (in `get`, `add`, `update`, `link` output, and each entry returned by `list`/`check`) includes a computed `full_id` — the `<namespace>/<id>` composite — so output from one command can be passed straight into another's `<namespace/id>` argument without the caller concatenating fields itself.

| command | input | output |
|---|---|---|
| `init` | — | path, hooks installed, docs updated |
| `add` | `--id --namespace --kind --body [--tags] [--provenance] [--source file:line]` | created statement |
| `update` | `<id> --body [--status]` | updated statement |
| `link` | `<from-id> <to-id> --type [--note]` | confirmation |
| `reject` | `--id --namespace --body [--see-instead]` | created rejection |
| `get` | `<id>` | full statement incl. resolved relationships, `stale` flag if code-derived |
| `list` | `[--namespace] [--kind] [--status] [--tag]` | array of compact summaries |
| `check` | `--namespace --text [--tags]` | ranked array of compact candidates — id, namespace, kind, status, short excerpt, relevance signal; statements and rejections included, distinctly tagged. Full bodies are a deliberate second `get` call, not inline, to keep `check` cheap regardless of match count. |
| `reindex` | — | counts: added/updated/removed/unchanged |
| `review` | — | human-readable description of staged changes |
| `commit` | `[--message]` | commit sha |
| `discard` | `[<id>]` | confirmation |

## Status

Architecture, data model, and CLI surface are settled. Next: implementation planning.
