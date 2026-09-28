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
    - to: principles/models-are-optional
      type: depends_on
    - to: retrieval/classifier-endpoint-kinds
      type: depends_on
    - to: model/add-checks-before-writing
      type: depends_on
      note: the write-time check keeps add's promise not to wait on the network
---

When a classifier is configured, requiem scores each active statement for open wording: the probability that the body leaves part of its decision undecided (an nli classifier weighs the body against 'Part of this decision is still undecided.'; a chat classifier is asked). list --open-wording ranks active statements by that score, and the calling agent clears each one with dismiss --open-wording, which records the body hash it judged under .requiem/open-wording/, so a cleared statement returns only when its body changes. add and update warn, never refuse, when an active body scores above classifier.open_wording_threshold. The threshold defaults to 0.9 for an nli classifier and is off for a chat classifier unless set, because a chat model's probabilities are not calibrated: gemma4 scored a plainly settled body 0.88. The write-time check allows the classifier 3 seconds and is skipped silently when it is slower or unreachable, so a write never waits on it (model/add-checks-before-writing). Why: a body that leaves its decision open hides a question from list --status proposed (model/open-questions-are-proposals). A phrase list flagged all 20 lookalikes in 482 real bodies ('open formats', 'cannot yet resolve') and missed question-form bodies, while a 435M three-way NLI model ranked all 5 known open bodies in the top 9 and scored the whole corpus in 4.2 s. The list lives under list rather than audit because audit needs an embedder and this needs only a classifier.
