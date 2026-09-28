---
id: audit-orders-by-pair-classifier
namespace: retrieval
kind: rule
modality: must
status: active
provenance:
    type: dialogue
created_at: 2026-09-27T16:47:18.349741335Z
relationships:
    - to: retrieval/decision-model-pair-scoring
      type: supersedes
      note: 'answers the question: accepted as ordering evidence only'
    - to: principles/retrieval-not-judge
      type: depends_on
      note: the score orders; the agent's verdict decides
    - to: retrieval/audit-pairs-share-identifiers
      type: refines
      note: changes the order of the candidates audit already builds, not which pairs it builds
    - to: retrieval/nli-classifier-endpoint
      type: depends_on
    - to: principles/models-are-optional
      type: depends_on
---

When a pair classifier is configured, audit orders its queue by it. Each statement of a candidate pair is given to the model as premise with the other as hypothesis, and the larger of the two contradiction probabilities ranks the pair. The score orders the queue and does nothing else: every pair stays in the queue until the calling agent records a verdict, and no threshold hides or dismisses one. Without a classifier, audit is unchanged, and check does not use it. Why: embedding cosine cannot tell a conflict from a compatible pair among the pairs audit surfaces (AUC 0.51 with mxbai-embed-large, 0.76 with qwen3-embedding-8b on sbs), while a 435M three-way NLI model reached 0.93, put 78% of conflicts in the top tenth of the queue and scored 1,336 pairs in 27 s. Its probabilities moved with setup, so they may order a list but never gate one (principles/retrieval-not-judge); re-ranking check candidates did not beat a good embedder. The measurements are recorded on retrieval/decision-model-pair-scoring.
