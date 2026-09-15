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
---

The index records a derivation_version, and changing it makes the next reindex reparse every statement file. Reindex is incremental, so anything derived from a body — the facet index, and data of the same character added later — is invisible on an existing index without a forced reparse: every unmodified file reports unchanged, the code that would populate the new table never runs, and the feature does nothing until someone edits each file by hand. The facet index shipped exactly that way and matched nothing at all on an already-indexed corpus. Embeddings and code reference counts are exempt from the rebuild, because a vector cannot be recovered by reparsing a file.
