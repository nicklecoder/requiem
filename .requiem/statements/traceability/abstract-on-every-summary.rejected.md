---
id: abstract-on-every-summary
rejected_at: 2026-09-16T06:25:19.031684849Z
see_instead: traceability/abstract-declarations-are-reviewable
---

Carry abstract as a field on StatementSummary so every list and check result shows it, with omitempty to keep it off the rows where it is false. Rejected: the summary exists to be cheap to read under a context budget, and a conditional key is worse than a constant one — an agent cannot tell a false value from an older requiem that never wrote the field. Nine statements in ten are not abstract, so the information belongs in a filter that asks for them, not in every row that is not.
