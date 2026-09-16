---
id: relationships-are-removable
namespace: model
kind: rule
modality: must
status: active
provenance:
    type: dialogue
created_at: 2026-09-16T15:20:12.620732535Z
relationships:
    - to: model/relationship-unique-per-pair
      type: depends_on
      note: a pair may carry two types, which is why removal is explicit rather than a replacing link
    - to: principles/files-canonical
      type: refines
---

unlink <from> <to> removes a recorded relationship, narrowed to one type with --type. Without it there is no way to take an edge back except hand-editing the statement file, and that is where a resolved audit finding goes to rot: audit skips any pair that already carries a relationship, so a conflicts_with edge recorded during adjudication and then resolved keeps the pair out of the queue forever while the graph goes on asserting a conflict nobody believes. Every other write in requiem has a way back — discard unstages, dismiss --restore takes a verdict back, update revises a body — and the relationship graph was the one structure that only ever grew.

Retyping is unlink then link rather than a flag on link. A pair legitimately carries two relationships of different types (model/relationship-unique-per-pair), so a link that replaced whatever was there would have to guess which of them the caller meant to destroy, and would sometimes guess a depends_on edge that had nothing to do with the retype.
