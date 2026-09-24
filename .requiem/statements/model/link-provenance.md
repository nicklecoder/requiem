---
id: link-provenance
namespace: model
kind: question
status: proposed
provenance:
    type: dialogue
created_at: 2026-09-24T21:08:46.359011438Z
relationships:
    - to: retrieval/graph-expansion
      type: depends_on
      note: The question got sharper once check started following edges.
---

Open question: should a relationship record where it came from (dialogue, audit, a bulk run) and whether a person reviewed it, so edges applied in bulk can be found and checked later? In a field corpus agents proposed 149 refines and depends_on links and only a sample was read before they were applied, and nothing now tells a reader which edges were checked. What hangs on it: since retrieval/graph-expansion, an edge puts a record into check results, so an unreviewed edge is no longer only a navigation error; and statements already carry provenance while relationships carry only a note.
