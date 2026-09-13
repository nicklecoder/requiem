---
id: score-normalization
rejected_at: 2026-09-13T21:25:03.900028425Z
see_instead: retrieval/rrf-fusion
---

Min-max rescale bm25 and cosine into a common range before merging. Rejected: a single outlier compresses everything else into a narrow band — with one strong lexical hit the rest collapse to a tie — and the right blend shifts per query. RRF sidesteps both by fusing on position.
<!-- requiem:entry -->
---
id: static-stoplist
rejected_at: 2026-09-13T21:25:03.903447039Z
see_instead: retrieval/df-filter
---

Drop a fixed list of common English words at query-build time. Rejected: simple and language-specific, but blind to domain stopwords — 'token' saturating an auth-heavy corpus is the case that actually hurts, and no English list catches it.
<!-- requiem:entry -->
---
id: tokenizer-stoplist
rejected_at: 2026-09-13T21:25:03.906688145Z
see_instead: retrieval/df-filter
---

Configure the FTS5 tokenizer so stopwords never enter the index. Rejected: smallest index and fastest queries, but it changes the index format and bakes the list in at write time, so changing your mind means rebuilding.
