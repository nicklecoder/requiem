---
id: audit-question-rule-noise
namespace: retrieval
kind: question
status: proposed
provenance:
    type: dialogue
created_at: 2026-09-24T21:08:46.444041299Z
relationships:
    - to: retrieval/audit-pairs-share-identifiers
      type: depends_on
      note: Would change how audit orders its candidate pairs.
---

Open question: should audit rank lower a pair made of a proposed question and the rule it gates in the same namespace? In a field corpus most of 236 dismissals out of 387 audit pairs had that shape, and 2 pairs were real findings. What hangs on it: the answer to a question can conflict with the rule it gates, which is exactly the finding a lower rank would hide, so this wants a second corpus showing the same pattern first.
