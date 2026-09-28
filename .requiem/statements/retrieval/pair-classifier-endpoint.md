---
id: pair-classifier-endpoint
namespace: retrieval
kind: question
status: superseded
provenance:
    type: dialogue
created_at: 2026-09-27T16:47:18.392323623Z
relationships:
    - to: retrieval/audit-orders-by-pair-classifier
      type: depends_on
    - to: cli/init-sets-up-models
      type: depends_on
      note: init must be able to probe and prove the endpoint
    - to: embedding/configured-endpoint
      type: depends_on
      note: whatever the contract, requiem calls a configured endpoint and bundles no model
---

Open question: what endpoint does requiem call for a pair classifier? Candidates are an NLI-shaped request (premise and hypothesis pairs in; entailment, neutral and contradiction probabilities out), the TypeSafe /v1/systemone format Ollaya speaks, or a rerank-style endpoint. What hangs on it: neither Ollama nor Ollaya's library serves the three-way NLI model the measurement used, and a test on the M4 Mac Mini (an Ollaya import, or a small fallback server) will show what can be served there; cli/init-sets-up-models has to probe and prove whichever is chosen, and the contract should not be fixed to one server's quirks.
