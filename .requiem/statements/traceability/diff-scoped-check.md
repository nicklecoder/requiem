---
id: diff-scoped-check
namespace: traceability
kind: rule
modality: should
status: active
provenance:
    type: dialogue
created_at: 2026-09-16T00:11:46.708100201Z
relationships:
    - to: retrieval/identifier-facets
      type: depends_on
      note: Reaching an unlabelled decision from a patch needs the identifier index.
    - to: principles/agent-native
      type: refines
      note: The moment of use is the patch, not a remembered intention to ask.
---

check --diff takes a patch instead of a draft and reports the decisions bearing on it: statements whose labels sit inside an edited hunk, statements whose own code-derived source range the change touches, and records naming an identifier the patch adds — rejections among them, reported separately, since re-introducing a rejected idea is the most valuable thing to catch. A mature repository has no fresh intent document to check a change against; it has a diff, and that is the artifact the corpus has to be able to take. Retrieval that only answers when somebody thinks to ask is retrieval that gets skipped. Requiem own records are excluded from the scan, because a patch that records a decision would otherwise report itself.
