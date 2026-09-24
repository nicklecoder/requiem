---
id: graph-neighbours-in-own-section
rejected_at: 2026-09-24T21:06:41.534505423Z
see_instead: retrieval/graph-expansion
---

Return graph neighbours as a separate section of check's output, apart from the ranked hits. Rejected: check prints a bare array, and a section would need the envelope already rejected as cli/coverage-envelope, making every caller unwrap one command's output. match_kind graph with rank 0 separates them within the array the caller already reads.
