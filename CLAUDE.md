<!-- >>> requiem v22 >>> -->
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

   **Never write the open part into the body of an active statement.** A
   body that says "undecided", "open whether" or "for now" is a question
   filed where nothing will look for it: `list --status proposed` is the
   only queue of open decisions, and an active statement is not in it. The
   question then goes stale silently the moment it is answered elsewhere.
   Split it — the settled part `active`, the open part `proposed` — or
   record the whole statement as `proposed`. Requiem's own corpus carried
   four such bodies, every one long since shipped, while the queue read
   zero.
4. `link` it to the principle it refines, so the graph is navigable from
   both ends, and `reject` the alternatives that lost with `--see-instead`
   pointing at what replaced them. A rejection costs one command and saves
   the next agent from re-deriving it.
5. While implementing, write `requiem: <namespace/id>` in a comment above
   the code that carries the decision — an ordinary comment, in that
   file's own syntax, written by you. Requiem has no command for this on
   purpose: you are editing the file and know its language, where requiem
   could only guess from the extension. Labelling a **test** is stronger
   than labelling an implementation: a passing labelled test is evidence
   the statement holds, where a comment only asserts intent.

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
requiem check --text "<the idea>"
```

Returns ranked, excerpt-only candidates (top 10; `--limit` to change), each
tagged `statement` or `rejection`. Call `requiem get <namespace/id>` for the
full body of one that actually matters.

`--namespace <area>` narrows to one namespace and its children. Omit it and
check searches every namespace, which is the better default: a cross-area
decision is the one you are least likely to think of looking for, and a
wrong guess at the scope returns an empty result that reads as "nothing
here states this".

Each candidate carries a `verdict` — `duplicate`, `related` or `weak` — with
the evidence behind it: the cosine where a vector was compared, and the
identifiers it shares with your draft. A result set that is entirely `weak`
is the answer "nothing here states this already"; check always fills its
limit, so the count of results never meant anything on its own.

`--touches <identifier>` retrieves by identifier — a column, field or symbol
the change touches — which finds a prior decision about
`external_venues.status` even when it was written in words your draft does
not use.

**Checking a change rather than a draft.** `requiem check --diff` takes the
patch instead: it reports the decisions whose labels sit in an edited hunk,
the ones whose own source range you touched, and any record naming an
identifier your change adds — rejections listed separately, since walking
back into a rejected idea is the thing most worth catching. `--staged` and
`--diff-rev <range>` scope it elsewhere. Failing a build on it is opt-in per
project (`gate.diff` in `.requiem/config.yaml`), and even then it fails only
on facts: a label pointing at a retired or rejected decision, a covering
statement whose source range has drifted, and a decision whose last label
the patch removes.

That last one is reported under `dropped`, and it is the reason to run this
on a change you did not write by hand. A regeneration that rewrites a file
drops the comments in it, and a decision with no label left is one `trace`
can no longer answer for — a scan of what the patch leaves behind cannot
tell that from a decision nobody ever labelled. A label that merely moved
between files is not reported: the tree is rescanned after the change, so
a label that travelled is still found.

**Starting work in an area.** `requiem brief --namespace <area>` returns the
minimal set in force there: must/must_not rules, the principles they refine,
and what has already been rejected. Small enough to paste into a prompt, and
it counts what the cap left out instead of hiding it.

**Recording outcomes.** Writes auto-stage in git but never auto-commit, so a
dead end leaves no trace if you back out:

- `requiem add` / `update` / `link` — decisions that stand. `add` runs the
  duplicate check itself and refuses when the corpus already says this,
  naming what it found; `--duplicate-ok` records it anyway.
- `requiem batch` — many writes as JSON Lines on stdin, one `{"op":...}` per
  line, with a per-record result for each. A malformed line writes nothing;
  a refused write is reported against its line while the rest apply.
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
  is the queue of open decisions — and the only one, which is why an open
  question must never be written into an active statement's body instead.

**Recording an open question.** There is no separate question record and none
is needed: `kind` is a free string and `modality` is optional, so

```sh
requiem add --namespace <area> --id <slug> --kind question --status proposed \
  --body "Open question: ... . What hangs on it: ..."
```

is a question, and `list --status proposed` is the queue that holds it.

Answer it by adding the decision, then:

```sh
requiem link <decision> <question> --type supersedes
requiem update <question> --status superseded
```

Both steps are needed: `link` records the edge and deliberately leaves the
target's status alone, because a decision can supersede another while that
one stays in force through a migration window. It says so on stderr, since
an `active` statement keeps turning up in `check` and `audit` as a decision
still in force.

That keeps the question's history instead of erasing it. Six research agents on
one real project produced about 120 open questions and recorded five, because
writing one looked like it needed a normative force it does not have; two
defects later traced back to questions that never entered the corpus.

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
requiem check --text "<the idea>" --semantic
```

It is opt-in because a plain `check` stays offline and fast. Without an
endpoint, supply your own with `--model <name> --vector <json>`; requiem
refuses a vector from a different model, since cosine similarity across two
models is meaningless.

`requiem audit` sweeps the whole corpus for pairs that may conflict or
duplicate each other. Pairs that share an identifier — or come from the same
source file — rank first, because that is evidence about subject matter
rather than a guess from wording; the rest are each statement's nearest
neighbours by embedding. Opposed modality is only a tiebreak: ranking by it
put 48 unrelated pairs in the top 50 of a real audit.

The stderr line reports two numbers: pairs outstanding, and how many
statements have been swept at the current depth. The second is the one that
rises as you work, because each statement's window is fixed — judging a pair
removes it for good rather than promoting the next-nearest neighbour into its
place. Once a namespace is clear, `--neighbors 5` sweeps deeper.

Record what you find:

- `requiem link <a> <b> --type conflicts_with|duplicates` — a real finding,
  which belongs in the graph.
- `requiem dismiss <a> <b> --note "..."` — seen and unrelated. Stored as a
  verdict, not an edge, so dismissals never accumulate in the graph you read
  to understand how decisions fit together. `--restore` takes one back.

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
- Write the marker yourself, in the file's own comment syntax. Spacing and
  capitalisation are tolerated — `Requiem : ns/id` is found — but the id
  must be exact. Copy `full_id` from `add` or `check` output rather than
  retyping it.
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
- `--abstract` on a statement no code can implement keeps it out of
  `--unreferenced`. Three kinds qualify: a principle, a decision about
  process rather than software, and a **prohibition** — "requiem must not
  do X" is implemented by absence, so there is no site to label. A
  prohibition enforced by a specific guard is the exception: label the
  guard. `audit` flags an abstract statement if code references it anyway.
  It is an assertion nothing can verify, so `list --abstract` shows every
  one of them: review them the way you would review a file's nolint
  pragmas, because a statement marked abstract to quiet `--unreferenced`
  hides a real gap permanently.
- `list --unreferenced` reports only live decisions. A superseded or
  deprecated one has no implementation because it was withdrawn, which is
  true and useless.

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
