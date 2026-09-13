---
id: check-result-limit
namespace: retrieval
kind: rule
modality: must
status: active
provenance:
    type: dialogue
created_at: 2026-09-13T21:24:38.711633656Z
relationships:
    - to: principles/agent-native
      type: refines
      note: a cap is what keeps check inside an agent's context budget
---

check caps its results, defaulting to ten. Without a cap an ordinary English draft sentence OR-matched 84% of a 200-statement corpus, roughly 45KB of JSON, which defeats the context economy check exists to provide.
