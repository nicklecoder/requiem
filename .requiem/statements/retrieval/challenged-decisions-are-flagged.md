---
id: challenged-decisions-are-flagged
namespace: retrieval
kind: rule
modality: should
status: active
provenance:
    type: dialogue
created_at: 2026-09-15T23:45:43.546526015Z
relationships:
    - to: principles/no-silent-success
      type: refines
      note: A corpus makes a contested decision read as settled unless something says otherwise.
    - to: principles/retrieval-not-judge
      type: refines
      note: It surfaces that a proposal points here and stops; whether the challenge is right stays with the reader.
---

A check result marks a statement as challenged when a proposed statement conflicts with it or would supersede it. A corpus of settled decisions makes existing decisions easy to honour, which is the point and also the risk: in the field an agent planning against this corpus treated the current identity key as settled and never asked whether changing it was the real fix, where an earlier plan written without the corpus had named exactly that as the root cause. Only proposals count, because a conflict recorded between two active statements is a judgment already made and not yet resolved, while a proposal pointing at a statement is someone arguing that this specific decision is wrong.
