---
id: add-checks-before-writing
namespace: model
kind: rule
modality: must
status: active
provenance:
    type: dialogue
created_at: 2026-09-15T23:50:00.927313202Z
relationships:
    - to: principles/retrieval-not-judge
      type: refines
      note: The refusal is overridable, which is what keeps the judgment with the agent.
---

add runs check against the draft body before writing and refuses when an existing record comes back as a duplicate, naming what it found; --duplicate-ok records it anyway. Recording a decision is the last moment the corpus can be kept clean, and leaving the check to the agent meant it was skipped: a real ingestion of 255 statements produced 54 duplicates, and the method built around it — load a base, check each batch, merge by hand — existed only to compensate for a step the tool can perform itself. The check uses lexical and identifier evidence and never reaches the network, because a write path that hangs when an embedding endpoint is down is worse than one that misses a differently-worded duplicate; check --semantic stays the way to find that one.
