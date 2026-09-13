<!-- >>> requiem v4 >>> -->
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
- `requiem commit` — the approval step; nothing is permanent until this runs

Pass `--modality must|should|may|must_not|should_not` when a statement carries
normative force. Optional, and the one field requiem reasons with: `audit`
flags a pair whose modalities oppose each other.

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

Full reference: `requiem --help`.
<!-- <<< requiem <<< -->
