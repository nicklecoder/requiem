---
id: configured-endpoint
namespace: embedding
kind: design
modality: must
status: active
provenance:
    type: dialogue
created_at: 2026-09-13T21:24:38.728785536Z
---

Requiem obtains vectors by calling a configured OpenAI-compatible endpoint. It bundles no model and no inference runtime, so the binary stays static and cross-compiles with CGO disabled. Calling a configured endpoint is the same posture as shelling out to git.
