---
id: derived-data-never-downgrades
namespace: model
kind: rule
modality: must
status: active
provenance:
    type: dialogue
created_at: 2026-09-27T17:10:52.967563326Z
relationships:
    - to: model/derivation-downgrade
      type: supersedes
      note: answers the question
    - to: model/derived-data-is-versioned
      type: refines
---

A requiem build rebuilds the index's derived data only when the stored derivation_version is older than its own. A build that finds a newer version leaves the derived data alone and warns on stderr that the index was built by a newer requiem and this one should be upgraded; it does not refuse, so an older build run by a git hook never blocks a commit. Why: hooks run whichever requiem is on PATH, so an installed older build and a newer one in use rebuilt each other's derived data on every alternation, and the older one then queried a stemmed index with unstemmed lookups; one hook-driven test failed with SQLITE_BUSY. This protects from the build that adopts it onward and cannot change builds already installed.
