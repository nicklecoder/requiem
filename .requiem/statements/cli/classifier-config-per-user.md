---
id: classifier-config-per-user
namespace: cli
kind: rule
modality: must
status: active
provenance:
    type: dialogue
created_at: 2026-09-28T05:09:22.801300326Z
relationships:
    - to: cli/machine-config
      type: refines
      note: adds a section the machine layer supplies whole
    - to: retrieval/classifier-model-not-chosen
      type: depends_on
---

The classifier section of config (kind, endpoint, model, api_key_env, timeouts) is read from the machine config and the project's gitignored config.local.yaml, never from the committed config.yaml. Why: which classifier to run is each user's choice, not the team's: nothing stored in the corpus depends on it, since scores are only cached in the disposable index keyed by model, so the reason the embedding model is committed (every vector in a corpus must come from one model) does not apply; and the checkpoints differ in licence terms (retrieval/classifier-model-not-chosen), so a committed choice would push one person's model and its terms onto every clone.
