---
id: open-questions-are-proposals
namespace: model
kind: rule
modality: must
abstract: true
status: active
provenance:
    type: dialogue
created_at: 2026-09-14T04:38:12.565498872Z
relationships:
    - to: principles/no-silent-success
      type: refines
      note: an empty proposals queue read as 'no open decisions' when four were filed as active
---

An open question is recorded as a statement with status proposed, never as an active statement whose body hedges. A statement is active when it states what is true now; when the load-bearing content is a question, the status is proposed. list --status proposed is the only queue of open decisions requiem has, so a question buried in an active body is invisible to it and goes stale silently once the question is answered elsewhere. Measured in requiem's own corpus: four active statements carried 'Undecided' or 'open whether' in their bodies, every one had long since shipped, nothing detected it, and list --status proposed returned zero the whole time.
