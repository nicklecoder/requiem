# requiem

A CLI for tracking requirements, rules, and design decisions ("statements")
per project, so an AI coding agent can check whether a new idea contradicts a
prior decision before building it.

Requiem is retrieval infrastructure, not a judge. It surfaces the narrow slice
of prior decisions relevant to what you are about to write; deciding whether
something genuinely conflicts stays with you or your agent.

## The problem it addresses

Spec-first development works until a project outgrows what one person or one
agent holds in mind at once. Past that point:

- Decisions get re-litigated because nobody remembers they were settled.
- Ideas that were explicitly rejected get re-proposed, because the rejection
  was a conversation and conversations evaporate.
- The same requirement gets written twice, in different words, in files
  authored independently — so nothing flags them as the same thing.
- Code keeps implementing a decision that was reversed months ago.

The constraint is rarely that the specification does not fit in context. It is
that having the text available is not the same as *noticing* the conflict. A
targeted lookup at the moment of writing catches what rereading does not.

## Quick start

```sh
go build -o requiem ./cmd/requiem
cd /path/to/your/project
requiem init
```

`init` creates `.requiem/`, installs git hooks, and appends a short workflow
summary to `AGENTS.md` and `CLAUDE.md` — after any existing content, never
replacing it — so an agent whose harness loads either learns requiem exists
without being told each session.

```sh
# Before proposing something non-trivial, look for prior decisions.
requiem check --namespace auth --text "should sessions use sliding expiry"

# Record a decision, and the alternative that lost.
requiem add --id no-plaintext-tokens --namespace auth/session --kind rule \
  --modality must_not --body "Session tokens are never stored in plaintext."
requiem reject --id sliding-expiry --namespace auth/session \
  --body "Sliding expiry. Rejected: unbounded blast radius on a leaked token." \
  --see-instead auth/session/no-plaintext-tokens

# Nothing is permanent until this. Writes auto-stage; discard backs them out.
requiem commit
```

Full reference: `requiem --help`, or `requiem man --dir man` for man pages.

## How it works

**Statement files are canonical and git-tracked**, one per statement under a
path mirroring its namespace. A SQLite index beside them is disposable and
rebuilt on demand, so deleting it loses nothing. Git carries history, diffing,
blame and revert; requiem adds only what git does not have.

**Commit is approval.** Every write auto-stages but never auto-commits, so
exploring a dead end and running `discard` leaves no trace in history.

**Rejections are first-class.** An idea explicitly considered and turned down
is recorded, so `check` can answer "we already looked at that, and here is
why" — the single thing most likely to save an agent's time.

### Fields worth knowing

| | |
|---|---|
| `--kind` | Open string: requirement, rule, design, whatever suits. Deliberately carries no semantics; it exists for grouping and retrieval. |
| `--modality` | Closed: `must`, `should`, `may`, `must_not`, `should_not`. The one field requiem reasons with — `audit` flags a pair whose modalities oppose. Optional. |
| `--status` | `proposed` → `active` → `superseded`/`deprecated`. A proposal is searched and audited like a decision, so you learn whether it conflicts before committing to it. |
| `--abstract` | No code can implement this: a principle, a process rule, or a prohibition — "must not do X" is implemented by absence, so there is no site to label. Keeps it out of `list --unreferenced`. |

## Semantic search

Lexical search cannot find a decision worded in vocabulary your draft does not
share — exactly the case when two people wrote the same requirement
independently. Point `.requiem/config.yaml` at any OpenAI-compatible
`/v1/embeddings` endpoint (Ollama, LM Studio, llama.cpp, vLLM, LocalAI, or
OpenAI):

```yaml
embedding:
  endpoint: http://localhost:11434/v1/embeddings
  model: mxbai-embed-large
  api_key_env: OPENAI_API_KEY   # the variable's NAME, never the key itself
```

```sh
requiem reindex --embed                      # fetch missing/stale vectors
requiem check --namespace auth --text "..." --semantic
requiem audit                                # corpus-wide duplicate/conflict sweep
```

That file is committed on purpose. Vectors live in the gitignored index and
cannot be rebuilt by reparsing statement files the way every other table can,
so the index is only genuinely disposable because the pipeline that reproduces
it is in version control.

Requiem bundles no model and no inference runtime — it calls a configured
endpoint, the same posture as shelling out to `git`. With none configured, the
commands that need vectors are hidden rather than offered and failing;
everything else works unchanged.

**A note on thresholds.** `audit` takes each statement's nearest neighbours
rather than every pair above a similarity cutoff, because an absolute
threshold does not survive a real corpus: every statement in one project
shares a vocabulary. Measured on a real 36-statement set, `0.5` surfaced 69%
of all pairs, while `0.85` — the usual near-duplicate cutoff in information
retrieval — found none of five planted paraphrases. Candidates are ranked by
CSLS, which corrects for the hubs high-dimensional embedding spaces
generically produce.

## Linking decisions to code

A marker comment names the statement a piece of code implements:

```go
// requiem: auth/session/no-plaintext-tokens
func storeToken(...) { ... }
```

You write that comment yourself, in whatever syntax the file uses. Requiem
deliberately has no command for inserting it: whoever is editing the file
already knows its language, where requiem would be guessing from a file
extension — and guessing wrong means writing invalid syntax into a file it
does not own. The scanner takes the other side of that trade and is
forgiving: spacing and capitalisation vary freely, so `Requiem : ns/id` is
found too.

Then `requiem trace <id>` reports where a decision lives, `update` prints
which sites a body change affects, and `audit` flags code still referencing a
superseded or rejected statement — a decision reversed while the code kept
asserting the old position.

Labels travel with code through refactors, which a stored line range cannot.
Labelling a **test** is stronger than labelling an implementation: a passing
labelled test is evidence a statement holds, where a comment only asserts
intent. Scanning is one `git grep`; requiem keeps no index of your source.

**Labels are a shortcut, not an obligation.** Nothing enforces them, and
nothing should — deciding whether a change *should* have carried one is a
judgment about intent, and a rule approximating it would be wrong often enough
to get disabled. `requiem trace <id> --search` answers from the statement's
own wording wherever a label is missing, so the tool is useful at zero
coverage and merely sharper as coverage rises.

Two conventions for the edges:

- `Requiem-Id: <namespace/id>` as a git trailer on a code commit records *when*
  a decision was implemented. Requiem reads these; it never writes them.
- `requiem:ignore` on a line stops the scanner treating it as a label — for a
  test fixture or a tutorial snippet, which are textually identical to the
  real thing.

## Status

Working, and in use on this repository, which tracks its own decisions in
`.requiem/`. Treat that as the worked example — try `requiem list` or
`requiem get principles/no-silent-success`.

It has not yet been proven on a large corpus somebody else wrote. The
mechanisms are measured: paraphrases written in entirely different words rank
their original first, and duplicate detection recovers them from a few percent
of candidate pairs. But that was a corpus authored in one sitting, with
duplicates planted deliberately. Whether it earns its keep on a real
multi-file specification is the open question.

## Build

Requires Go (see `go.mod` for the minimum version).

`CGO_ENABLED=0` cross-compiles cleanly to any Go-supported platform — the
SQLite index is pure Go (`modernc.org/sqlite`), so distribution is a single
static binary with no runtime dependency beyond `git` on `PATH`.

## Test

```sh
go test ./...
```

`internal/git` runs against real temporary git repositories rather than mocks,
since hook and staging behaviour has to match the real binary exactly.
Embedding tests use `httptest`, so nothing here needs a live endpoint.

See [SPEC.md](./SPEC.md) for the full design and the reasoning behind each
decision.
