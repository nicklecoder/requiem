---
id: touches-stands-alone
namespace: retrieval
kind: rule
modality: must
status: active
provenance:
    type: dialogue
created_at: 2026-09-24T20:55:55.566475674Z
relationships:
    - to: retrieval/identifier-facets
      type: refines
      note: identifier-facets promised retrieval by identifier; this makes the identifier sufficient by itself.
---

check --touches <identifier> runs with no --text: an identifier is an exact key and a complete question on its own, so a caller who knows the column or field a change touches must not have to invent prose to ask what was decided about it. The documented form was refused with --text is required, which a field report hit on the first try. --semantic still needs --text, since there is nothing else to embed.
