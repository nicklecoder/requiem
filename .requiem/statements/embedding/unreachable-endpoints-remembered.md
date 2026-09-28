---
id: unreachable-endpoints-remembered
namespace: embedding
kind: rule
modality: should
status: active
provenance:
    type: dialogue
created_at: 2026-09-28T16:59:38.889482919Z
relationships:
    - to: embedding/fail-fast-unreachable
      type: depends_on
      via: link
    - to: principles/models-are-optional
      type: refines
      via: link
---

An endpoint that fails to connect is remembered as down in the project's index for 5 minutes. Commands that can do their job without it (check --semantic, audit's ordering, the open-wording check on add and update) skip it at once while it is marked down, saying so in one line on stderr, and try it again once the 5 minutes pass. Commands whose whole job is the model (reindex --embed, list --open-wording, init) always try it, so the user asking for the model is never refused on a stale memory. Why: failing fast still costs 2 seconds per call, paid again by every command while a laptop is off the LAN; remembering the outage makes the degraded commands instant after the first try, and the index is a disposable cache, so the memory costs nothing to lose.
