---
id: derived-data-is-versioned
namespace: model
kind: rule
modality: must
status: active
provenance:
    type: dialogue
created_at: 2026-09-15T23:45:43.550020658Z
relationships:
    - to: principles/files-canonical
      type: refines
      note: Files are canonical only if the index actually rebuilds from them.
    - to: embedding/committed-pipeline
      type: depends_on
      note: Embeddings are exempt from the rebuild precisely because the pipeline that reproduces them is committed instead.
---

The index records a derivation_version, and changing it makes the next reindex reparse every statement file. Reindex is incremental, so anything derived from a body — the facet index, and data of the same character added later — is invisible on an existing index without a forced reparse: every unmodified file reports unchanged, the code that would populate the new table never runs, and the feature does nothing until someone edits each file by hand. Changing how derived data is extracted counts as much as adding a table: tightening the facet extractor left every previously-indexed body carrying its old facets, so the new filter appeared to do nothing at all until the version moved. The facet index shipped both ways before this was understood. Embeddings and code reference counts are exempt from the rebuild, because a vector cannot be recovered by reparsing a file.
