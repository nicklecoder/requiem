---
id: edge-origin-and-review
namespace: model
kind: rule
modality: should
status: active
provenance:
    type: dialogue
created_at: 2026-09-27T17:16:38.829906443Z
relationships:
    - to: model/link-provenance
      type: supersedes
      note: answers the question with the minimal version
    - to: retrieval/graph-expansion
      type: depends_on
    - to: model/relationships-are-removable
      type: depends_on
      note: an unconfirmed edge is removed with unlink
---

Each relationship records how it was made: link, batch, or an audit finding. An edge applied through batch starts unconfirmed, and audit lists unconfirmed edges until the calling agent confirms one or removes it with unlink; a confirmed edge never returns to the list. An edge made with a single link counts as confirmed, because it was written deliberately. Why: in a field corpus agents proposed 149 refines and depends_on links and applied them in bulk after reading a sample, and nothing told a later reader which were checked. Since retrieval/graph-expansion an edge puts records into check results, so an unreviewed edge changes answers rather than only navigation.
