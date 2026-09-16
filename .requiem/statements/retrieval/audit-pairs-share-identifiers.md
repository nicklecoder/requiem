---
id: audit-pairs-share-identifiers
namespace: retrieval
kind: rule
modality: must
status: active
provenance:
    type: dialogue
created_at: 2026-09-16T00:03:07.050266374Z
relationships:
    - to: retrieval/identifier-facets
      type: depends_on
      note: Pairing on an identifier needs the facet index.
    - to: model/modality-is-not-conflict-detection
      type: depends_on
      note: The demotion follows from what modality can and cannot decide.
    - to: principles/no-silent-success
      type: refines
      note: An unsweepable corpus says so, and the backlog count keeps progress visible.
---

audit builds its candidate pairs from shared identifiers and shared source files first, then from each statement nearest neighbours by embedding, and treats opposed modality as a tiebreak only. Measured on a real 255-statement corpus: nearest-neighbour search over 1,936 candidate pairs missed three genuine conflicts, every one of which shared an identifier while sharing almost no prose, and ranking by modality opposition pushed 48 unrelated pairs into the top 50, because a must set against a must_not on different subjects is ordinary. An identifier is an exact key, so pairing on one asserts something about subject matter instead of guessing from wording, and it needs no vectors at all — which is why a corpus is refused as unsweepable only when it has neither vectors nor identifiers. audit also reports how many unadjudicated pairs remain, because a queue that appears to grow as it is worked gets abandoned: one corpus went from 415 to 425 outstanding pairs after 97 verdicts.
