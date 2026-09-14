<!-- >>> requiem v6 >>> -->
## requiem

This project tracks requirements, rules, and design decisions ("statements")
in `.requiem/` using requiem, so you can check a new idea against prior
decisions without loading the whole spec into context.

**Before proposing anything non-trivial**, look for prior decisions and
previously-rejected ideas:

```sh
requiem check --namespace <area> --text "<the idea>"
```

Returns ranked, excerpt-only candidates (top 10; `--limit` to change), each
tagged `statement` or `rejection`. Call `requiem get <namespace/id>` for the
full body of one that actually matters.

**Recording outcomes.** Writes auto-stage in git but never auto-commit, so a
dead end leaves no trace if you back out:

- `requiem add` / `update` / `link` — decisions that stand
- `requiem reject` — ideas explicitly considered and rejected, so they aren't re-proposed
- `requiem mv <from> <to>` — relocate a statement, rewriting inbound references
- `requiem review` — describe what is staged but not yet committed
- `requiem discard [<id>]` — unstage and revert; everything pending if no id given
- `requiem commit` — the approval step; nothing is permanent until this runs

Two optional fields on `add`/`update`:

- `--modality must|should|may|must_not|should_not` — normative force. The one
  field requiem reasons with: `audit` flags a pair whose modalities oppose.
- `--status proposed` — a decision under consideration rather than in force.
  Searched and audited like a settled one, so you find out whether a proposal
  conflicts with something before committing to it. `list --status proposed`
  is the queue of open decisions.

`requiem get` also reports what points *at* a statement: `referenced_by` for
statements refining or depending on it, and `rejected_alternatives` for ideas
turned down in its favour — read those before re-proposing something.

**Semantic matching.** Lexical search cannot find a prior decision worded in
vocabulary your draft doesn't share. Embeddings close that gap. If
`.requiem/config.yaml` names an embedding endpoint, requiem fetches vectors
itself:

```sh
requiem reindex --embed      # fill every missing or stale vector
```

- Vectors live in the gitignored index and **do not survive a clone**. Run
  `requiem reindex --embed` after cloning; `requiem list --needs-embedding`
  shows what is missing.
- A partial run exits nonzero and keeps whatever succeeded — re-running
  retries only the failures.
- `audit` and `check` warn on stderr when part of the corpus is unembedded,
  because a short result list would otherwise be indistinguishable from a
  thorough one.

Add `--semantic` to search by meaning as well as vocabulary — requiem embeds
the query text for you:

```sh
requiem check --namespace <area> --text "<the idea>" --semantic
```

It is opt-in because a plain `check` stays offline and fast. Without an
endpoint, supply your own with `--model <name> --vector <json>`; requiem
refuses a vector from a different model, since cosine similarity across two
models is meaningless.

`requiem audit` sweeps the whole corpus for pairs that may conflict or
duplicate each other. Record every verdict with
`requiem link <a> <b> --type conflicts_with|duplicates|not_related` so the
pair stops resurfacing.

**Linking decisions to code.** Label the code that implements a statement so a
changed decision can report what it affects:

```go
// requiem: auth/session/no-plaintext-tokens
func storeToken(...) { ... }
```

Labels travel with the code through refactors, and labelling a *test* is
stronger than labelling an implementation: a passing labelled test is evidence
the statement holds, where a comment only asserts intent.

- `requiem trace <namespace/id>` — the labelled sites referencing a statement.
- `requiem update` prints which sites a body change affects.
- `requiem audit` flags code still referencing a superseded or rejected
  statement — the decision changed and the code was never brought along.
- `get`/`list` show `code_refs`, a count, absent where labelling is unused.
- `requiem list --unreferenced` finds statements no code implements. A
  statement counts as covered when the statements refining it are labelled;
  `--direct` suppresses that inference.
- A label in a `.md` or `.txt` file is a *mention*, not a reference: it does
  not count toward `code_refs`, but a changed decision still flags it.
- A mistyped id is reported by `audit` and by `reindex`, which exits nonzero.

`requiem mv` rewrites labels to the new id and leaves those edits unstaged,
so review them with `git diff`. `--no-rewrite-refs` reports them instead.

A code commit can also name the decision it implements, which records *when*
something was built rather than where it lives now:

```
Enforce single-model embedding

Requiem-Id: embedding/model-pinning
```

`requiem trace` reports those commits. Requiem only reads them — you write
them on your own commits.

Full reference: `requiem --help`.
<!-- <<< requiem <<< -->
