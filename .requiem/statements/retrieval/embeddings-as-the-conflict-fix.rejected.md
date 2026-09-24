---
id: embeddings-as-the-conflict-fix
rejected_at: 2026-09-24T21:01:18.149414399Z
see_instead: retrieval/check-the-negation
---

Treat the conflict-versus-paraphrase miss as an embedding-quality problem, to be fixed by moving to a better embedding model. Rejected: an embedding measures topic and wording, and a conflicting rule is worded from the other side, so a model that is better at similarity is not better at opposition. The fixes that reach it are ones that do not depend on wording: stemming, identifiers, the graph, and a query written from the opposing side. Per-sentence vectors are a separate idea, aimed at bodies diluted by citations, and are not what this rejects.
