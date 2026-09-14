---
id: relationship-unique-per-pair
namespace: model
kind: rule
modality: must
status: active
provenance:
    type: dialogue
created_at: 2026-09-14T13:22:27.905924828Z
relationships:
    - to: model/validate-write-tolerate-read
      type: depends_on
      note: collapsing duplicates on read instead of failing the file is that asymmetry applied to relationships
---

A statement carries at most one relationship per target and type. Linking a pair again with the same type updates that entry's note in place, and an omitted note keeps the recorded one, rather than appending a second entry: the index keys relationships by (from, to, type), so a duplicate made every reindex fail, and re-recording an audit verdict is exactly how one gets written. A file that already holds duplicates — hand-edited, or appended by an older binary — reads with them collapsed, the later note winning, so the next write saves it clean; the write path rejects a duplicate. The same pair under two different types stays two relationships.
