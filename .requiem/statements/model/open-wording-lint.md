---
id: open-wording-lint
namespace: model
kind: question
status: proposed
provenance:
    type: dialogue
created_at: 2026-09-24T21:08:46.400335542Z
relationships:
    - to: model/open-questions-are-proposals
      type: depends_on
      note: The rule the lint would enforce.
---

Open question: should add and update warn when an active body contains wording that marks it open, such as for now, undecided, open whether, or until counsel answers? A field corpus carried an active body saying 'Until counsel answers', which the working process forbids and nothing caught. What hangs on it: plain until is legitimate in a rule ('irrevocable until close'), so only narrow phrases could be matched, and it would have to warn rather than refuse.
