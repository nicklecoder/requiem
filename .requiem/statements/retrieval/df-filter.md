---
id: df-filter
namespace: retrieval
kind: rule
modality: should
status: active
provenance:
    type: dialogue
created_at: 2026-09-13T21:24:38.721955785Z
relationships:
    - to: retrieval/check-result-limit
      type: depends_on
      note: the filter improves ranking; the limit is what actually bounds output
---

Query terms occurring in more than half the rows are dropped, which is exactly where FTS5 clamps a term's IDF to zero. Frequency-driven rather than a fixed stoplist, so it catches domain stopwords like 'token' in an auth-heavy namespace. No filtering below twenty rows, because dropping a term removes its documents from the result set entirely, which is destructive on a small corpus.
