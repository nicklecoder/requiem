---
id: mv-rewrites-labels
namespace: traceability
kind: design
modality: may
status: active
provenance:
    type: dialogue
created_at: 2026-09-13T22:40:10.194740468Z
relationships:
    - to: traceability/code-labels
      type: depends_on
      note: only matters once labels are in use
---

mv rewrites labels in source files rather than only reporting them. Rewriting means requiem editing files outside .requiem, which no other command does, so the edits are left unstaged and appear in git diff before anything can be committed — that reversibility is what made this safe to decide. The risk of hitting a string literal or doc example is bounded by matching only the marker and its exact id, and the rewrite goes through the same matcher the scanner uses, so every form found is a form that can be moved. --no-rewrite-refs keeps the report-only behaviour available.
