---
id: label-typos-uncaught
namespace: traceability
kind: design
modality: should
status: active
provenance:
    type: dialogue
created_at: 2026-09-13T23:16:42.516598251Z
relationships:
    - to: traceability/code-labels
      type: depends_on
      note: a cost of the marker-in-comment approach
---

A label is a comment, so nothing catches a typo at the point of writing. A mistyped id scans as a dangling reference, which trace shows and audit deliberately ignores. The verification pass is reindex: a dangling label within Levenshtein distance 2 of a real id is reported as a near miss and exits nonzero, so a typo fails CI rather than surfacing late. Distance 2 is cobra's own SuggestionsMinimumDistance, reused rather than invented. A dangling id far from anything real is left alone — it is more likely a statement not yet written than a misspelling.
