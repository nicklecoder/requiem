---
id: related-projects-stay-separate
namespace: retrieval
kind: design
modality: must
status: active
provenance:
    type: dialogue
created_at: 2026-10-05T00:42:56.762105374Z
relationships:
    - to: embedding/vectors-per-model
      type: depends_on
      note: why each project is searched with its own model and never fused
      via: batch
    - to: retrieval/related-projects-opt-in
      type: refines
      via: batch
---

A related project is searched as itself: requiem opens it at its own root, with its own index, its own config and therefore its own embedding model and endpoint, exactly as running the command inside that project would. Nothing from it enters this project's index, vectors, audit queue or graph, and its results are never fused into one ranking with this project's: check prints this project's candidates first, then each related project's as a separate run, each ranked and limited on its own and each candidate marked with the project it came from. Why: the projects are not one corpus. They can embed with different models, and cosine across two models means nothing (embedding/vectors-per-model); a fused ranking would imply the two corpora are commensurable and let a larger neighbour crowd this project's own decisions out of the limit. Refreshing the related project's own gitignored index, as any run there would, is the one effect on it.
