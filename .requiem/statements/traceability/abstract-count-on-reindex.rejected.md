---
id: abstract-count-on-reindex
rejected_at: 2026-09-16T06:25:19.033605988Z
see_instead: traceability/abstract-declarations-are-reviewable
---

Report a count of abstract statements on stderr from reindex, alongside its near-miss reporting. Rejected: a count says how many suppressions exist but not which, and reviewing them is the whole point — the reader has to go find them anyway, now knowing only that the search will not be empty. It also fires on every reindex, which is most of them, for a review that happens rarely.
