# requiem

An agent-native CLI for tracking requirements, rules, and design decisions
("statements") on a per-project basis, so an AI coding agent can cheaply
check whether a new idea conflicts with a prior decision without loading an
entire spec into context. It's retrieval infrastructure, not a judge —
reasoning about conflicts stays with the calling agent.

See [SPEC.md](./SPEC.md) for the full design and rationale.

## Build

Requires Go (see `go.mod` for the minimum version).

```sh
go build -o requiem ./cmd/requiem
```

`CGO_ENABLED=0` cross-compiles cleanly to any Go-supported platform — the
SQLite index is pure Go (`modernc.org/sqlite`), so distribution is a single
static binary with no runtime dependency beyond `git` on `PATH`.

## Usage

Run from a project's root, alongside its own git repository. `init` also
writes a short workflow summary into `AGENTS.md`/`CLAUDE.md` (appending
after any existing content), so an agent whose harness loads either at
session start learns requiem exists and how to use it automatically:

```sh
requiem init                                               # sets up .requiem/, installs reindex hooks

requiem add --id no-plaintext-tokens --namespace auth/session --kind rule \
  --body "Session tokens are never stored in plaintext." --tags security \
  --modality must_not                                      # optional normative strength
requiem add --id code-labels --namespace traceability --kind design \
  --status proposed --body "..."                            # a decision still under consideration

requiem check --namespace auth --text "should tokens expire on inactivity"  # surfaces related prior decisions
                                                           # top 10 by default; --limit to change
                                                           # --semantic also matches by meaning

requiem get auth/session/no-plaintext-tokens
requiem list --namespace auth
requiem update auth/session/no-plaintext-tokens --status deprecated
requiem link auth/session/no-plaintext-tokens auth/session/other --type depends_on
requiem reject --id sliding-session-expiration --namespace auth/session \
  --body "Proposed sliding expiration. Rejected: unbounded blast radius on leak." \
  --see-instead auth/session/no-plaintext-tokens

requiem reindex --embed                                     # fetch every missing/stale vector
requiem embed auth/session/no-plaintext-tokens \
  --model all-minilm --vector "$(...)"                      # manual fallback when no endpoint is configured
requiem audit --namespace auth                              # sweep for conflicting/duplicate pairs
requiem mv auth/session/old auth/shared/new                 # relocate, rewriting inbound references

requiem review                                              # optional — inspect staged changes
requiem commit                                               # the approval step
requiem discard [namespace/id]                                # unstage + revert; all pending if no id given

requiem trace auth/session/no-plaintext-tokens               # labelled code sites implementing it

requiem reindex                                              # rarely needed explicitly — get/list/check already do this
                                                             # (also rescans source labels; the read path never does)
```

Every command writes bare JSON to stdout on success; failures go to stderr
with a nonzero exit code. Diagnostics that don't belong in the payload —
incomplete embedding coverage, for instance — also go to stderr, so `jq`
pipelines stay clean while an agent reading combined output still sees them.

## Linking decisions to code

A marker comment names the statement a piece of code implements:

```go
// requiem: auth/session/no-plaintext-tokens
func storeToken(...) { ... }
```

`requiem trace <id>` finds those sites, `update` reports which of them a body
change affects, and `audit` flags code still referencing a superseded or
rejected statement — the case where a decision changed and the code was never
brought along, so the corpus reads as settled while the source still asserts
the old position.

Labels travel with code through refactors, which a stored line range cannot.
Labelling a test is stronger than labelling an implementation: a passing
labelled test is evidence a statement holds, where a comment only asserts
intent. Scanning is one `git grep` — requiem keeps no index of your source.

Entirely optional; a repository with no labels behaves exactly as before.

## Embeddings

Lexical search can't find a prior decision worded in vocabulary your draft
doesn't share, which is exactly the drift this tool exists to catch. Point
`.requiem/config.yaml` at any OpenAI-compatible `/v1/embeddings` endpoint —
Ollama, LM Studio, llama.cpp, vLLM, LocalAI, or OpenAI — and `reindex --embed`
fills in the rest:

```yaml
embedding:
  endpoint: http://localhost:11434/v1/embeddings
  model: nomic-embed-text
  api_key_env: OPENAI_API_KEY   # the variable's name, never the key itself
```

That file is committed on purpose. Vectors live in the gitignored index and
can't be rebuilt by reparsing statement files the way every other table can —
so the index is only genuinely disposable because the pipeline that
reproduces it is in version control. `requiem init` writes a commented-out
template; requiem bundles no model and no inference runtime, so the binary
stays static and dependency-free.

## Test

```sh
go test ./...
```

`internal/git`'s tests run against real temporary git repositories (not
mocked), since hook and staging behavior needs to match the real `git`
binary exactly.
