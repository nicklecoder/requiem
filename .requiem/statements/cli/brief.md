---
id: brief
namespace: cli
kind: rule
modality: should
status: active
provenance:
    type: dialogue
created_at: 2026-09-15T23:55:10.244765038Z
relationships:
    - to: principles/agent-native
      type: refines
      note: A set small enough to paste is the form an agent can actually use.
    - to: model/modality-closed
      type: depends_on
      note: brief selects must and must_not rules with prohibitions first, so its whole selection criterion is the modality enum being closed and meaning something.
    - to: principles/rules-serve-principles
      type: depends_on
      note: brief returns the principles the rules refine, which it can only do because a rule is expected to name the principle it refines.
---

requiem brief returns the minimal set of decisions in force for a namespace: must and must_not rules with prohibitions first, the principles those rules refine, and the ideas already rejected there, each section capped with whatever was cut counted in omitted. A corpus of hundreds of statements had no way to say which handful matter for the change in front of an agent, so it either loaded everything or loaded nothing, and memory nobody loads is indistinguishable from having none. The comparison is a constitution of a few always-in-force rules, served here out of a corpus that is already the memory of decisions.
