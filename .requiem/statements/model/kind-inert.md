---
id: kind-inert
namespace: model
kind: rule
modality: must
status: active
provenance:
    type: dialogue
created_at: 2026-09-13T21:24:38.752570226Z
relationships:
    - to: model/modality-closed
      type: refines
      note: category stays open precisely because strength closes cleanly
---

The kind field stays an open string and carries no semantics. It exists for grouping and retrieval. Category resists closure — the functional/non-functional boundary is unclear in practice, so agents asked to pick one value answer inconsistently across sessions and split statements that belong together.
