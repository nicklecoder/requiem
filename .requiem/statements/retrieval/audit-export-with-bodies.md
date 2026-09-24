---
id: audit-export-with-bodies
namespace: retrieval
kind: question
status: proposed
provenance:
    type: dialogue
created_at: 2026-09-24T21:08:46.466172711Z
relationships:
    - to: cli/batch-dismiss
      type: depends_on
      note: The other half of the same workflow.
---

Open question: should audit be able to emit its pairs as JSON Lines with both full bodies, ready to split across reviewers and feed back through batch? A field agent built that export by hand for a 387-pair first audit. What hangs on it: batch now takes dismiss records, which covers the apply half; whether the export half is needed often enough to be a flag is unknown from one report.
