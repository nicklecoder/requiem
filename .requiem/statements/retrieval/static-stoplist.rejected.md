---
id: static-stoplist
rejected_at: 2026-09-13T21:25:03.903447039Z
see_instead: retrieval/df-filter
---

Drop a fixed list of common English words at query-build time. Rejected: simple and language-specific, but blind to domain stopwords — 'token' saturating an auth-heavy corpus is the case that actually hurts, and no English list catches it.
