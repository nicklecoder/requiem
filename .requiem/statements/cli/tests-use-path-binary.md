---
id: tests-use-path-binary
namespace: cli
kind: question
status: superseded
provenance:
    type: dialogue
created_at: 2026-09-24T21:08:46.573988921Z
relationships:
    - to: model/derivation-downgrade
      type: depends_on
      note: Found through the same failure.
---

Open question: should the test suite put the requiem it just built on PATH, or disable the hooks it installs? Service tests install real git hooks, which run whichever requiem is on PATH, so their results depend on the installed build: the concurrency test failed with SQLITE_BUSY against an older build and passed against the current one. What hangs on it: the hooks are part of what those tests exercise, so disabling them loses coverage, while building a binary per test run slows the suite.
