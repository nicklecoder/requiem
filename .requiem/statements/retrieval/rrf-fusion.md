---
id: rrf-fusion
namespace: retrieval
kind: rule
modality: must
status: active
provenance:
    type: dialogue
created_at: 2026-09-13T21:24:38.715099081Z
relationships:
    - to: retrieval/rank-direction
      type: depends_on
      note: fusion emits the score negated to preserve the documented direction
---

Lexical and semantic results are fused by Reciprocal Rank Fusion on rank position, discarding raw scores. bm25 and cosine are not comparable and the mismatch is not a constant bias: FTS5 clamps a common term's IDF to 1e-6, so a common-term lexical hit sorts below every semantic hit while a rare-term hit sorts above. The direction flips per query term.
