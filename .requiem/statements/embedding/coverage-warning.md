---
id: coverage-warning
namespace: embedding
kind: rule
modality: must
status: active
provenance:
    type: dialogue
created_at: 2026-09-13T21:24:38.745533663Z
relationships:
    - to: principles/no-silent-success
      type: refines
      note: partial coverage must not read as a thorough sweep
    - to: cli/diagnostics-stderr
      type: refines
      note: 'One instance of the convention: the shortfall is a diagnostic, not payload.'
---

audit and semantic check warn on stderr when part of the in-scope corpus lacks a fresh vector. A short result list would otherwise be indistinguishable from a thorough sweep that found little.
