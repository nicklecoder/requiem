---
id: see-instead-is-checked
namespace: model
kind: rule
modality: must
status: active
provenance:
    type: dialogue
created_at: 2026-09-15T23:35:46.088991082Z
relationships:
    - to: principles/no-silent-success
      type: refines
---

A rejection see_instead is validated when written, rewritten when the statement it names moves, and reported by audit when it names nothing. That pointer is the half of a rejection which answers what was done instead, so one resolving to nothing empties the record of its value. It used to break in silence: mv rewrote statement relationships only, reindex exited 0, audit said nothing, and get reported rejected_alternatives as null with no indication why.
