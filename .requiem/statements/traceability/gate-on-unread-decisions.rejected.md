---
id: gate-on-unread-decisions
rejected_at: 2026-09-16T00:11:46.736515342Z
see_instead: traceability/diff-gate-is-per-project
---

Fail the build when a change touches code covered by decisions the author did not demonstrably read, which is what an analyze-style gate in a forward spec workflow approximates. Rejected: requiem cannot observe whether anyone read anything, so the rule would have to proxy it — by label coverage, or by requiring an acknowledgement — and a proxy for intent is wrong often enough to be disabled, taking the honest findings with it. The gate fails on facts instead: code labelled with a retired or rejected decision, and statements whose source range has drifted.
