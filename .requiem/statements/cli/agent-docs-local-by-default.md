---
id: agent-docs-local-by-default
namespace: cli
kind: rule
modality: must
status: active
provenance:
    type: dialogue
created_at: 2026-09-26T06:54:05.613181762Z
relationships:
    - to: cli/init-is-rerunnable
      type: depends_on
      note: moving an uncommitted block out of shared files is part of bringing an older project over
---

init writes the agent workflow block to CLAUDE.local.md by default, not to AGENTS.md or CLAUDE.md, and keeps that file out of git through .git/info/exclude rather than the project's .gitignore. One contributor adopting requiem is no reason to change the agent instructions every other contributor's checkout loads, or to edit a file they all carry. A team that has adopted requiem opts in with init --shared; a block already committed to a shared file is refreshed where it stands, since committing it was the team's choice, and one never committed is moved to the local file. Because Claude Code stops reading AGENTS.md once a CLAUDE.local.md exists, a local file created beside an AGENTS.md with no CLAUDE.md begins with an @AGENTS.md import.
