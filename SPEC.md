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
- **Embedding vectors live in that index too, and the disposability claim covers them only because the pipeline that produces them is itself committed.** A vector cannot be recovered by reparsing a statement file the way every other table can; it has to be recomputed. That is what `.requiem/config.yaml` (see Semantic Retrieval) exists to guarantee — the endpoint and model are version-controlled, so `reindex --embed` reproduces the vectors on any clone. Without a committed pipeline this bullet would be false for embeddings, which is the reason the config is a tracked file rather than a local setting.

This direction (files canonical, DB derived) was chosen deliberately over the reverse, because it lets git carry all history/diff/blame responsibility, avoids a two-sources-of-truth consistency problem, and means cloning the project doesn't lose anything.

### Indexing

- A manifest table (`file_path`, `mtime`, `size`) lets reindexing be **incremental**: on read or trigger, stat the tree, skip anything unchanged, and only reparse files that differ. Relationships are stored on the *owning* file's frontmatter (not a global edges file), so updating one statement never requires touching the files it references — avoids a merge-conflict hotspot and keeps reindex genuinely per-file.
- **Trigger 1 — lazy staleness check on read.** Before `get`/`list`/`check` answer a query, compare the manifest to disk and reindex anything stale first. Correct regardless of what caused the change (hand edit, git operation) and needs no extra infrastructure.
- **Trigger 2 — git hooks** (`post-checkout`, `post-merge`, `post-rewrite`), installed automatically by `requiem init`, chaining after any pre-existing hook rather than clobbering it. These fire on bulk-change operations and run a plain `reindex`, keeping the index eagerly warm without a background process. A targeted `git diff --name-only`-scoped scan was considered instead of a full tree walk, but dropped: the manifest-diff skip logic (see above) already makes a full walk cheap when little changed, and `post-rewrite` doesn't cleanly offer a before/after ref pair the way `post-checkout`/`post-merge` do, so it would've needed a separate code path anyway.
- No filesystem watcher / daemon — would require a long-running process per project, which fights the "no server, no lifecycle" goal, and isn't needed given the above two triggers.
- **`reindex --embed`** additionally fills in any missing or stale vector by calling the configured embedding endpoint. It is separate from plain `reindex` because it is the one indexing operation that reaches the network: a bare `reindex` must stay fast, offline, and safe to run from a git hook. Whether the installed hooks should pass `--embed` when an endpoint is configured — making a fresh clone self-heal its vectors — is deliberately left open; it would make `git checkout` perform network I/O, which is surprising enough to want a decision rather than a default.

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

## Semantic Retrieval

Lexical search has a structural blind spot: it cannot find a prior decision worded in vocabulary the draft doesn't share. That is precisely the *vocabulary drift* failure named in the Problem section, so full-text search alone cannot solve the problem this tool exists for. Embeddings close that gap.

### Inference stance

Requiem bundles no model, no inference runtime, and no ML dependency — `CGO_ENABLED=0` and the single static binary are preserved. It does, however, know *how to obtain* a vector: `.requiem/config.yaml` names an OpenAI-compatible `/v1/embeddings` endpoint, a model, and the *name* of an environment variable holding an API key (never the key itself). One client shape covers Ollama, LM Studio, llama.cpp, vLLM, LocalAI, and OpenAI.

This is a deliberate narrowing of an earlier, broader rule — "requiem never computes embeddings itself" — which conflated two claims: *bundles no inference runtime* (worth defending) and *cannot obtain a vector* (which made the index's disposability guarantee false). Shelling out to a configured endpoint is the same posture as shelling out to `git`, which requiem already does. `embed --vector` remains as a manual escape hatch, so an agent with access to some exotic model can still supply one directly.

### Model pinning

Cosine similarity between vectors from two different models is a number that looks plausible and means nothing. `embedding_meta` pins the corpus to one model/dims pair; `embed` refuses a mismatch on write, and `check --vector` refuses one on read. Re-pinning requires an explicit force, which wipes every existing vector — mixing vector spaces is worse than having none.

### Rank fusion

`check` merges up to three ranked lists: statement FTS, rejection FTS, and semantic similarity. Their scores are not comparable, and the incomparability is not a constant bias that could be corrected with a scale factor. FTS5 clamps a term's IDF to `1e-6` whenever it appears in more than half the rows (`ext/fts5/fts5_aux.c`), so a *common*-term lexical hit scores near zero and sorts below every semantic hit, while a *rare*-term hit scores around −5 and sorts above. The direction flips per query term.

Results are therefore fused by **Reciprocal Rank Fusion** — `Σ 1/(60 + position)` across the lists a candidate appears in, discarding raw scores in favour of rank position. RRF needs no tuning constants that would require recalibration per embedding model, and it resolves the statement-vs-rejection bm25 incomparability in the same stroke. The fused value is emitted negated, so the `rank` field keeps its documented "lower is more relevant" direction; only the scale changes, which was never specified.

Fusion also changes what happens when both paths find the same statement. Previously the semantic pass skipped anything lexical search had already returned, so agreement between the two was invisible. Summing both contributions instead makes agreement *raise* a candidate — shared vocabulary and embedding proximity are independent signals, and a statement carrying both is a better answer than one carrying either. Such a candidate is marked `match_kind: both`.

### Query construction

`check` builds its FTS query by OR-joining the draft's tokens. Terms occurring in more than half the rows are dropped first, via an `fts5vocab` lookup — exactly where FTS5 clamps a term's IDF to `1e-6`, so these are the terms its own ranking already treats as carrying no information. Being frequency-driven rather than a fixed word list, this adapts to the corpus and catches domain stopwords (`token` in an auth-heavy namespace) that no English stoplist would. Frequencies are counted per FTS table, since a term saturating the statement corpus may still discriminate among rejections.

Two limits on it, both deliberate.

It is **more aggressive than the clamp it mirrors**: the clamp lowers a score, while dropping a term removes its documents from the result set entirely. That difference is harmless on a large corpus and destructive on a small one, where "more than half the rows" describes a handful of documents rather than the vocabulary — at the limit, every term in a single-document table saturates it. So no filtering happens below a floor of twenty rows, which costs nothing, because the blowup this exists to prevent needs a corpus large enough for a full-table match to be expensive. If every term would be dropped above that floor, all are kept: an empty query returns nothing, which reads as "no prior decisions" rather than "no discriminating terms".

It **reduces broad matching without bounding it**. Measured on a 200-statement corpus, an ordinary draft sentence matched 86% of rows unfiltered and 66% filtered. The remainder is not a tuning failure: several terms that are each individually informative still union to most of the corpus, and no per-term threshold can bound a union. Lowering the threshold to chase that number would discard real signal. Bounding what reaches the caller is the result limit's job, which is why both exist.

### Coverage honesty

A statement with no vector is invisible to semantic search, and an empty result set is indistinguishable from "swept everything, found nothing" — a false all-clear on the one job this tool exists to do. So: zero coverage is an error, and partial coverage emits a warning on stderr naming the shortfall. Warnings go to stderr rather than into the payload so `stdout` stays bare JSON per the CLI output convention; agent harnesses surface combined output, so the warning still reaches its reader.

### Corpus-wide audit

Where `check` compares one draft against prior decisions, `audit` compares every active statement against every other, surfacing close pairs as conflict or duplicate candidates. Pairs with any recorded relationship are excluded, so an adjudicated pair stops resurfacing. The scan is O(n²) in memory; at the corpus sizes this tool targets (hundreds of statements, so tens of thousands of comparisons over a few hundred floats) that is milliseconds, and no ANN index is warranted. As everywhere else, requiem surfaces the candidate and the agent classifies it.

## Data Model

**Statement**
| field | notes |
|---|---|
| `id` | namespace-relative slug, doubles as the filename; stable once other statements may reference it — renaming is a deliberate operation |
| `namespace` | hierarchical path, mirrors directory structure |
| `kind` | requirement / rule / design — plain string, not a hard-constrained enum, so new kinds can be added without a migration. Deliberately **inert**: it exists for grouping and retrieval (`list --kind`), not semantics — see Modality below |
| `modality` | optional, closed: `must` / `should` / `may` / `must_not` / `should_not`. The normative strength of the statement — see Modality below |
| `body` | |
| `status` | active / superseded / deprecated |
| `tags` | |
| `provenance` | `dialogue` or `code-derived`. For `code-derived`: `file`, `line_range`, and a hash of that line range captured at write time — see Staleness below. |
| `created_at` | |

Three fields are *derived at read time and never written to the file*: `stale` (code-derived provenance rehashed against current source), `embedding_status` (`missing` / `stale` / `fresh`, from comparing the stored vector's `source_hash` against the current body), and the `rank` returned by `check`. None is content, so none passes through the stage/commit flow — a fact about a statement is not an edit to it.

**Relationship**: `from`, `to`, `type` (`conflicts_with`, `supersedes`, `depends_on`, `refines`, `duplicates`, `not_related`, `moved_to`), `note`. The last three exist to service `audit` and `mv`: `duplicates` and `not_related` record an agent's verdict on a candidate pair so it stops resurfacing — `not_related` asserts no semantic relationship at all, only that the pair has been judged — and `moved_to` marks the stub `mv --leave-link` leaves behind. (`scoped_to` was considered and dropped — namespace already expresses what a statement applies to; a redundant statement-to-statement edge for the same thing wasn't worth the extra relationship type.)

**Rejection** — a lighter-weight companion to Statement, for ideas explicitly considered and rejected (not abandoned mid-thought — those are just `discard`ed and leave no trace). Recorded so a future agent doesn't re-propose the same rejected idea.
- Lives in a sister file per namespace: `.requiem/statements/<namespace>/_rejected.md`, an append-only list of entries alongside that namespace's statement files.
- Each entry: the body of what was proposed, the reason it was rejected, and an optional pointer to the active statement that addresses the concern instead.
- Indexed into SQLite alongside statements but tagged distinctly (`source_kind: rejection`), so `check` can surface them clearly labeled "previously rejected" rather than mixed in with active statements.
- Follows the same stage → commit model as everything else — not permanent until committed.

### Modality

`kind` and normative strength are orthogonal, and only one of them can bear semantics.

Category resists closure. ISO/IEC/IEEE 29148 splits requirements into functional, quality, usability, interface and more, and the boundary between functional and non-functional is famously unclear in practice — many requirements sit on both sides. Asking an agent to pick one value from a closed category set produces inconsistent answers across sessions, which splits statements that belong together. The Volere requirements shell is explicit that a requirement's Type exists "as an aid to discovering the requirements and to be able to group the requirements that are relevant to a specific expert specialty" — retrieval, not meaning. That is exactly the job `list --kind` already does, so `kind` stays open and inert.

Normative strength is the opposite: small, closed, and settled decades ago. RFC 2119 fixes MUST / SHOULD / MAY / MUST NOT / SHOULD NOT, and deontic logic has formalised the same triad (obligation, permission, prohibition) since the 1950s. `modality` is therefore a closed enum, and it is the one field in the data model that carries machine-usable meaning.

What it buys, precisely: `audit` can flag a pair whose modalities are incompatible — one `must` against one `must_not` on a similar subject — as a distinct, decidable signal rather than another similarity score. `check` surfaces it so an agent can weigh a MUST differently from a MAY.

What it does not buy, and must not claim to: conflict detection. Contradiction research distinguishes negation, antonym, replacement, switch, scope, and latent contradictions, and current methods miss contraries and subalterns entirely — "must be red" and "must be blue" conflict while both are `must`. The state of the art combining formal logic with LLMs detects roughly 60% of contradictions. Modality gives requiem one narrow, cheap, decidable slice of that space, which suits a tool that surfaces candidates and leaves adjudication to the agent. It is not a contradiction checker and will not become one.

`modality` is optional. Many `design` statements have no normative force at all — "we chose Postgres" is neither obligation nor permission — and forcing a value would produce noise. Statements written before this field existed simply have none, so there is no migration.

**Validate on write, tolerate on read.** `add`/`update` reject an unknown modality; the reader treats one as unset rather than erroring. Without this asymmetry, adding a member later would make every older binary reject files that use it — turning a closed enum into a forward-compatibility trap, which is the usual reason people avoid closing an enum at all.

## CLI

**Output convention:** bare data on stdout on success; nonzero exit code + error on stderr on failure. No wrapper envelope to unwrap on the common path. Every statement payload (in `get`, `add`, `update`, `link` output, and each entry returned by `list`/`check`) includes a computed `full_id` — the `<namespace>/<id>` composite — so output from one command can be passed straight into another's `<namespace/id>` argument without the caller concatenating fields itself.

| command | input | output |
|---|---|---|
| `init` | — | path, hooks installed, docs updated |
| `add` | `--id --namespace --kind --body [--modality] [--tags] [--provenance] [--source file:line]` | created statement |
| `update` | `<id> --body [--status] [--modality]` | updated statement |
| `link` | `<from-id> <to-id> --type [--note]` | confirmation |
| `reject` | `--id --namespace --body [--see-instead]` | created rejection |
| `get` | `<id>` | full statement incl. resolved relationships, `stale` flag if code-derived |
| `list` | `[--namespace] [--kind] [--status] [--tag]` | array of compact summaries |
| `check` | `--namespace --text [--tags] [--vector --model] [--limit]` | ranked array of compact candidates — id, namespace, kind, status, short excerpt, `match_kind` (`lexical`/`semantic`/`both`), `rank` (lower is more relevant; scale unspecified and comparable only within one result set). Statements and rejections included, distinctly tagged. Defaults to 10 results; `--limit 0` is unlimited. Full bodies are a deliberate second `get` call. |
| `embed` | `<id> --vector --model [--force]` | stored vector's id, model, dims, timestamp. Manual escape hatch; `reindex --embed` is the normal path. |
| `audit` | `[--namespace] [--min-score] [--limit]` | ranked array of candidate conflicting/duplicate pairs, excerpt-only, excluding pairs with any recorded relationship; pairs with incompatible modality flagged distinctly |
| `mv` | `<from-id> <to-id> [--leave-link]` | from, to, rewritten inbound references, whether a stub was left |
| `reindex` | `[--embed]` | counts: added/updated/removed/unchanged. With `--embed`, also fills missing/stale vectors; partial failure persists progress and exits nonzero. |
| `review` | — | human-readable description of staged changes |
| `commit` | `[--message]` | commit sha |
| `discard` | `[<id>]` | confirmation |

## Status

Implemented and in use: the full CLI surface above, incremental indexing, git hook installation, the stage/commit approval flow, agent doc generation, lexical and semantic retrieval, and corpus-wide audit.

Decided and specified above, not yet built: the embedding pipeline (`.requiem/config.yaml`, `reindex --embed`), RRF rank fusion, frequency-driven query-term filtering, partial-coverage warnings, and the `modality` field. Until the pipeline lands, vectors must be supplied via `embed --vector`.

Proposed, not yet decided: requirement–implementation traceability (see below).

---

## Proposed: Requirement–Implementation Traceability

**Status: proposed, not decided.** Recorded here because it is cheap to change on paper and expensive to change once identifiers are baked into commit history.

### The gap

Requiem can already point a statement *at* code: `provenance: code-derived` stores `file`, `line_range`, and a hash of that range, rehashed on read to flag drift. What it cannot do is answer the inverse question — *"this decision just changed; what code implements it?"* — which is the question that actually arises when a settled requirement is revisited.

These are not the same edge reversed. Their causality differs, and so does their decay:

| | `provenance: code-derived` | implementation label |
|---|---|---|
| means | this statement was reverse-engineered *from* existing code | this code was written *to satisfy* this statement |
| direction | statement → code | code → statement |
| survives refactor? | no — moving or reformatting the range breaks the hash | yes — the label lives inside the code that moved |
| fan-out | one range per statement | one statement, many labelled sites |

The second column is strictly better at the thing the first is worst at. A requirement satisfied across six files cannot be expressed as one `line_range`, but it is six comments. Conversely, a label asserts intent without verifying it, where a hash actually detects change. They are complements, not substitutes, and both should exist.

### Mechanism

Three carriers, chosen because they fail in non-overlapping ways:

- **Source comments** — `// requiem: auth/session/no-plaintext-tokens`. A marker prefix rather than a bare id, so the reference doesn't collide with an ordinary file path appearing in prose. Travels with the code through refactors at zero maintenance cost.
- **Commit trailers** — `Requiem-Id: auth/session/no-plaintext-tokens`, written by `requiem commit`. Git already has this convention (the same mechanism as `Co-Authored-By:`), it is parseable with `git interpret-trailers`, searchable with `git log --grep`, and immutable once written.
- **Tests, labelled the same way.** This is the strongest of the three and the least obvious. An implementation comment *asserts* that code satisfies a requirement; a labelled test that passes is *evidence* of it. Labelling tests turns traceability from documentation into verification, and is the only one of the three a machine can check.

### Scanning, not indexing

Requiem must not acquire a second index. Indexing the source tree would mean another manifest, another staleness problem, and a large expansion of what this tool owns — the same expansion it already refuses for filesystem watchers.

It does not need one. `git grep` is already available (requiem requires `git` on `PATH`) and answers the whole question in a single pass:

```
git grep --untracked -oh -E 'requiem: [a-z0-9/-]+' -- . ':(exclude).requiem'
```

That yields every referenced id with a count. Gitignored paths are skipped for free, so `node_modules` and build output never appear. `--untracked` means code an agent has just written counts before it is staged. The `:(exclude).requiem` pathspec keeps statements' own cross-references from registering as code references. No tree walking, no manifest, no binary-file handling, no dependency requiem does not already have.

**Where the scan runs matters more than how.** Measured cost is roughly 200ms on a small repository, worst case, and it grows with tree size — far too expensive for Trigger 1, the lazy reindex that precedes *every* `get`/`list`/`check`. So the scan runs on explicit `reindex` and on the installed git hooks (Trigger 2), never on the read path. The resulting counts therefore lag reality slightly between those points, which is acceptable precisely because this is a hint and not a claim; a stale hint costs an unnecessary glance, where a stale *assertion* would cost a wrong decision.

Commit-trailer references are deliberately *not* scanned. Walking history with `git log --grep` is far more expensive than one working-tree grep and has no place in an indexing path, so those stay an on-demand `trace` lookup. Code references are cheap and proactive; history references are expensive and pull-only.

### Why a count, surfaced by default

The obvious design is a boolean on `trace`, fetched when asked. Both halves of that are wrong.

*Pull-only is out of step with the rest of the tool.* `check` surfaces prior decisions before anyone thinks to look for them; `stale` and `embedding_status` appear on `get` and `list` unbidden. A reference hint belongs in that family — requiring an agent to know to ask for it is exactly the failure the rest of the design avoids.

*A boolean overstates what is known.* `code_refs: 3` invites a look at three specific places. `implemented: true` invites trust the data cannot support: a label records intent, not verification, and nothing checks that the labelled code does what the statement says. The field is named for what it counts, never for what it might imply.

**The ambiguity of zero is the real design problem.** No references can mean not implemented, implemented but unlabelled, or not implementable at all — "we chose Postgres" has no code site to point at. An agent that reads zero as "not implemented" has manufactured a confident answer out of missing data, which is the same failure as an `audit` that returns an empty list because nothing was embedded.

It takes the same answer, too. Below a threshold of adoption, labelling is uninitialised rather than informative, and `code_refs` reads as *unknown* rather than `0`. Only once a corpus genuinely uses labels does a zero begin to carry signal. The value's asymmetry should be assumed throughout: a nonzero count is useful evidence, and a zero is weak evidence at best.

### Classifying a reference

A scanned id is a bare string. Resolving it says what the reference *means*, and the resolution has to consult both statements and rejections — not statements alone.

That is not a completeness nicety. Resolve against statements only, and a rejection id found in source matches nothing and gets reported as a **dangling label**: "this points at something that no longer exists." The truthful report is the opposite in character — "this code implements an idea this project explicitly rejected." Same input, inverted meaning, and the only difference is one extra lookup. Getting it wrong turns the most interesting signal in the system into routine cleanup noise.

Five outcomes, of which two matter:

| resolves to | reading |
|---|---|
| active statement, refs > 0 | normal; nothing to say |
| active statement, refs 0 | weak — unbuilt, unlabelled, or unimplementable (see the ambiguity of zero above) |
| **non-active statement, refs > 0** | **code implements a rescinded decision** |
| **rejection, refs > 0** | **code implements an explicitly rejected idea** |
| nothing | dangling label; the reference rotted |

The two bold rows are the ones worth raising unprompted, and neither is expressible today. They describe the same underlying hazard: a decision was made and the code was never brought along, so the corpus reads as settled while the source still asserts the superseded position. The requirements look internally consistent precisely because the contradiction has been pushed into the code, where none of requiem's other checks can see it. This is the *undeclared prior constraint* from the Problem section, manufactured by the tool's own workflow rather than inherited from legacy code.

Note the asymmetry with staleness: the commit-date heuristic below would catch a superseded statement only incidentally, because a status change happens to be a commit to that file. Classifying by the target's status catches it directly and can say *why* it is suspect, which a date comparison never can.

### Derived staleness, again

A labelled site is suspect when the statement changed *after* the code did. Both dates are already in git: compare the statement file's last commit against each referencing file's last commit. Nothing is stored, nothing needs invalidating — the same read-time-comparison posture as `stale` and `embedding_status`. It is a heuristic (a file changes for unrelated reasons too), which is acceptable for the same reason the code-derived hash is: cheap for an agent to glance at and dismiss, versus the cost of a silently wrong assumption.

### Surface

- `code_refs` on `get`/`list`/`check` — how many labelled source sites reference this statement, or *unknown* where labelling is not yet in use. Derived, held only in the disposable index, never written back to the statement file: the same posture as `stale` and `embedding_status`, and for the same reason — a fact about a statement is not an edit to it.
- `requiem trace <namespace/id>` — the labelled source sites themselves, plus commits referencing this statement (the latter searched on demand, see above).
- **`update` reports the blast radius automatically.** This is the payoff, and it should not require remembering a separate command: changing a statement's body prints the sites and commits that referenced it, flagging those that predate the change. Everything else here is plumbing for this one behaviour.
- `audit` additionally surfaces the two strong signals above — code referencing a non-active statement, and code referencing a rejection — alongside dangling labels pointing at ids that resolve to nothing. Dangling labels are cleanup; the other two are contradictions between what the project has decided and what it currently does.
- Listing statements with no references, in a corpus where labelling is well covered, becomes possible for the first time. This is the closest thing the model has to an undefined symbol: something declared and never linked to anything. It is not expressible today at all.
- `mv` reports code references it cannot rewrite. This is the sharpest cost of the proposal: an id is already "stable once other statements may reference it", and once ids also live in source and in *immutable commit history*, renaming gets materially more expensive. `mv` can rewrite source comments; it can never fix a trailer in a published commit.

### Boundary

Traceability tooling has a strong pull toward compliance bureaucracy — this is the established shape of requirements-traceability practice in regulated software (DO-178C, IEC 62304, ISO 26262), and it is not the shape this should take. The existing principle holds the line: **infrastructure and retrieval, not a judge.** `trace` surfaces candidates. It must never enforce coverage, block a commit for an unlabelled change, or report a traceability percentage. The moment it scores you, it has become a different product.

That last prohibition constrains how the ambiguity-of-zero problem above is solved, and the constraint is worth stating because the two nearly collide. Making a zero interpretable requires knowing whether labelling is in use at all — but emitting "coverage: 43%" would be a traceability score in everything but name, and someone would start managing it. So adoption is expressed as a *state* that decides whether `code_refs` is meaningful, never as a number to move. The distinction is between calibrating a signal and grading the user.

These stay reports, and the distinction is sharper now that some of them look like violations. Code referencing a superseded statement is frequently a legitimate in-progress state: the decision landed on Tuesday, the migration ships on Friday, and both facts are true in the meantime. Surfacing that is useful. Refusing a commit over it would make requiem something developers route around, and a tool that gets routed around reports on a corpus nobody maintains.

Wholly optional and additive: a repository with zero labels behaves exactly as it does today.
