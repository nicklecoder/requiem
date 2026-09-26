---
id: local-endpoint-overlay
namespace: embedding
kind: rule
modality: must
status: active
provenance:
    type: dialogue
created_at: 2026-09-26T06:54:05.531182151Z
relationships:
    - to: embedding/committed-pipeline
      type: refines
      note: narrows what is committed to the model; the endpoint is per machine
    - to: embedding/configured-endpoint
      type: refines
      note: 'says where the configured endpoint is recorded: per machine, not in git'
---

The embedding endpoint lives in a gitignored .requiem/config.local.yaml, overlaid on the committed config.yaml field by field with the local value winning. An endpoint names one machine — a localhost Ollama on one laptop is a server on the LAN from another — so committing it both publishes a detail of someone's network and hands every other clone an address that is wrong for them. The model stays committed, because the model is what makes vectors comparable across clones; field-by-field overlay is what lets the usual local file carry nothing but an endpoint.
