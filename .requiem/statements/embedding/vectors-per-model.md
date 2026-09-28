---
id: vectors-per-model
namespace: embedding
kind: rule
modality: must
status: active
provenance:
    type: dialogue
created_at: 2026-09-28T17:11:26.472074771Z
relationships:
    - to: embedding/model-pinning
      type: supersedes
      note: keeps never comparing across models; drops one model per index and the discard on switch
      via: link
    - to: embedding/committed-pipeline
      type: depends_on
      via: link
---

The index keeps a separate set of vectors for each embedding model, keyed by model, and every comparison uses one model's set: a query embedded by a model is scored only against that model's vectors, and audit pairs only vectors of one model. Switching the active model selects its set and embeds only what that set is missing; nothing is discarded. The committed model stays the project's primary (embedding/committed-pipeline), and a machine may add sets for fallback models it runs itself. Why: cosine between vectors from two models is a plausible-looking number that means nothing, which is the whole of what model pinning protected; storing one model per index went further and made every switch discard the corpus's vectors, so moving between a LAN server and a laptop model would re-embed everything each way. Measured on sbs (510 records), a full re-embed takes 18 s with qwen3-embedding-8b on an RTX 4070, 5 s with mxbai-embed-large, and about 6 minutes on the M4 Mac Mini over the LAN: affordable once per model, not on every switch.
