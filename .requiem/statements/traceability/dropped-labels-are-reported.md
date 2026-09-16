---
id: dropped-labels-are-reported
namespace: traceability
kind: rule
modality: must
status: active
provenance:
    type: dialogue
created_at: 2026-09-16T15:01:29.208390321Z
relationships:
    - to: traceability/diff-scoped-check
      type: refines
    - to: traceability/diff-gate-is-per-project
      type: depends_on
      note: the gate mode decides whether a dropped label warns or fails
    - to: traceability/labels-not-enforced
      type: depends_on
      note: 'Stays inside the prohibition only because it never asks for a label that did not exist: it reports the patch deleting one. If that line moves, this rule goes with it.'
---

check --diff reports a decision whose last code label the patch removes, and the gate fails on it. The removed line is in the patch — a label deletion is an ordinary minus line — but the scan only ever read the post-image, so the one patch that makes a decision invisible was the one patch reporting no covering decisions at all: measured on a stripped label, covering came back empty while code_refs dropped to zero and nothing failed. A moved label needs no special case and gets none: the working tree is rescanned after the change, so a label that merely travelled between files is still found and nothing is reported. Four exclusions, each because the removal is correct rather than a loss — a label naming no statement, one naming a rejection, one on a retired decision, and one on an abstract statement, which audit asks you to delete anyway. Scoped to code files: a mention in a .md was never coverage, so losing it loses nothing.

This sits close to traceability/labels-not-enforced, and the line between them is the whole argument. Enforcing labels would mean judging that a change ought to have carried one, which is a judgment about intent. This judges nothing: it reports a fact the patch itself contains, that a claim someone wrote down was deleted here. Nothing is ever asked of a project that did not label in the first place — with no labels there is nothing to drop and the check never fires, so a repository with zero labels behaves exactly as it did before, which is the boundary's own closing promise.
