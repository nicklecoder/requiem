---
id: per-sentence-vectors
namespace: embedding
kind: question
status: deprecated
provenance:
    type: dialogue
created_at: 2026-09-24T21:08:46.516225155Z
---

Open question: should a body be embedded per sentence and scored by its best sentence, or given a separate short claim line to embed, instead of one vector for the whole body? Bodies dense with citations and enum values dilute a single vector, a field report suggests. What hangs on it: several vectors per record multiply storage and endpoint calls, and the report tested one model on one corpus, so the dilution needs confirming with another model first.
