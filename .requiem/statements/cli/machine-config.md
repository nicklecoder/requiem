---
id: machine-config
namespace: cli
kind: design
modality: should
status: active
provenance:
    type: dialogue
created_at: 2026-09-27T16:24:12.605233703Z
relationships:
    - to: embedding/local-endpoint-overlay
      type: refines
      note: adds a per-machine layer beneath the per-project local overlay
---

A per-machine config file in the user's config directory (os.UserConfigDir()/requiem/config.yaml: ~/.config/requiem on Linux, ~/Library/Application Support/requiem on macOS), read beneath every project. Layers apply in order, each field overriding the one before: machine config, then the committed .requiem/config.yaml, then the gitignored .requiem/config.local.yaml. The machine config holds what is true of the machine rather than the project: model endpoints, the name of the environment variable holding an API key, timeouts and concurrency. Model names stay in the committed project config, because every vector in a corpus has to come from one model and the team has to share it (embedding/gitignore-whole-config). The machine config may carry a default model that init writes into a new project's committed config. Why: endpoints rarely differ between projects on one machine, so per-project local files repeat the same line in every repo and every new project or fresh clone starts unconfigured.
