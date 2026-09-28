---
id: fail-fast-unreachable
namespace: embedding
kind: rule
modality: must
status: active
provenance:
    type: dialogue
created_at: 2026-09-28T16:59:36.452010087Z
relationships:
    - to: principles/models-are-optional
      type: refines
      via: link
---

Every call to a model endpoint, embedder or classifier, gives up on connecting after 2 seconds, separately from the request timeout that bounds a slow answer. Why: off the LAN, a server's address stops answering rather than refusing, so each call waited out the whole 30-second request timeout: check --semantic, audit's ordering and list --open-wording each stalled 30 seconds and init 65, measured with both servers unreachable. A server that accepts the connection but answers slowly still gets the full request timeout, because a large batch or a cold model legitimately takes that long.
