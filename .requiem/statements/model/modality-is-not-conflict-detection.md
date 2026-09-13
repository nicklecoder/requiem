---
id: modality-is-not-conflict-detection
namespace: model
kind: rule
modality: must_not
status: active
provenance:
    type: dialogue
created_at: 2026-09-13T21:24:38.76242484Z
relationships:
    - to: principles/retrieval-not-judge
      type: refines
      note: a decidable signal is not a verdict
---

Modality must not be presented as conflict detection. Contraries defeat it entirely: 'must be red' and 'must be blue' contradict each other while both are must. It buys one narrow decidable signal — opposed polarity on a similar subject — and nothing more.
