---
id: verdict-band-percentile
namespace: retrieval
kind: question
status: proposed
provenance:
    type: dialogue
created_at: 2026-09-24T21:08:46.490635846Z
relationships:
    - to: retrieval/calibrated-verdict
      type: depends_on
      note: Questions the thresholds calibrated-verdict set.
---

Open question: should the verdict's similarity bands be set by percentile within the corpus rather than by raw cosine? In a single-domain corpus cosines crowd into a narrow band: a field report saw 0.63 for a conflicting rule and 0.79 for an agreeing one, around a related threshold of 0.72, so a fixed threshold separates little. What hangs on it: retrieval/score-normalization rejected rescaling scores to merge lists, which is a different use, but percentiles move as the corpus grows, so a verdict would change without either record changing.
