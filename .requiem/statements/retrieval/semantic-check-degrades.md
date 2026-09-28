---
id: semantic-check-degrades
namespace: retrieval
kind: rule
modality: must
status: active
provenance:
    type: dialogue
created_at: 2026-09-28T16:59:39.162224648Z
relationships:
    - to: principles/models-are-optional
      type: refines
      via: link
    - to: embedding/coverage-warning
      type: depends_on
      via: link
    - to: cli/diagnostics-stderr
      type: depends_on
      note: the degraded answer is announced on stderr, keeping stdout bare
      via: batch
---

When check --semantic cannot embed its query because no endpoint answers, it answers from word search alone, and says on stderr that the results are lexical-only and why. It does not fail. Why: check is the command an agent runs before proposing anything, and an error there stops the work while word search still finds most prior decisions; the stderr line keeps the answer honest in the way embedding/coverage-warning does for a partly embedded corpus, so a short list is not mistaken for a thorough one. A query given with --vector is unaffected, since no endpoint is involved.
