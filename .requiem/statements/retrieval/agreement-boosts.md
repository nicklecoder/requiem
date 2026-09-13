---
id: agreement-boosts
namespace: retrieval
kind: rule
modality: must
status: active
provenance:
    type: dialogue
created_at: 2026-09-13T21:24:38.725397247Z
relationships:
    - to: principles/retrieval-not-judge
      type: refines
      note: surface the stronger candidate, still let the agent decide
---

A candidate found by both the lexical and semantic paths ranks above one found by either alone, and is marked match_kind 'both'. Shared vocabulary and embedding proximity are independent signals; agreement between them is the most useful thing fusion can report.
