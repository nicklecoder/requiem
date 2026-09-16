---
id: agent-supplies-every-vector
rejected_at: 2026-09-13T21:25:03.893412492Z
see_instead: embedding/configured-endpoint
---

Keep requiem inference-agnostic by having the agent supply every vector by hand. Rejected: it left the index's disposability claim false, since vectors could not be regenerated, and made re-embedding after a clone a manual per-statement loop costing 1k-8k context tokens each. The rule conflated 'bundles no inference runtime', worth defending, with 'cannot obtain a vector', which was the costly half.
