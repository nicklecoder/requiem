---
id: committed-vectors
rejected_at: 2026-09-13T21:25:03.878455581Z
see_instead: embedding/committed-pipeline
---

Commit vectors as sidecar files so they survive a clone with no pipeline at all. Rejected: against prevailing practice — vectors are large, opaque in diffs, model-versioned, and churn on every body edit. A 200-statement corpus at 768 dims is roughly 1.5MB of unreviewable noise per re-embed. Committing the pipeline achieves the same guarantee.
