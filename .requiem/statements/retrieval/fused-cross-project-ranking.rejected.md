---
id: fused-cross-project-ranking
rejected_at: 2026-10-05T00:42:56.852131054Z
see_instead: retrieval/related-projects-stay-separate
---

Merge a related project's candidates into one ranking with this project's, ordered by verdict or fused rank, so check returns a single top-ten across both. Rejected: it treats two independent corpora as one, the ranks and cosines are not comparable across projects that may embed with different models, and a larger neighbour (versine-ce has 164 statements against hosted versine's 28) would crowd this project's own decisions out of the limit.
