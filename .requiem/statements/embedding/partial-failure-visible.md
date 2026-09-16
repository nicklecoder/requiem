---
id: partial-failure-visible
namespace: embedding
kind: rule
modality: must
status: active
provenance:
    type: dialogue
created_at: 2026-09-13T21:24:38.738697905Z
relationships:
    - to: principles/no-silent-success
      type: refines
      note: a partial run must not exit zero
    - to: cli/diagnostics-stderr
      type: refines
      note: cli/diagnostics-stderr names partial-failure summaries as one of its two examples; this is that rule applied to the embedding run.
---

A partial embedding run keeps every vector that succeeded, reports failures grouped by reason on stderr, and exits nonzero. Embedding is keyed on the body hash, so re-running resumes rather than restarting.
