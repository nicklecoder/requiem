---
id: related-projects-opt-in
namespace: retrieval
kind: design
modality: may
status: active
provenance:
    type: dialogue
created_at: 2026-10-05T00:42:56.727465915Z
---

check, get and brief may reach a related project — another requiem project on the same machine — but only when the caller names it with --related (or --related all for every one configured); without the flag every command answers from this project alone, exactly as before. Only the projects named are searched, never the projects they name in turn. Why: related projects refer to each other (hosted versine cites the open-source versine-ce in 7 of its 28 statements, in hand-written prose like "(versine-ce: server/sqlite-specific-sql)"), so an agent working in one sometimes has to read the other's requirements; but the two are independent corpora, and searching a neighbour on every check would put another project's decisions in front of every draft, most of which concern only this one.
