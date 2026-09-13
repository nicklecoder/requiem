---
id: approval-by-accident
namespace: cli
kind: design
modality: should
status: proposed
provenance:
    type: dialogue
created_at: 2026-09-13T22:40:10.199051079Z
relationships:
    - to: principles/no-silent-success
      type: refines
      note: approval without review is a silent success
---

A plain git commit sweeps in whatever requiem has auto-staged, so approval can happen without review — the inverse of the path-scoping that stops requiem commit sweeping in code. A pre-commit hook warning about pending statements would keep auto-staging's convenience while making accidental approval loud.
