---
id: lexical-stemming
namespace: retrieval
kind: rule
modality: must
status: active
provenance:
    type: dialogue
created_at: 2026-09-24T21:00:22.850463847Z
relationships:
    - to: principles/no-silent-success
      type: refines
      note: A miss that depends on word form reads as 'nothing here conflicts'.
    - to: retrieval/df-filter
      type: depends_on
      note: The vocab tables hold stems, so the filter has to look up stems or it stops dropping anything inflected.
---

Both full-text tables index with the Porter tokenizer, so cancel, cancellation and cancellable are one term, and the document-frequency filter looks query words up by the stem SQLite itself produces, through a temp table using the same tokenizer string. Without stemming, whether check found a conflict depended on the exact word form: in a field report the draft 'investor may cancel before closing' never reached the rule it contradicted, which said cancellable and cancellation; rewording the draft to 'cancellation' put it at rank 7. The semantic path missed the same rule, so fusion had nothing to promote and check reported nothing to conflict with. The stemmer runs inside SQLite rather than as a Go port because a port drifting from fts5_porter.c by one rule would silently disable the filter for the words it disagreed on. This differs from the rejected tokenizer stoplist: the index is a disposable cache that reindex rebuilds, and Porter is a fixed algorithm rather than a list a project would want to revise.
