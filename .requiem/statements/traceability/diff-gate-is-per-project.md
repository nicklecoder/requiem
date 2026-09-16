---
id: diff-gate-is-per-project
namespace: traceability
kind: rule
modality: may
status: active
provenance:
    type: dialogue
created_at: 2026-09-16T00:11:46.732787796Z
relationships:
    - to: traceability/labels-not-enforced
      type: refines
      note: A gate must not become the enforcement this project refuses.
    - to: traceability/diff-scoped-check
      type: refines
      note: The gate is a property of the diff-scoped check, not a separate mechanism.
---

Whether check --diff fails a build is a per-project setting in config.yaml — off, warn or error — and off by default. The answer genuinely differs between projects: a team that has adopted labelling wants continuous integration to catch code contradicting a recorded decision, while a team mid-adoption would be blocked by noise it cannot act on yet, and requiem has no basis for choosing between them. Even at error the gate fails only on checkable facts: a labelled site pointing at a retired or rejected decision, and a covering statement whose source range has drifted. It never fails because a change touches decisions the author may not have read, which is a judgment about intent and the same line that keeps labels unenforced.
