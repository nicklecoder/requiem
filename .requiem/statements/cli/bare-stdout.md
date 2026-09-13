---
id: bare-stdout
namespace: cli
kind: rule
modality: must
status: active
provenance:
    type: dialogue
created_at: 2026-09-13T21:24:38.704449837Z
relationships:
    - to: principles/agent-native
      type: refines
      note: no envelope to unwrap on the common path
---

Commands write bare JSON data to stdout on success, with no wrapper envelope to unwrap on the common path. Failures write to stderr with a nonzero exit code.
