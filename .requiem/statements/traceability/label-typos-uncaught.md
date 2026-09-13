---
id: label-typos-uncaught
namespace: traceability
kind: design
modality: should
status: proposed
provenance:
    type: dialogue
created_at: 2026-09-13T23:16:42.516598251Z
relationships:
    - to: traceability/code-labels
      type: depends_on
      note: a cost of the marker-in-comment approach
---

A label is a comment, so nothing catches a typo at the point of writing. A mistyped id scans as a dangling reference, which trace shows and audit deliberately ignores, so the mistake surfaces late or not at all. Inherent to comment-carried metadata; open whether requiem should offer a verification pass.
