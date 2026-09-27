---
id: derivation-downgrade
namespace: model
kind: question
status: superseded
provenance:
    type: dialogue
created_at: 2026-09-24T21:08:46.547738304Z
relationships:
    - to: model/derived-data-is-versioned
      type: depends_on
      note: The mechanism that ping-pongs.
---

Open question: what should happen when two requiem builds share one index, so that each resets derivation_version to its own value? A git hook runs whatever requiem is on PATH, so an older installed build and a newer one in use rebuild each other's derived data on every alternation, and the older one queries a stemmed index with unstemmed frequency lookups. Found when the stemming change moved the version to 5 and a hook-driven test began failing with SQLITE_BUSY. What hangs on it: rebuilding only when the stored version is older would stop a new build undoing an old one, but cannot change builds already installed.
