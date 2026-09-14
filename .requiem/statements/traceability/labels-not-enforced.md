---
id: labels-not-enforced
namespace: traceability
kind: rule
modality: must_not
abstract: true
status: active
provenance:
    type: dialogue
created_at: 2026-09-14T02:26:04.018218441Z
relationships:
    - to: principles/retrieval-not-judge
      type: refines
      note: enforcing would require judging intent
---

Requiem must not enforce code labels. Deciding whether a change should have carried one is a judgment about intent, which this tool refuses to make; any rule approximating it would be wrong often enough to be disabled, leaving neither enforcement nor labels.
