---
id: staleness-travels-with-results
namespace: retrieval
kind: rule
modality: should
status: active
provenance:
    type: dialogue
created_at: 2026-09-15T23:35:46.096619053Z
relationships:
    - to: principles/no-silent-success
      type: refines
---

Every check result carries the stale flag for a code-derived statement, not only get. The provenance hash was already stored and already rehashed on the get path, so the anti-rot mechanism existed but reached only the reader who had already chosen a candidate: an agent scanning a ranked list could not see that the code a statement was derived from had changed since it was written.
