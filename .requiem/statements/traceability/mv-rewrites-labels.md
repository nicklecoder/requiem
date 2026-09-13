---
id: mv-rewrites-labels
namespace: traceability
kind: design
modality: may
status: proposed
provenance:
    type: dialogue
created_at: 2026-09-13T22:40:10.194740468Z
---

mv could rewrite labels in source files rather than only reporting them. Undecided: rewriting means requiem editing files outside .requiem, which no other command does, and a text-driven edit can hit a string literal or a doc example. Report-only for now, because the reversible choice stays available.
