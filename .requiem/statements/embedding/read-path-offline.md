---
id: read-path-offline
namespace: embedding
kind: rule
modality: must
status: active
provenance:
    type: dialogue
created_at: 2026-09-13T21:24:38.748985289Z
relationships:
    - to: principles/agent-native
      type: refines
      note: the most-used commands stay fast
---

The read path stays offline by default. reindex --embed is separate from a plain reindex, and check --semantic is opt-in, because those are the commands run most often and a network call on every one of them would be felt.
