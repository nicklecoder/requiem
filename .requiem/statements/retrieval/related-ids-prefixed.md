---
id: related-ids-prefixed
namespace: retrieval
kind: design
modality: must
status: active
provenance:
    type: dialogue
created_at: 2026-10-05T00:42:56.822081107Z
relationships:
    - to: retrieval/related-projects-stay-separate
      type: depends_on
      via: batch
---

A record from a related project is addressed as <name>:<namespace>/<id>, where name is the alias from config.local.yaml: check reports related candidates with that full_id and a project field, and get accepts it. The colon cannot occur in a namespace or id, whose segments are lowercase slugs, so the prefix is unambiguous. Why: two related projects commonly share namespace names — hosted versine and versine-ce both have server/ and principles/ — so an unprefixed id from a neighbour is ambiguous at the moment an agent passes it to get, and the prefix matches the form agents already write by hand when citing the other project.
