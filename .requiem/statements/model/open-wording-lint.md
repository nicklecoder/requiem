---
id: open-wording-lint
namespace: model
kind: question
status: superseded
provenance:
    type: dialogue
created_at: 2026-09-24T21:08:46.400335542Z
relationships:
    - to: model/open-questions-are-proposals
      type: depends_on
      note: The rule the lint would enforce.
---

Open question: should add and update warn when an active body contains wording that marks it open, such as for now, undecided, open whether, or until counsel answers? A field corpus carried an active body saying 'Until counsel answers', which the working process forbids and nothing caught. What hangs on it: plain until is legitimate in a rule ('irrevocable until close'), so only narrow phrases could be matched, and it would have to warn rather than refuse. Measured 2026-09-27 on 482 historical active bodies from requiem and sbs, 5 hand-labelled as carrying an open part. The keyword pattern caught all 5 only because they were found with it, and it also flagged all 20 lookalikes ('open formats', 'cannot yet resolve') and missed open questions written as questions (AUC 0.52 on 14 proposed questions). A three-way NLI model (MoritzLaurer/DeBERTa-v3-large-mnli-fever-anli-ling-wanli, 435M), reading the entailment probability of 'Part of this decision is still undecided.' with the body as premise, ranked the 5 within the top 9 (AUC 0.995; 0.998 on question-form bodies) and scored every body in 4.2 s on an RTX 4070 SUPER; Ollaya's zero-shot NLI models did as well. A score that orders a list for the calling agent to review, rather than a phrase match that gates a write, removes the narrow-phrase constraint. It needs a classifier endpoint, which neither Ollama nor Ollaya's library provides for the three-way model today.
