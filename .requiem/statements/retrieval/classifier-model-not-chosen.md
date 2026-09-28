---
id: classifier-model-not-chosen
namespace: retrieval
kind: rule
modality: must_not
abstract: true
status: active
provenance:
    type: dialogue
created_at: 2026-09-28T00:39:46.5032027Z
relationships:
    - to: embedding/configured-endpoint
      type: refines
---

requiem must not ship, download, or default to a classifier model, and its docs name no single one as the default. The documentation lists the measured candidates with their quality and licence notes, and the server the user runs decides. Why: the best-measured checkpoint (MoritzLaurer/DeBERTa-v3-large-mnli-fever-anli-ling-wanli: 78% of conflicts in the top tenth of the queue, lint AUC 0.995) is labelled MIT but trained partly on ANLI, published CC BY-NC 4.0, and whether that carries to the weights is unsettled; the clean alternative (cross-encoder/nli-deberta-v3-large, Apache-2.0, SNLI and MultiNLI) caught 57% and reached lint AUC 0.922. A user choosing between them should do so knowingly, and requiem, which bundles no model (embedding/configured-endpoint), keeps its own MIT licence clear of the question by never making the choice for them.
