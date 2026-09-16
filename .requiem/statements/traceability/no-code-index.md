---
id: no-code-index
namespace: traceability
kind: rule
modality: must_not
status: active
provenance:
    type: dialogue
created_at: 2026-09-13T22:28:53.714195331Z
relationships:
    - to: principles/files-canonical
      type: refines
      note: One canonical store and one derived index; a second index over source would be a second manifest to keep true.
---

Requiem must not acquire a second index over the source tree. git grep answers the whole question in one pass, skips gitignored paths for free, and needs no manifest.
