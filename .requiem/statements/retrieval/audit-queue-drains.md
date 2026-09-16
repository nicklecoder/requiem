---
id: audit-queue-drains
namespace: retrieval
kind: rule
modality: must
status: active
provenance:
    type: dialogue
created_at: 2026-09-16T05:17:52.316532968Z
relationships:
    - to: retrieval/audit-pairs-share-identifiers
      type: refines
      note: 'Same sweep: this fixes which pairs the window holds, not how they are ranked.'
    - to: principles/no-silent-success
      type: refines
      note: A count that never falls reports motion as progress.
---

Each statement window of candidate pairs is fixed before adjudication is considered, so a verdict removes a pair for good instead of promoting the next-nearest neighbour into the freed slot. The sweep used to rank only the unadjudicated partners and take the nearest k, which meant the outstanding count never fell: fifteen verdicts against this corpus moved it from 93 to 92, and the field report measured 415 to 425 after 97 verdicts. A queue that cannot be emptied gets abandoned, and the fix is ordering rather than filtering. The cost is deliberate: a pair at depth k plus one is no longer offered merely because a nearer pair was judged, so depth becomes a dial the reader turns — sweep at two, then at five — rather than a dribble the tool decides. Ordering is also the only model-independent lever here, since a similarity floor would have to name an absolute cosine and what counts as close is a property of the embedding model rather than of the corpus. Progress is reported as two numbers: pairs outstanding, and statements swept at the current depth, the second being the one that rises as work is done.
