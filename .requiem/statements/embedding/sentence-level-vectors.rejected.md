---
id: sentence-level-vectors
rejected_at: 2026-09-27T17:09:21.693678649Z
---

Embed each body per sentence and score it by its best sentence, or embed a separate short claim line, instead of one vector per body, after a field report found citation-dense bodies diluted a single vector. Rejected: with a strong embedder the whole-body vector is not what limits retrieval. On sbs, querying with each rejected idea, qwen3-embedding-8b put the statement that replaced it first 75% of the time and in the top 3 98% of the time, against 66% and 92% for mxbai-embed-large, the weaker kind of model the report likely used. Several vectors per record multiply storage and endpoint calls, and a claim line adds a field every author must write, for a gain nothing measured shows.
