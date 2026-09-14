---
id: principles-covered-transitively
namespace: traceability
kind: design
modality: may
status: active
provenance:
    type: dialogue
created_at: 2026-09-13T23:01:01.113457589Z
relationships:
    - to: traceability/code-labels
      type: depends_on
      note: only matters once labelling is in use
---

A principle is implemented through the rules that refine it, not at any single code site, so --unreferenced would report every principle forever. A statement therefore counts as covered when the active statements refining it are labelled, making the list mean 'genuinely unimplemented' rather than 'has no direct label'. Transitive coverage is an inference and can be wrong — refiners may implement only part of what they refine — so it is reported as covered_via naming the refiners responsible, and --direct suppresses it to recover the raw answer. Partially labelled refiners do not cover.
