---
id: models-are-optional
namespace: principles
kind: principle
modality: must
status: active
provenance:
    type: dialogue
created_at: 2026-09-28T00:39:46.419466858Z
---

requiem runs fully without any model. An embedder and a classifier each improve something that already works without them: without an embedder, check and add search by words alone; without a classifier, audit orders its queue as it did before and add and update run no open-wording check. No command fails, warns on every run, or asks to be set up because a model is absent, with two exceptions: init, whose job is setup and which reports what it could not connect, and a command whose whole job is a model's (check --semantic, reindex --embed), which says what to configure when asked to run without one. Why: a tool that needs a model server before it does anything useful loses everyone who has not set one up yet, and requiem's core (recording decisions, word search, the graph, labels) needs none.
