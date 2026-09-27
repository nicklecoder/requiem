---
id: tests-build-their-binary
namespace: cli
kind: rule
modality: must
status: active
provenance:
    type: dialogue
created_at: 2026-09-27T17:11:34.240602863Z
relationships:
    - to: cli/tests-use-path-binary
      type: supersedes
      note: answers the question
    - to: model/derived-data-never-downgrades
      type: depends_on
      note: a stale PATH binary also rebuilt derived data
---

The test suite builds requiem from the tree under test once per run, in TestMain, and puts that binary first on PATH, so every git hook a test installs runs the code being tested. Why: service tests install real hooks, which run whichever requiem is on PATH, so results depended on the installed build: the concurrency test failed with SQLITE_BUSY against an older build and passed against the current one. Disabling the hooks would lose coverage of what those tests exercise; one cached go build per run costs seconds.
