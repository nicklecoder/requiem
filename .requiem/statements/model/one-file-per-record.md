---
id: one-file-per-record
namespace: model
kind: rule
modality: must
status: active
provenance:
    type: dialogue
created_at: 2026-09-15T23:35:46.08472749Z
relationships:
    - to: principles/files-canonical
      type: refines
      note: Files are canonical, so the unit of a file is the unit a command can address.
---

Every record requiem writes lives in its own file: a statement at <id>.md, a rejection at <id>.rejected.md. A shared per-namespace file made an individual rejection unaddressable — discard resolved only <id>.md and so could never remove one, mv never rewrote its see_instead, and update had no way to correct one entry — and it made the namespace a merge-conflict hotspot when several agents record decisions at once. Legacy per-namespace _rejected.md files are still read, so a corpus written by an older requiem keeps working and keeps being searchable.
