---
id: batch-isolation
namespace: embedding
kind: rule
modality: must
status: active
provenance:
    type: dialogue
created_at: 2026-09-13T21:24:38.742123393Z
relationships:
    - to: embedding/partial-failure-visible
      type: depends_on
      note: resumability is what makes per-item retry worth its round trips
---

When a batch fails, its members are retried individually. Batching is deterministic, so without this a permanently-failing input re-forms the same doomed batch on every retry and its healthy batchmates can never succeed.
