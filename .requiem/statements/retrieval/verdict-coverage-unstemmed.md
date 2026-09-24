---
id: verdict-coverage-unstemmed
namespace: retrieval
kind: question
status: proposed
provenance:
    type: dialogue
created_at: 2026-09-24T21:00:22.891318198Z
relationships:
    - to: retrieval/calibrated-verdict
      type: depends_on
      note: The coverage proxy it questions is one of the verdict's inputs.
    - to: retrieval/lexical-stemming
      type: depends_on
      note: The gap exists only because retrieval now stems and coverage does not.
---

Open question: should the verdict's term coverage compare stems rather than surface words? Retrieval stems since retrieval/lexical-stemming, but coverageTerms and termCoverage still match exact lowercase words, so a record check now finds only through a stem ('cancel' against 'cancellation') scores low coverage and can come back weak, and an all-weak result prints 'nothing here appears to state this already'. What hangs on it: stemming coverage means tokenizing up to limit bodies through SQLite per check, and the coverage stopword list is written in surface forms, so it would have to be stemmed too or it stops matching.
