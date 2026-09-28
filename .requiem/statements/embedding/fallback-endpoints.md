---
id: fallback-endpoints
namespace: embedding
kind: rule
modality: may
status: active
provenance:
    type: dialogue
created_at: 2026-09-28T16:59:39.780454899Z
relationships:
    - to: embedding/model-pinning
      type: depends_on
      via: link
    - to: cli/classifier-config-per-user
      type: depends_on
      via: link
---

An embedder may list fallback endpoints, tried in order when the configured endpoint does not answer; every one must serve the configured model, since vectors from two models cannot be compared (embedding/model-pinning). A classifier may list fallback classifiers, each with its own kind, endpoint and model, which can differ from the first, since scores are cached per classifier and never compared across them. Why: away from the LAN, a laptop running the same embedding model locally keeps semantic search working at lower speed, because the corpus's vectors are already stored and only one query needs embedding per check; a smaller local classifier keeps audit's ordering, less well.
