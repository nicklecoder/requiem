---
id: gitignore-whole-config
rejected_at: 2026-09-26T06:54:05.615138417Z
see_instead: embedding/local-endpoint-overlay
---

Gitignore .requiem/config.yaml whole so every clone configures embedding for itself. Rejected: the model has to be shared — vectors from two models cannot be compared, so clones configuring their own would quietly diverge — and gate.diff is a per-project setting CI has to be able to read. Only the endpoint is machine-specific.
