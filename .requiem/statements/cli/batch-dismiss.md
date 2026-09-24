---
id: batch-dismiss
namespace: cli
kind: rule
modality: must
status: active
provenance:
    type: dialogue
created_at: 2026-09-24T21:03:29.092900936Z
relationships:
    - to: cli/batch-input
      type: refines
      note: Extends batch to the write an audit produces most.
    - to: model/verdicts-are-not-edges
      type: depends_on
      note: A batched dismissal is still a verdict, never an edge.
---

batch accepts dismiss records, {"op":"dismiss","from":a,"to":b,"note":...}, writing the same audit verdict the dismiss command does. A first audit is mostly dismissals: one field corpus surfaced 387 pairs, of which 2 were real findings and 236 were dismissed, and without a batch op the agent wrote its own export, split and apply scripts to record them. One process per verdict is the same ingestion problem batch was added to solve for statements.
