---
id: open-wording-review
namespace: model
kind: rule
modality: should
status: active
provenance:
    type: dialogue
created_at: 2026-09-27T17:00:08.926010931Z
relationships:
    - to: model/open-wording-lint
      type: supersedes
      note: answers the question with a model score, not a phrase list
    - to: model/open-questions-are-proposals
      type: refines
    - to: retrieval/pair-classifier-endpoint
      type: depends_on
      note: same classifier endpoint
    - to: model/verdicts-are-not-edges
      type: depends_on
      note: a cleared statement is a verdict, stored like audit dismissals
    - to: retrieval/nli-classifier-endpoint
      type: depends_on
    - to: principles/models-are-optional
      type: depends_on
---

When a classifier is configured, requiem scores each active statement for open wording: the model reads the body as premise and gives the probability that it entails 'Part of this decision is still undecided.' audit lists active statements ranked by that score, and the calling agent clears each one with a verdict that records the body hash it judged, so a cleared statement returns only when its body changes. add and update warn, never refuse, when an active body scores above a configurable threshold. Why: a body that leaves its decision open hides a question from list --status proposed (model/open-questions-are-proposals). A phrase list flagged all 20 lookalikes in 482 real bodies ('open formats', 'cannot yet resolve') and missed question-form bodies, while a 435M three-way NLI model ranked all 5 known open bodies in the top 9 and scored the whole corpus in 4.2 s. The ranked queue needs no threshold; the write-time warning does, which is why it only warns.
