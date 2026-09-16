---
id: namespaces-as-tags
namespace: model
kind: question
status: proposed
provenance:
    type: dialogue
created_at: 2026-09-16T04:27:03.425484189Z
---

Open question: should a statement belong to more than one namespace, or should namespaces become tags? In the field most duplicates turned out to be one decision seen from two areas, and mv was used eight times to choose a home while the choice stayed arguable afterwards. Against the change: SPEC already dropped scoped_to on the reasoning that a namespace expresses what a statement applies to, tags already exist for cross-cutting grouping, and the namespace is the file path, so multiple membership means either symlink-like duplication or breaking the one-file-per-record mapping that discard and mv depend on. What hangs on it: the path mapping, check scoping, brief scoping, and audit scoping. Revisit when a second project reports the same ambiguity, with the tags field tried first.
