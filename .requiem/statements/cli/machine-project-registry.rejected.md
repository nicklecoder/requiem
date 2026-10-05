---
id: machine-project-registry
rejected_at: 2026-10-05T00:43:02.553479932Z
see_instead: cli/related-projects-per-developer
---

Give every project a committed name (project: versine-ce), record each project's root in a per-machine registry whenever requiem runs there, and have links name projects rather than paths, so a checkout can move without editing config. Rejected: it binds projects together through shared identity and machine-wide state the developer did not ask for, and needs rules for two clones or a worktree registering one name; one path per related project in the local overlay is the whole requirement.
