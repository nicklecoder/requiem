---
id: shell-command-template
rejected_at: 2026-09-13T21:25:03.874201355Z
see_instead: embedding/configured-endpoint
---

Store a shell command template in config and shell out per statement to obtain a vector. Rejected: maximum provider flexibility, but fragile quoting around the text placeholder, a shell-injection surface, platform-dependent behaviour on Windows, and harder to test than an HTTP call.
<!-- requiem:entry -->
---
id: committed-vectors
rejected_at: 2026-09-13T21:25:03.878455581Z
see_instead: embedding/committed-pipeline
---

Commit vectors as sidecar files so they survive a clone with no pipeline at all. Rejected: against prevailing practice — vectors are large, opaque in diffs, model-versioned, and churn on every body edit. A 200-statement corpus at 768 dims is roughly 1.5MB of unreviewable noise per re-embed. Committing the pipeline achieves the same guarantee.
<!-- requiem:entry -->
---
id: agent-supplies-every-vector
rejected_at: 2026-09-13T21:25:03.893412492Z
see_instead: embedding/configured-endpoint
---

Keep requiem inference-agnostic by having the agent supply every vector by hand. Rejected: it left the index's disposability claim false, since vectors could not be regenerated, and made re-embedding after a clone a manual per-statement loop costing 1k-8k context tokens each. The rule conflated 'bundles no inference runtime', worth defending, with 'cannot obtain a vector', which was the costly half.
<!-- requiem:entry -->
---
id: auto-embed-on-read
rejected_at: 2026-09-13T21:25:03.896800038Z
see_instead: embedding/read-path-offline
---

Have audit and check fill missing vectors before answering, mirroring the lazy reindex-on-read trigger. Rejected: coverage would always be complete, but it puts network I/O and unbounded latency in a read path and turns check into a hang when the endpoint is down.
