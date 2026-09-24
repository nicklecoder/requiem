---
id: link-retire-flag
rejected_at: 2026-09-16T06:21:30.98640454Z
see_instead: model/supersedes-does-not-retire
---

Add a --retire flag to link so a supersedes edge and the target's retirement can be recorded in one command. Rejected for now: the failure it addresses is forgetting the second step, and a warning naming the exact command to run addresses that without a second way to spell the same thing. Revisit if the warning is observed being read and ignored. A field session on 2026-09-24 ran the two-step form about eight times and nearly missed the second step once, but reported that the warning caught it every time: evidence the warning works, not that it is ignored, so the condition was not met.
