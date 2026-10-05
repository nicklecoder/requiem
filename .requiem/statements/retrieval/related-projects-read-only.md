---
id: related-projects-read-only
namespace: retrieval
kind: rule
modality: must_not
abstract: true
status: active
provenance:
    type: dialogue
created_at: 2026-10-05T00:42:56.848621708Z
relationships:
    - to: retrieval/related-projects-stay-separate
      type: refines
      via: batch
---

requiem must not write a related project's records: add, update, link, reject, dismiss, audit verdicts, mv, discard and commit act only on the current project, and no relationship may point into a related project. Why: the projects stay independent corpora with their own review and commit history, and a relationship naming another project's record would be committed into a repository whose other clones may not have that project at all, so it could never be checked there.
