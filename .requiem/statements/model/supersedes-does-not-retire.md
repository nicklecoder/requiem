---
id: supersedes-does-not-retire
namespace: model
kind: rule
modality: must_not
status: active
provenance:
    type: dialogue
created_at: 2026-09-16T06:21:21.583640723Z
relationships:
    - to: principles/no-silent-success
      type: refines
---

Recording a supersedes relationship must not change the target's status. Requiem stores what it is told and does not infer a consequence from a fact, and the split state is legitimate on its own: A can supersede B while B stays in force through a migration window, which an automatic retirement would make impossible to express. But the silence was the defect — a statement something else explicitly supersedes stays active, so Status.Searchable() keeps it in check and audit results as a current decision, and the omission was caught twice in one session only because list --unreferenced happened to surface it. link therefore warns on stderr that the target is still active and names the update --status superseded that retires it.
