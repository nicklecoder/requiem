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

`init` creates `.requiem/`, installs git hooks, and writes a short workflow
summary to `CLAUDE.local.md`, so Claude Code learns requiem exists without
being told each session. That file is yours alone — init excludes it through
`.git/info/exclude` — because one contributor adopting requiem is no reason to
change every contributor's agent instructions. A team that has adopted it runs
`requiem init --shared` to write the summary into `AGENTS.md` and `CLAUDE.md`
instead, after any existing content and never replacing it; `--local` moves it
back out.

`init` also connects the embedder, so requiem is fully working when it
returns. It uses the project's committed model (or `--embedding-model`, or the
machine config's default), finds an endpoint that answers a real request with
it — `--embedding-endpoint`, the one already configured, the server
`OLLAMA_HOST` names, then Ollama on localhost; it never scans the network —
saves it to the machine config so every project on the machine finds it, and
embeds the corpus. At a terminal it asks for what it cannot find; run by an
agent it never waits, and reports what is missing instead. `--skip-models`
leaves all of this alone.

`init` is safe to re-run, and a fresh clone needs it: hooks and the local
config are not carried by git. Re-running it also brings a project set up by
an older requiem up to date.

```sh
# Before proposing something non-trivial, look for prior decisions.
requiem check --text "should sessions use sliding expiry"
requiem check --namespace auth --text "..."   # narrow to one area and its children

# Record a decision, and the alternative that lost.
requiem add --id no-plaintext-tokens --namespace auth/session --kind rule \
  --modality must_not --body "Session tokens are never stored in plaintext."
requiem reject --id sliding-expiry --namespace auth/session \
  --body "Sliding expiry. Rejected: unbounded blast radius on a leaked token." \
  --see-instead auth/session/no-plaintext-tokens

# Nothing is permanent until this. Writes auto-stage; discard backs them out.
requiem commit
```

Two more commands worth knowing early:

```sh
# The decisions in force in an area — small enough to paste into a prompt.
requiem brief --namespace auth

# What bears on the change you already have, rather than on a draft.
requiem check --diff
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
why" — the single thing most likely to save an agent's time. First-class
literally: one file per rejection like any statement, carrying an embedding so
meaning-based search ranks it alongside decisions, with its `see_instead`
pointer validated on write and rewritten when the statement it names moves.

**`add` checks before it writes.** Recording a decision is the last moment the
corpus can be kept clean, so `add` runs the duplicate check itself and refuses
when something already says this, naming what it found. `--duplicate-ok`
records it anyway: requiem states a finding, you still decide.

### Fields worth knowing

| | |
|---|---|
| `--kind` | Open string: requirement, rule, design, `question`, whatever suits. Deliberately carries no semantics; it exists for grouping and retrieval. `--kind question --status proposed` is how an open question is recorded — no separate record type is needed, since `modality` is optional and a question carries no normative force. Answer it by adding the decision, `link`ing it at the question with `--type supersedes`, then setting the question `--status superseded`. |
| `--modality` | Closed: `must`, `should`, `may`, `must_not`, `should_not`. The one field requiem reasons with — `audit` flags a pair whose modalities oppose. Optional. |
| `--status` | `proposed` → `active` → `superseded`/`deprecated`. A proposal is searched and audited like a decision, so you learn whether it conflicts before committing to it. An open question belongs here, never hedged into an active statement's body — `list --status proposed` is the only queue that lists it. |
| `--abstract` | No code can implement this: a principle, a process rule, or a prohibition — "must not do X" is implemented by absence, so there is no site to label. Keeps it out of `list --unreferenced`. |

## Semantic search

Lexical search cannot find a decision worded in vocabulary your draft does not
share — exactly the case when two people wrote the same requirement
independently. Name a model in `.requiem/config.yaml`, and in
`.requiem/config.local.yaml` point at any OpenAI-compatible `/v1/embeddings`
endpoint serving it (Ollama, LM Studio, llama.cpp, vLLM, LocalAI, or OpenAI):

```yaml
# .requiem/config.yaml — committed
embedding:
  model: mxbai-embed-large

# .requiem/config.local.yaml — gitignored, overlaid field by field
embedding:
  endpoint: http://localhost:11434/v1/embeddings
  api_key_env: OPENAI_API_KEY   # the variable's NAME, never the key itself
```

An endpoint every project on a machine shares can be set once instead, in the
machine config: `~/.config/requiem/config.yaml` on Linux,
`~/Library/Application Support/requiem/config.yaml` on macOS. It sits beneath
both project files, and holds only how to reach the endpoint — a `model` there
is never used by a project that names none, since two clones must never embed
one corpus with different models.

```sh
requiem reindex --embed                      # fetch missing/stale vectors
requiem check --text "..." --semantic
requiem audit                                # corpus-wide duplicate/conflict sweep
```

The model is committed on purpose. Vectors live in the gitignored index and
cannot be rebuilt by reparsing statement files the way every other table can,
so the index is only genuinely disposable because the model that reproduces it
is in version control. Where that model is served from names a machine, so it
stays local.

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

**And a note on what similarity cannot do.** On a 255-statement corpus built
from a real project's history, nearest-neighbour search over 1,936 candidate
pairs missed three genuine conflicts — and all three shared an identifier
while sharing almost no wording. So `audit` builds pairs from shared
identifiers and shared source files first, and `check --touches
external_venues.status` retrieves by identifier directly. Opposed modality is
only a tiebreak: ranking by it put 48 unrelated pairs in the top 50, because a
`must` set against a `must_not` on different subjects is ordinary.

Every `check` result also carries a `verdict` — `duplicate`, `related` or
`weak` — with the evidence behind it. `check` always fills its limit, so the
number of results never meant anything; a result set that is entirely `weak`
is the answer "nothing here states this already".

Record what an audit turns up: `link --type conflicts_with|duplicates` for a
real finding, and `dismiss <a> <b>` for a pair you have judged unrelated.
Dismissals are stored as verdicts rather than edges, so bookkeeping never
accumulates in the graph you read to understand how decisions relate.

## Optional classifier

Embedding cosine cannot tell a conflict from a compatible pair among the pairs
`audit` surfaces: on one real corpus it separated them no better than chance
with mxbai-embed-large, and only moderately with qwen3-embedding-8b. A model
that reads both statements together can. Configure one and `audit` puts the
likely contradictions first; it only orders the queue and never hides a pair.
requiem runs fully without one.

Two kinds are supported, and requiem recommends neither — measured candidates
are listed below so you can choose knowingly:

```yaml
# ~/.config/requiem/config.yaml, or .requiem/config.local.yaml — never the
# committed config.yaml: which classifier to run is each user's choice
classifier:
  kind: nli                          # a server answering POST /v1/nli
  endpoint: http://192.168.1.111:11436

# or any OpenAI-compatible chat endpoint with logprobs, such as Ollama:
classifier:
  kind: chat
  endpoint: http://localhost:11434
  model: gemma4:latest
  # reasoning_effort: omit           # if your server rejects the field
```

An `nli` server takes `{"pairs": [{"premise", "hypothesis"}]}` and answers one
`{"entailment", "neutral", "contradiction"}` set per pair. A `chat` model is
asked one question with options A (no) and B (yes) and scored from the
logprob of its answer letter.

Measured on the sbs corpus (91 known conflicts, 945 compatible pairs):

| Model | Kind | Conflicts in the top tenth of the queue | Notes |
|---|---|---|---|
| `MoritzLaurer/DeBERTa-v3-large-mnli-fever-anli-ling-wanli` (435M) | nli | 78% | Weights labelled MIT; trained partly on ANLI, which is CC BY-NC 4.0 — whether that carries to the weights is unsettled. About 9 pairs a second on an M4 Mac Mini. |
| `cross-encoder/nli-deberta-v3-large` (435M) | nli | 57% | Apache-2.0; trained on SNLI and MultiNLI. |
| `gemma4` 8B via Ollama | chat | about the same as the first row | Ranked the one real defect found first where the NLI model ranked it 79th; no server beyond Ollama; about 0.7 s a pair on the Mini. |

Neither kind of server is Ollama's own embedding endpoint, and requiem ships,
downloads and defaults no classifier model. `requiem init` proves a configured
classifier and says nothing when none is configured.

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

It has now had one trial outside this repository: an agent reconstructed a
corpus of 255 statements from another project's git history and code, then
planned work against it. Retrieval held up — it found 95% of the overlaps the
author had predicted, and seven they had not — and the corpus measurably
improved the plan. The failures were elsewhere, and all of them are fixed
above: rejections were second-class in the very mechanism meant to surface
them, `check` could not say whether anything actually matched, dismissals were
accumulating in the graph as permanent noise, and nothing answered "what
decisions bear on this patch" at the moment a change existed.

What remains unproven is the long run: whether a corpus stays accurate across
months of drift, and whether the identifier index earns its keep in a project
whose vocabulary requiem cannot know in advance.

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
