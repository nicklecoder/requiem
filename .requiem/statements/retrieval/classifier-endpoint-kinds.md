---
id: classifier-endpoint-kinds
namespace: retrieval
kind: rule
modality: must
status: active
provenance:
    type: dialogue
created_at: 2026-09-28T05:09:07.819772385Z
relationships:
    - to: retrieval/nli-classifier-endpoint
      type: supersedes
      note: widens the contract to chat classifiers after the real-use result
    - to: principles/models-are-optional
      type: depends_on
    - to: embedding/configured-endpoint
      type: depends_on
      note: a configured endpoint, no bundled model
---

An optional classifier is one of two kinds, named in its config. An nli classifier takes a POST of {"pairs": [{"premise": ..., "hypothesis": ...}]} and answers one {"entailment", "neutral", "contradiction"} probability set per pair, in request order; labels are read by name and other fields ignored. A chat classifier is any OpenAI-compatible /v1/chat/completions endpoint with logprobs: requiem asks one question with lettered options (A: no, B: yes), requests a single token with top_logprobs, and reads the probability of the answer letter. Conflict scoring gives an nli classifier each statement as premise with the other as hypothesis and keeps the larger contradiction probability; it asks a chat classifier whether the two statements contradict each other so that one design cannot follow both. The open-wording check gives an nli classifier the body as premise and 'Part of this decision is still undecided.' as hypothesis, and asks a chat classifier whether the statement leaves part of its decision open. Why: both kinds do the job and each has a strength. On sbs a 435M three-way NLI model put 78% of conflicts in the top tenth of the queue (AUC 0.93) at about 9 items a second on the M4 Mac Mini, but on the one real defect found it moved the pair from 308th only to 79th, with unrelated pairs above it at 1.00. gemma4 8B through Ollama's chat endpoint ranked conflicts alike (AUC 0.938) and put that defect first, needing no server beyond Ollama, at about 0.7 s a pair. Neither is recommended over the other (retrieval/classifier-model-not-chosen).
