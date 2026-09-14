---
id: search-fallback
namespace: traceability
kind: design
modality: should
status: active
provenance:
    type: dialogue
created_at: 2026-09-14T02:26:04.013916047Z
relationships:
    - to: traceability/code-labels
      type: refines
      note: makes labels optional rather than optional-but-regretted
---

When no label exists, trace --search finds code by the statement's own distinctive vocabulary. The tool must be useful at zero label coverage and merely sharper as coverage rises, so labels stay a shortcut rather than a precondition.
