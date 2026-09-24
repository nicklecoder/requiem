---
id: get-reads-rejections
namespace: cli
kind: rule
modality: must
status: active
provenance:
    type: dialogue
created_at: 2026-09-24T20:54:40.079615701Z
relationships:
    - to: principles/agent-native
      type: refines
      note: An agent follows the documented path literally; a dead end on it is a broken interface.
    - to: retrieval/rejections-embedded
      type: depends_on
      note: Rejections come back from check as candidates only because they are retrieved like statements.
---

get answers for a rejection as well as a statement, printing it with source_kind rejection beside full_id, see_instead and body, and update given a rejection's id without --rejection names the flag instead of reporting the record missing. check returns rejections as candidates and tells the caller to get the full body of any that matters, so a get that knew only statements dead-ended on exactly the record most worth reading: a field report followed the documented path to a rejection and got not found. A statement wins when both share an id, since statements were get's only answer until now.
