---
id: rejections-embedded
namespace: retrieval
kind: rule
modality: must
status: active
provenance:
    type: dialogue
created_at: 2026-09-15T23:35:46.092859807Z
relationships:
    - to: principles/no-silent-success
      type: refines
    - to: retrieval/rrf-fusion
      type: depends_on
      note: Fusing on rank position is what made a missing vector cost a rejection every slot.
---

Rejections carry embedding vectors exactly as statements do. The embeddings table is keyed by (source_kind, full_id) rather than by full_id alone, since a statement and a rejection may share an id. Without a vector a rejection could only score the lexical half of the rank fusion, so every statement outranked every rejection under semantic search: measured on a 255-statement corpus, a query describing an already-rejected idea returned no rejections at all in the top ten and left the exact match at position 37 of 55, while the documented workflow tells the agent to read the rejections first.
