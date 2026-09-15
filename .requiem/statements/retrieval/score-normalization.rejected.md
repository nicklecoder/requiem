---
id: score-normalization
rejected_at: 2026-09-13T21:25:03.900028425Z
see_instead: retrieval/rrf-fusion
---

Min-max rescale bm25 and cosine into a common range before merging. Rejected: a single outlier compresses everything else into a narrow band — with one strong lexical hit the rest collapse to a tie — and the right blend shifts per query. RRF sidesteps both by fusing on position.
