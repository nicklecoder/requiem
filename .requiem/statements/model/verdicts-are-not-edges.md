---
id: verdicts-are-not-edges
namespace: model
kind: rule
modality: must
status: active
provenance:
    type: dialogue
created_at: 2026-09-16T00:03:07.020193417Z
relationships:
    - to: principles/agent-native
      type: refines
      note: The graph is read under a context budget; bookkeeping crowds out meaning.
---

An audit dismissal is recorded as a verdict file under .requiem/verdicts/, never as a relationship in the statement graph. A dismissal asserts only that somebody looked at a pair, and one real corpus accumulated 75 not_related edges on that basis: permanent noise in the structure people read to understand how decisions fit together. Real findings stay edges, because conflicts_with, duplicates and supersedes each say something about the decisions themselves. A verdict excludes its pair from the sweep exactly as a relationship does, lives in a file like every other canonical record, and can be taken back with dismiss --restore so the pair returns to the queue. link still reads not_related from an older corpus but refuses to write one, naming dismiss instead.
