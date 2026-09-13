---
id: modality-closed
namespace: model
kind: rule
modality: must
status: active
provenance:
    type: dialogue
created_at: 2026-09-13T21:24:38.755969399Z
relationships:
    - to: model/validate-write-tolerate-read
      type: depends_on
      note: closing the enum is only safe given tolerant reads
---

Normative strength is a closed, optional enum: must, should, may, must_not, should_not. Unlike category, strength was settled decades ago by RFC 2119 and deontic logic before it. It is the one field requiem reasons with, and it is optional because many design statements carry no normative force at all.
