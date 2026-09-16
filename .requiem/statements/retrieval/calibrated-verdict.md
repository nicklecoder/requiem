---
id: calibrated-verdict
namespace: retrieval
kind: rule
modality: must
status: active
provenance:
    type: dialogue
created_at: 2026-09-15T23:45:43.54288515Z
relationships:
    - to: principles/no-silent-success
      type: refines
      note: A full result list that means nothing is a silent non-answer.
    - to: retrieval/identifier-facets
      type: depends_on
      note: A shared identifier is the evidence that promotes a pair prose similarity would leave in the noise.
    - to: principles/retrieval-not-judge
      type: refines
      note: 'The statement draws the line itself: the band is calibration, not adjudication, and requiem still does not decide whether two decisions conflict.'
    - to: retrieval/rank-direction
      type: depends_on
      note: 'The verdict band exists because rank deliberately carries no absolute meaning: the scale is outside the contract and comparable only within one result set, so fusion scoring position and discarding magnitude cannot say whether a top result is a duplicate or noise.'
---

Every check result carries a verdict of duplicate, related or weak, together with the evidence behind it: the raw cosine where a vector was compared, and the identifiers the draft and the record share. check always fills its limit, so a duplicate at rank 1 and noise at rank 1 were indistinguishable and an agent could never conclude that a draft is new. The fused rank cannot answer that by construction, since reciprocal rank fusion scores position and discards magnitude, so the top result of a hopeless query scores exactly what the top result of a perfect one does. A whole result set coming back weak is the answer that nothing here states this already, and it is reported on stderr as well. The band is calibration, not adjudication: requiem still does not decide whether two decisions conflict.
