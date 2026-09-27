---
id: audit-bodies-flag
namespace: retrieval
kind: rule
modality: should
status: active
provenance:
    type: dialogue
created_at: 2026-09-27T16:48:05.028396893Z
relationships:
    - to: retrieval/audit-export-with-bodies
      type: supersedes
      note: answers the question as an opt-in flag
    - to: cli/batch-dismiss
      type: depends_on
      note: verdicts return through batch
    - to: retrieval/audit-orders-by-pair-classifier
      type: depends_on
      note: the agent reviews the queue from the top
---

audit --bodies emits one JSON line per candidate pair, in queue order and bounded by --limit, carrying both full bodies alongside the pair's ids and evidence, so the calling agent can judge each pair from one read and send its verdicts back through batch as dismiss and link records. It is opt-in; plain audit output keeps excerpts. Why: audit is a triage queue the agent works from the top (retrieval/audit-orders-by-pair-classifier), and judging from excerpts costs two get calls per pair; a field agent hand-built this export for a 387-pair first audit.
