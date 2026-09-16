---
id: supersedes-sets-status
rejected_at: 2026-09-16T06:21:30.984711292Z
see_instead: model/supersedes-does-not-retire
---

Have link --type supersedes set the target's status to superseded automatically, reporting it on stderr the way mv reports rewritten refs, since the relationship type has exactly one meaning. Rejected: it infers a consequence from a recorded fact, which is not what requiem does, and it makes the migration window impossible to express — A supersedes B while B is still in force is a real state, and an automatic retirement silently overwrites the author's intent with the tool's.
