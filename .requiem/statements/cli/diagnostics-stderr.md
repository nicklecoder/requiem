---
id: diagnostics-stderr
namespace: cli
kind: rule
modality: must
status: active
provenance:
    type: dialogue
created_at: 2026-09-13T21:24:38.708098253Z
relationships:
    - to: cli/bare-stdout
      type: refines
      note: how diagnostics reach a reader without polluting the payload
---

Diagnostics that are not part of the payload — incomplete embedding coverage, partial-failure summaries — go to stderr. Agent harnesses surface combined output so the reader still sees them, while jq pipelines stay clean.
