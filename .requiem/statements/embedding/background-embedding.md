---
id: background-embedding
namespace: embedding
kind: rule
modality: should
status: active
provenance:
    type: dialogue
created_at: 2026-09-28T17:11:27.579296447Z
relationships:
    - to: embedding/vectors-per-model
      type: depends_on
      via: link
    - to: embedding/coverage-warning
      type: depends_on
      via: link
    - to: embedding/unreachable-endpoints-remembered
      type: depends_on
      via: link
    - to: embedding/read-path-offline
      type: depends_on
      note: only commands already reaching for the embedder start a background run
      via: batch
    - to: embedding/partial-failure-visible
      type: depends_on
      note: a background run keeps what succeeded and retries the rest next time
      via: batch
---

When a command that already uses the embedder (check --semantic, audit, init) finds the active model's vector set incomplete, because the model changed or statements were added or edited, and an endpoint for that model is reachable, it starts requiem reindex --embed as a detached background process and answers at once from what the index already holds, with the coverage warning saying the set is incomplete. A command that stays offline (plain check, get, list, and plain reindex, which the git hooks run) never starts one (embedding/read-path-offline). One background run at a time: a lock in the index keeps a second command from starting another, and a run that finds nothing to do exits. Why: a switch of model costs from seconds to minutes to re-embed, and nobody should have to remember to run it; but filling vectors before answering puts network waits and unbounded latency in check's path, which was rejected as embedding/auto-embed-on-read, and a detached run keeps that wait out of every command.
