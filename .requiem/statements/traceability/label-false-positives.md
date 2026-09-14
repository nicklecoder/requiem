---
id: label-false-positives
namespace: traceability
kind: design
modality: should
status: active
provenance:
    type: dialogue
created_at: 2026-09-13T22:51:24.917821639Z
relationships:
    - to: traceability/code-labels
      type: depends_on
      note: a cost of the marker-in-prose approach
---

Prose discussing the label format scans as a label, so documentation explaining traceability creates phantom references. Contained today because such ids classify as dangling and audit ignores those, but a project whose docs cite a real statement id would silently overcount it. Requiring a comment context needs language-aware parsing, and a markdown fence showing a comment would still match.
