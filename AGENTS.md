<!-- >>> requiem v10 >>> -->
## requiem

This project tracks requirements, rules, and design decisions ("statements")
in `.requiem/` using requiem, so you can check a new idea against prior
decisions without loading the whole spec into context.

### Working process

1. **Before proposing anything non-trivial**, run `check` on the relevant
   namespace. Read the `rejection` results first — those are ideas this
   project already considered and turned down, and re-proposing one is the
   most common way to waste the human's time.
2. If a result conflicts with what you were about to do, say so rather than
   silently picking a side. Requiem surfaces candidates; judging them is
   your job, and reporting the conflict is part of judging it.
3. When a decision is made, `add` it — with `--modality` if it carries
   normative force, `--status proposed` if it is not settled yet.
4. `link` it to the principle it refines, so the graph is navigable from
   both ends, and `reject` the alternatives that lost with `--see-instead`
   pointing at what replaced them. A rejection costs one command and saves
   the next agent from re-deriving it.
5. While implementing, run `requiem label <namespace/id> <file>:<line>` on
   the code that carries the decision. Labelling a **test** is stronger than
   labelling an implementation: a passing labelled test is evidence the
   statement holds, where a comment only asserts intent.

   Labels are a shortcut, not an obligation. Nothing enforces them, and
   partial coverage is still useful: a label is an exact answer, and
   `requiem trace <id> --search` answers from the statement's own wording
   wherever a label is missing. Label what you can; do not stall on it.
6. Periodically run `requiem reindex --embed` then `requiem audit`, and
   record a verdict on every pair it surfaces. An adjudicated pair stops
   resurfacing and frees the slot for the next candidate.
7. `requiem commit` when the decision is settled. Nothing is permanent
   until then, so exploring a dead end and running `discard` leaves no trace.

**Writing a statement that can be found again.** State the decision *and*
why. "Use Postgres" is not retrievable; "Session state lives in Postgres
rather than Redis, because it must survive a restart" matches a future draft
that shares neither word. Retrieval works on the body, so a body that omits
the reasoning cannot match a draft that arrives at it differently.

### Commands

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
duplicate each other, taking each statement's nearest neighbours rather than
everything above a similarity cutoff — on a real corpus every statement
shares a vocabulary, so an absolute threshold surfaces most of it or none.
Record every verdict with
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
  Add `--search` to also find files sharing the statement's vocabulary,
  which works before anything has been labelled at all.
- `requiem label <namespace/id> <file>:<line>` — insert the marker, with the
  right comment syntax for that file. Refuses an id that does not exist, so
  a typo cannot become a dangling reference.
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
- Writing *about* a label — a test fixture, a tutorial snippet — would
  otherwise scan as a real one. Put `requiem:ignore` on the line, with an
  optional reason after it.
- `--abstract` on a statement no code can implement (a principle, a process
  decision) keeps it out of `--unreferenced`. `audit` flags it if code turns
  up referencing it anyway.

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
