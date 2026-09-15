---
id: retrieval-not-judge
namespace: principles
kind: design
abstract: true
status: active
provenance:
    type: dialogue
created_at: 2026-09-13T21:24:38.693792447Z
---

Retrieval surfaces candidates and never adjudicates them: deciding whether two statements genuinely conflict is reasoning, and reasoning stays with the calling agent. A write may refuse when requiem own check reports a duplicate — see model/add-checks-before-writing — and that refusal is overridable by construction, so the tool states a finding while the agent still decides. The line this principle holds is that requiem never rules on substance: it does not decide whether two decisions conflict, it does not score the people using it, and it never blocks with no way past.
