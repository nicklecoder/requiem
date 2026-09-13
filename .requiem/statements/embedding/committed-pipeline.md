---
id: committed-pipeline
namespace: embedding
kind: rule
modality: must
status: active
provenance:
    type: dialogue
created_at: 2026-09-13T21:24:38.732368017Z
relationships:
    - to: principles/files-canonical
      type: refines
      note: what makes the index genuinely disposable for vectors
---

The embedding endpoint and model live in a committed config file. Vectors cannot be rebuilt by reparsing statement files the way every other index table can, so the index is only genuinely disposable because the pipeline that reproduces it is in version control.
