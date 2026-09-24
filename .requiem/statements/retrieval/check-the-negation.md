---
id: check-the-negation
namespace: retrieval
kind: guideline
modality: should
status: active
provenance:
    type: dialogue
created_at: 2026-09-24T21:01:18.133159304Z
relationships:
    - to: retrieval/calibrated-verdict
      type: refines
      note: Qualifies what an all-weak result can be taken to mean.
    - to: principles/retrieval-not-judge
      type: refines
      note: The agent writes the opposing query, since only a reader of the claim can invert it.
---

The agent instructions tell an agent to run check a second time on the negation of any idea that permits, requires or forbids something, phrased the way the author of an opposing rule would have put it. Lexical and semantic search both measure resemblance, and a conflicting rule is usually worded from the other side: in a field report the rule a cancellation draft contradicted had a cosine of 0.63 against it while an agreeing statement had 0.79, and it was absent from the semantic top 40 of about 262. The same instructions say plainly that an all-weak result means nothing is worded like the draft, not that nothing contradicts it.
