---
id: verdict-coverage-stemmed
namespace: retrieval
kind: rule
modality: must
status: active
provenance:
    type: dialogue
created_at: 2026-09-27T17:06:59.213797606Z
relationships:
    - to: retrieval/verdict-coverage-unstemmed
      type: supersedes
      note: answers the question
    - to: retrieval/calibrated-verdict
      type: refines
    - to: retrieval/lexical-stemming
      type: depends_on
---

The verdict's term coverage compares stems, produced by the same SQLite tokenizer that stems the full-text index, and its stopword list is stemmed the same way. Why: retrieval stems (retrieval/lexical-stemming) while coverage compared surface words, so check could find a record through a stem ('cancel' against 'cancellation'), score it weak, and on an all-weak result print that nothing states the draft already, a false all-clear on the offline default path where word overlap is the only verdict evidence. Using the index's own tokenizer means retrieval and verdict can never disagree about what a word is; it costs tokenizing at most --limit bodies per check.
