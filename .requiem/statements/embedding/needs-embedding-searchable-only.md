---
id: needs-embedding-searchable-only
namespace: embedding
kind: rule
modality: must_not
status: active
provenance:
    type: dialogue
created_at: 2026-09-27T04:49:06.407021346Z
relationships:
    - to: model/retired-is-not-unimplemented
      type: depends_on
      note: 'the same reasoning applied to embeddings: a retired statement''s absence from a work queue is correct, not a gap'
    - to: embedding/coverage-warning
      type: refines
      note: coverage already counts only searchable records; the list now agrees with it
---

list --needs-embedding must not report a superseded or deprecated statement. reindex --embed embeds only what the semantic paths search, active and proposed statements, so a retired one is never embedded and listing it reports a gap no command can close: an agent told to run reindex until the list is empty never gets there. A forced re-embed under a new model makes this common, since it discards the vectors retired statements kept from when they were live. The same reasoning keeps a retired statement out of list --unreferenced.
