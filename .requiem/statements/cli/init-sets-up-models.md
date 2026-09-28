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
    - to: principles/models-are-optional
      type: depends_on
---

init leaves requiem fully working. For the embedder, init looks for a configured endpoint in the project's local overlay, then the machine config. Failing that, it tries the server OLLAMA_HOST names, which Ollama's own clients use to reach a remote server, then the usual local one (Ollama at localhost:11434). It never scans the network: a sweep is slow, can trip security tools, and is unnecessary once a machine config holds the endpoint. It then makes one real request to prove the endpoint answers with the configured model, and runs reindex --embed so the corpus is searchable before init returns. An endpoint named with --embedding-endpoint is the only one tried, since someone who names a server means that one. A found endpoint is saved to the machine config by default, or to the project's local overlay if asked. A configured endpoint that fails to connect is kept rather than replaced, and a working one found beside it is added as a fallback (cli/init-keeps-unreachable-endpoint); a model chosen by flag or prompt also becomes the machine's default when it has none, so the next project on the machine needs no input. Without a working endpoint, init completes everything else and says exactly what is missing and which command finishes the setup; check keeps working lexically meanwhile. Because the usual caller is an agent (principles/agent-native), init never blocks on a prompt: flags set or skip each endpoint, and it asks only when a person is at a terminal. A classifier is optional and has no usual address to probe, so init proves one only when it is configured and otherwise leaves it alone (principles/models-are-optional). Why: a user who does not know requiem needs an embedder learns it from a coverage warning much later, after searches that quietly ran lexical-only.
