---
id: report-partial-label-drops
rejected_at: 2026-09-16T15:01:41.877637175Z
see_instead: traceability/dropped-labels-are-reported
---

Report a decision whose label count falls without reaching zero — three sites down to two. Rejected: consolidating two labelled call sites into one is ordinary and correct, so the finding would be noise on a routine refactor, and it cannot be told apart from a real loss without knowing what the author meant. Reaching zero is unambiguous: the decision has no implementation left pointing at it.
