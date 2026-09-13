---
id: model-pinning
namespace: embedding
kind: rule
modality: must
status: active
provenance:
    type: dialogue
created_at: 2026-09-13T21:24:38.735414187Z
relationships:
    - to: embedding/committed-pipeline
      type: depends_on
      note: the pinned model is the one named in committed config
---

Every vector in a corpus comes from one model. Cosine similarity between vectors from two different models is a number that looks plausible and means nothing, so a mismatch is refused on write and on read. Re-pinning requires an explicit force and discards every existing vector.
