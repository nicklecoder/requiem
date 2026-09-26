---
id: init-is-rerunnable
namespace: cli
kind: rule
modality: must
status: active
provenance:
    type: dialogue
created_at: 2026-09-26T06:54:05.583400437Z
relationships:
    - to: embedding/local-endpoint-overlay
      type: depends_on
      note: re-running init moves an endpoint an older config.yaml commits into the overlay
---

requiem init is safe to run again in a project that already has .requiem/, and a fresh clone needs it run: hooks, the local config overlay and the local agent doc file are all things git does not carry. It adds what is missing and leaves the rest, which is also how a project set up by an older requiem is brought up to date — missing lines are appended to .requiem/.gitignore rather than the file being written only on creation, and an endpoint an older config.yaml still commits is moved into config.local.yaml.
