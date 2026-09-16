---
id: check-scope-defaults-to-corpus
namespace: retrieval
kind: rule
modality: must
status: active
provenance:
    type: dialogue
created_at: 2026-09-16T06:13:33.26882942Z
relationships:
    - to: principles/agent-native
      type: refines
---

Omitting --namespace on check searches every namespace rather than failing. Requiring a scope makes the agent name the area before it is allowed to ask the question, and the prior decisions it is least able to anticipate are exactly the ones filed somewhere it would not have thought to look: in a trial corpus a draft adding political levers under ai/director needed checking against rules in information/ and factions/, and the agent worked around the required flag with broad prefixes and a separate call per area. The index has always treated an empty namespace as no filter, so scoped and unscoped retrieval are the same query with and without a WHERE clause — nothing new is searched, it was simply unreachable. --namespace remains available and still matches a namespace together with its children by prefix; audit and list already take it as optional.
