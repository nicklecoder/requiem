---
id: local-doc-in-project-gitignore
rejected_at: 2026-09-26T06:54:05.618624472Z
see_instead: cli/agent-docs-local-by-default
---

Add CLAUDE.local.md to the project's root .gitignore when init creates it. Rejected: that is one contributor's requiem editing a tracked file every checkout carries, the thing writing to CLAUDE.local.md was meant to avoid. .git/info/exclude does the same job and git never shares it.
