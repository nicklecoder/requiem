---
id: code-labels
namespace: traceability
kind: design
modality: should
status: active
provenance:
    type: dialogue
created_at: 2026-09-13T22:28:53.711038145Z
---

Source comments carry a statement id so a changed decision can report the code it affects. Labels travel with code through refactors, which a line-range hash cannot. Commit trailers are deferred: mv can rewrite a comment but never a published trailer.
