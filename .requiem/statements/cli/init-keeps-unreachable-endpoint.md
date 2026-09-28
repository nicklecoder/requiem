---
id: init-keeps-unreachable-endpoint
namespace: cli
kind: rule
modality: must_not
status: active
provenance:
    type: dialogue
created_at: 2026-09-28T16:59:39.358439893Z
relationships:
    - to: cli/init-sets-up-models
      type: refines
      via: link
    - to: embedding/fallback-endpoints
      type: depends_on
      via: link
---

init must not replace a configured endpoint that fails to connect. An unreachable endpoint may only be out of reach for now, as a LAN server is from a laptop away from home, so init keeps it, reports the outage, and when it finds another endpoint that answers with the same model it adds that one as a fallback instead. Why: run off the LAN, init replaced the machine config's LAN endpoint with a local Ollama serving the same model, so every project on the machine kept using the local server after returning to the LAN, and nothing said so.
