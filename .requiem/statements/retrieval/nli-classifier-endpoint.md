---
id: nli-classifier-endpoint
namespace: retrieval
kind: rule
modality: must
status: superseded
provenance:
    type: dialogue
created_at: 2026-09-28T00:39:46.464056378Z
relationships:
    - to: retrieval/pair-classifier-endpoint
      type: supersedes
      note: answers the question
    - to: principles/models-are-optional
      type: depends_on
---

An optional classifier is reached by POST to a configured URL with {"pairs": [{"premise": ..., "hypothesis": ...}]}, and answers with one {"entailment", "neutral", "contradiction"} probability set per pair, in request order. requiem reads the labels by name and ignores any other field. Pair scoring sends each statement as premise with the other as hypothesis; the open-wording check sends a body as premise with a fixed hypothesis. Why: this is a three-way NLI model's own question, so one request shape serves both features, and any server implementing it works. The alternatives measured worse or served nothing: framed as a yes/no question in TypeSafe's /v1/systemone format, two-way zero-shot models scored AUC 0.58-0.67 on conflicts against 0.93 for the three-way model asked this way, and Ollaya cannot import a three-way checkpoint. On an M4 Mac Mini a small server running MoritzLaurer/DeBERTa-v3-large-mnli-fever-anli-ling-wanli on the Apple GPU reproduced the RTX 4070's scores within 0.005, at about 9 items a second beside a loaded embedder.
