---
id: retired-is-not-unimplemented
namespace: model
kind: rule
modality: must_not
status: active
provenance:
    type: dialogue
created_at: 2026-09-14T04:28:36.773245659Z
---

A superseded or deprecated statement must not be reported by list --unreferenced. It has no implementation because the decision was withdrawn, not because anyone skipped the work, so reporting it is true and useless — and it crowds out findings that are neither. --direct does not reach this: that flag suppresses an inference, and a statement's status is a fact. Same reasoning as Status.Searchable excluding retired statements from check and audit.
