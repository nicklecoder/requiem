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
  --body "Session tokens are never stored in plaintext." --tags security

requiem check --namespace auth --text "should tokens expire on inactivity"  # surfaces related prior decisions

requiem get auth/session/no-plaintext-tokens
requiem list --namespace auth
requiem update auth/session/no-plaintext-tokens --status deprecated
requiem link auth/session/no-plaintext-tokens auth/session/other --type depends_on
requiem reject --id sliding-session-expiration --namespace auth/session \
  --body "Proposed sliding expiration. Rejected: unbounded blast radius on leak." \
  --see-instead auth/session/no-plaintext-tokens

requiem review                                              # optional — inspect staged changes
requiem commit                                               # the approval step
requiem discard [namespace/id]                                # unstage + revert; all pending if no id given

requiem reindex                                              # rarely needed explicitly — get/list/check already do this
```

Every command writes bare JSON to stdout on success; failures go to stderr
with a nonzero exit code.

## Test

```sh
go test ./...
```

`internal/git`'s tests run against real temporary git repositories (not
mocked), since hook and staging behavior needs to match the real `git`
binary exactly.
