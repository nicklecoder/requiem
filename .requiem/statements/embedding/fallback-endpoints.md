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
    - to: cli/classifier-config-per-user
      type: depends_on
      via: link
    - to: embedding/vectors-per-model
      type: depends_on
      via: link
---

An embedder may list fallback embedders, each an endpoint and a model, tried in order when the configured endpoint does not answer. A fallback serving the committed model uses the project's vector set; one serving a different model uses and fills its own (embedding/vectors-per-model), since vectors from two models are never compared. A classifier may list fallback classifiers, each with its own kind, endpoint and model, which can differ from the first, since scores are cached per classifier and never compared across them. Why: away from the LAN, a laptop running the same embedding model locally keeps semantic search working at lower speed, because the corpus's vectors are already stored and only one query needs embedding per check; a smaller local classifier keeps audit's ordering, less well.
