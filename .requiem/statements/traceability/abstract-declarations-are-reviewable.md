---
id: abstract-declarations-are-reviewable
namespace: traceability
kind: rule
modality: must
status: active
provenance:
    type: dialogue
created_at: 2026-09-16T06:25:09.538196076Z
relationships:
    - to: principles/no-silent-success
      type: refines
---

list --abstract narrows to the statements declaring that no code can implement them, so the suppressions are reviewable in bulk. --abstract is an unverifiable author assertion that quiets --unreferenced, and the one check that exists — audit flagging an abstract statement code turns out to reference — only catches the declaration falsified by evidence. It cannot catch the likelier error: a statement marked abstract to silence --unreferenced when it should have been implemented. Every linter that ships nolint or noqa grows a way to list them for the same reason. A filter rather than a field on StatementSummary: the summary is deliberately compact for context economy, and a bool that is false for nine rows in ten is noise in every result an agent reads, where --abstract sits beside --unreferenced and --needs-embedding as one more narrowing flag.
