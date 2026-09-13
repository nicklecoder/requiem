---
id: principles-covered-transitively
namespace: traceability
kind: design
modality: may
status: proposed
provenance:
    type: dialogue
created_at: 2026-09-13T23:01:01.113457589Z
relationships:
    - to: traceability/code-labels
      type: depends_on
      note: only matters once labelling is in use
---

A principle is implemented through the rules that refine it, not at any single code site, so --unreferenced reports every principle forever. Requiem could treat a statement as covered when the statements refining it are labelled, making the list mean 'genuinely unimplemented' rather than 'has no direct label'. Undecided: transitive coverage is a judgement, and inferring it risks reporting a principle as satisfied when only part of it is.
