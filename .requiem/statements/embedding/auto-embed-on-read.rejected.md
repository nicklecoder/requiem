---
id: auto-embed-on-read
rejected_at: 2026-09-13T21:25:03.896800038Z
see_instead: embedding/read-path-offline
---

Have audit and check fill missing vectors before answering, mirroring the lazy reindex-on-read trigger. Rejected: coverage would always be complete, but it puts network I/O and unbounded latency in a read path and turns check into a hang when the endpoint is down.
