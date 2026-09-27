---
id: decision-model-pair-scoring
namespace: retrieval
kind: question
status: proposed
provenance:
    type: dialogue
created_at: 2026-09-26T22:40:01.608038548Z
relationships:
    - to: principles/retrieval-not-judge
      type: depends_on
      note: a decision-model score is admissible only as ranking evidence
---

Open question: should audit take an optional score from a configured model that reads both statements of a candidate pair together and returns a probability that they contradict? A bi-encoder cosine judges each statement alone, so it misses contraries worded from the other side ('must be red' against 'must be blue'). Measured on the sbs corpus (2026-09-26, 91 rejected ideas scored against their see_instead statement, 945 adjudicated compatible pairs, 300 refines/depends_on pairs): mxbai cosine separated conflicts from compatible pairs at AUC 0.51, qwen3-embedding-8b at 0.76, and a pair-reading model at 0.94-0.96. A Jev-class model (decider-4b v2) and a general instruct model (gemma4 8B) scored alike, and gemma4 flagged 0 of 945 compatible pairs where decider flagged 3-6%, so nothing requires the Jev class: the readout is one question with lettered options, scored from first-token logprobs. The OpenAI-compatible chat endpoint with logprobs kept the ranking (AUC 0.938 against 0.941 through raw prompts) but not the probabilities (57% of conflicts above 0.5 through raw prompts, 16.5% through chat), so a score can order the audit queue and must never be read against a fixed threshold. It did not help check: re-ranking a draft's 20 nearest statements, qwen3 cosine alone found the replacing statement first 75% of the time, gemma4 fused with mxbai 80%. Latency per pair: about 0.72 s on the M4 Mac Mini, 0.05-0.1 s on an RTX 4070 SUPER. What hangs on it: principles/retrieval-not-judge admits the score only as evidence that orders the queue, never as a verdict that skips a pair; the positives are rejections, cleaner than a conflict that arises unnoticed; and on 382 unadjudicated sbs pairs gemma4 flagged 2, one a plausible tension that audit ranked 308th, which still needs the author's verdict. Deferred on 2026-09-27 until a real audit shows the model ranking a genuine conflict early: the first evidence is the author's verdict on that pair and on the 75 pairs qwen3-embedding surfaced in sbs after the switch. If it finds none, the answer is a rejection that carries these measurements; if it does, answer retrieval/audit-export-with-bodies alongside it, since scoring outside requiem is the lighter build.
