---
id: init-sets-up-models
namespace: cli
kind: design
modality: must
status: active
provenance:
    type: dialogue
created_at: 2026-09-27T16:24:12.670849464Z
relationships:
    - to: cli/init-is-rerunnable
      type: refines
      note: a re-run also finishes a model setup that was skipped
    - to: cli/machine-config
      type: depends_on
      note: a found endpoint is saved to the machine config by default
    - to: principles/agent-native
      type: depends_on
      note: flags and a report rather than prompts
    - to: embedding/configured-endpoint
      type: depends_on
      note: init finds and proves the endpoint requiem will call
---

init leaves requiem fully working. For each kind of model requiem depends on (the embedder today, any classifier added later), init looks for a configured endpoint in the project's local overlay, then the machine config. Failing that, it probes the usual local server (Ollama at localhost:11434). It then makes one real request to prove the endpoint answers with the configured model, and runs reindex --embed so the corpus is searchable before init returns. A found endpoint is saved to the machine config by default, or to the project's local overlay if asked. Without a working endpoint, init completes everything else and says exactly what is missing and which command finishes the setup; check keeps working lexically meanwhile. Because the usual caller is an agent (principles/agent-native), init never blocks on a prompt: flags set or skip each endpoint, and it asks only when a person is at a terminal. Why: a user who does not know requiem needs an embedder learns it from a coverage warning much later, after searches that quietly ran lexical-only.
