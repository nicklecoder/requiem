---
id: facet-vocabulary-excluded
namespace: retrieval
kind: rule
modality: must
status: active
provenance:
    type: dialogue
created_at: 2026-09-16T00:08:27.142456905Z
relationships:
    - to: retrieval/identifier-facets
      type: refines
      note: Which tokens count as identifiers, once a real corpus showed which ones do not.
---

Requiem own field names and enum values — must_not, see_instead, full_id and the rest of its interface vocabulary — are excluded from the facet index, because a statement mentioning one is describing how decisions get recorded rather than naming anything in the system the corpus describes. Left in they dominated a real audit: the top three candidate pairs in requiem own corpus were unrelated statements that happened to mention must_not. Document frequency cannot separate them, which is the part worth recording — must_not sat in 3 in-scope statements while genuine domain identifiers sat in 2, so inverse-frequency weighting ranks the noise above the signal. The separating property is provenance rather than rarity, and the only vocabulary requiem can know with certainty is its own; a framework vocabulary belonging to the project under description is what the per-facet record ceiling is for. An identifier named explicitly with check --touches is never filtered, since naming one says that it matters.
