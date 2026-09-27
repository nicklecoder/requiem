---
id: verdict-bands-by-percentile
rejected_at: 2026-09-27T17:02:16.245122227Z
see_instead: retrieval/calibrated-verdict
---

Set the verdict's similarity bands by percentile within the corpus rather than by raw cosine, after a field report saw mxbai-embed-large crowd every cosine into 0.75-0.90 so the fixed related band of 0.72 separated little. Rejected: the crowding came from a weak embedder, not from fixed bands. On the same sbs pairs, the share of adjudicated compatible pairs above 0.72 fell from 42-54% under mxbai to 8-10% under qwen3-embedding-8b, while conflicts stayed at 40% and refines/depends_on pairs at 22%, so the fixed band separated better once the embedder improved. A percentile would also change a verdict as the corpus grew, without either record changing, which is the wrong property for an answer to 'is this already stated?'.
