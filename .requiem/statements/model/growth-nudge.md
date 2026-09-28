---
id: growth-nudge
namespace: model
kind: rule
modality: should
status: active
provenance:
    type: dialogue
created_at: 2026-09-28T17:46:38.629975082Z
relationships:
    - to: model/one-decision-per-statement
      type: refines
      via: link
    - to: principles/retrieval-not-judge
      type: depends_on
      note: asks the author; judges nothing
      via: link
---

update prints a note on stderr when it grows a body by at least half its previous length and by at least 300 characters, asking whether what was added is a decision of its own that belongs in a separate statement. It never refuses, and batch updates carry the note as their warning. Why: a statement can also become several decisions by accretion, each update appending a little more; in sbs one statement nearly doubled over five revisions, and in requiem an open question more than doubled by collecting measurements. The moment of growth is when the author is already reasoning about that statement, so the question costs a glance there, where no cheap signal could find compound statements afterwards (model/one-decision-per-statement).
