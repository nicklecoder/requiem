---
id: marker-tolerance
namespace: traceability
kind: rule
modality: must
status: active
provenance:
    type: dialogue
created_at: 2026-09-14T04:14:53.382024613Z
relationships:
    - to: traceability/agent-writes-labels
      type: depends_on
      note: hand-written markers are the direct consequence of requiem not writing them
    - to: traceability/label-typos-uncaught
      type: refines
      note: 'narrows what counts as a typo: format variation no longer is one, only a wrong id'
---

The label scanner must accept the spacing and capitalisation a hand-written marker actually arrives with: any blanks around the colon, and any case. Since requiem writes no labels itself, every marker in a project comes from a hand, and a marker that fails to match produces no error anyone sees — the label is simply never found and the statement reads as unimplemented. Strictness here buys nothing and costs silence. mv's rewriter uses the same matcher, so every form the scanner finds is a form a rename can move.
