---
id: multi-source-provenance
namespace: model
kind: question
status: proposed
provenance:
    type: dialogue
created_at: 2026-09-16T04:27:03.398907761Z
relationships:
    - to: retrieval/staleness-travels-with-results
      type: depends_on
      note: 'The question names the rehash-on-read comparison as something that hangs on it: if provenance holds several ranges, stale becomes a per-source answer rather than the one flag this statement ships with every result.'
    - to: traceability/diff-scoped-check
      type: depends_on
      note: check --diff matches a change against a statement's own source range, so a provenance holding several ranges changes what this reports and how a partial overlap reads.
---

Open question: should a code-derived statement name more than one file and line range? Today provenance holds exactly one, and in the field that was the wrong shape for most of a real corpus: the decisions there lived across four provider modules plus a migration, so writing one down forced picking a single site and losing the rest. That weakens staleness detection precisely where drift is most likely, since the ranges left unrecorded are the ones nobody rehashes. What hangs on it: the provenance record, the rehash-on-read comparison, and whether stale becomes a per-source answer rather than one flag. Deferred deliberately rather than built: one project asked for it, and the cost of guessing the shape wrong is a migration of every code-derived statement. Revisit when a second project shows decisions spread the same way.
