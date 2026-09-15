---
id: identifier-facets
namespace: retrieval
kind: rule
modality: should
status: active
provenance:
    type: dialogue
created_at: 2026-09-15T23:45:43.539040954Z
relationships:
    - to: principles/agent-native
      type: refines
      note: An exact key costs an agent less to act on than a ranked guess.
---

Concrete identifiers named in a body — snake_case names, dotted paths, atoms, camelCase — are indexed as facets, and check --touches <identifier> retrieves the records naming one. Embeddings cannot pair two records that share an identifier and nothing else: measured on a real 255-statement corpus, nearest-neighbour search over 1,936 candidate pairs never surfaced three genuine conflicts, and every one of them shared an identifier while sharing almost no wording. An identifier is the one part of a decision that is unambiguous, so it is treated as an exact key rather than as another similarity score. Ordinary English words are deliberately not facets: session as a facet would match half an auth corpus and assert nothing.
