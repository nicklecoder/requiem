---
id: index-diffs-name-ids
namespace: cli
kind: rule
modality: should
status: active
provenance:
    type: dialogue
created_at: 2026-09-15T23:35:46.10017844Z
relationships:
    - to: principles/no-silent-success
      type: refines
---

reindex reports the ids it added, updated and removed, not only the counts. A bare count cannot be acted on: only requiem knows which file path held which record, so removed 1 leaves the reader unable to tell what left the index. Unchanged ids are deliberately omitted, since on a normal run that list is the whole corpus and would bury the few records that actually moved.
