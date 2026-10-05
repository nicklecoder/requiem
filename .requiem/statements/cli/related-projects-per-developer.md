---
id: related-projects-per-developer
namespace: cli
kind: design
modality: must
status: active
provenance:
    type: dialogue
created_at: 2026-10-05T00:43:02.551925642Z
relationships:
    - to: embedding/local-endpoint-overlay
      type: refines
      note: a path to a neighbouring checkout names one machine, like an endpoint
      via: batch
    - to: retrieval/related-projects-opt-in
      type: refines
      via: batch
---

Related projects are configured in the gitignored .requiem/config.local.yaml under related:, as a map from a name the developer chooses to the absolute path of that project's root (~ is expanded). Any location on the machine works; requiem assumes no layout such as sibling directories. A related: section in the committed config.yaml is ignored, and requiem says so, because a path names one machine's checkout. Why: which neighbour a developer has cloned, and where, differs per developer, the same reason the embedding endpoint lives in the local overlay (embedding/local-endpoint-overlay). The name is a local alias, not an identity the other project declares: it never appears in either project's committed files.
