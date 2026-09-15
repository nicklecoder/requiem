---
id: batch-input
namespace: cli
kind: rule
modality: should
status: active
provenance:
    type: dialogue
created_at: 2026-09-15T23:55:10.221053372Z
relationships:
    - to: principles/agent-native
      type: refines
      note: One invocation per record is a cost paid by the agent, not the human.
---

requiem batch applies many writes from JSON Lines on stdin and returns one result per record. One process per record does not survive a real ingestion: reconstructing a corpus from git history and code meant roughly 800 records, and the caller had to write a loader to drive them one invocation at a time. A line that does not parse aborts the batch before anything is written, because that is a defect in the caller and identical on a retry, while a write requiem refuses is a finding about this corpus and is reported against its own line with the remaining records still applied. No transaction of its own is needed: writes stage and commit nothing, so discard backs the whole batch out.
