---
id: marker-in-fixtures
namespace: traceability
kind: design
modality: should
status: proposed
provenance:
    type: dialogue
created_at: 2026-09-14T00:29:42.372871439Z
relationships:
    - to: traceability/label-false-positives
      type: refines
      note: the same ambiguity, now inside test code
---

A literal marker in a test fixture or code sample is indistinguishable from a real label, so the scanner finds it. Third instance of the same class after README/SPEC prose and requiem's own generated docs. Handled so far by discipline (build markers at runtime, exclude generated files), but nothing enforces that and a future fixture will reintroduce it. A suppression convention in the linter tradition (nolint, noqa) is the standard answer; undecided whether it is worth the surface.
