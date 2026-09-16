---
id: label-false-positives
namespace: traceability
kind: design
modality: should
status: active
provenance:
    type: dialogue
created_at: 2026-09-13T22:51:24.917821639Z
relationships:
    - to: traceability/code-labels
      type: depends_on
      note: a cost of the marker-in-prose approach
    - to: traceability/marker-tolerance
      type: depends_on
      note: 'The false positives are the cost of the tolerance: matching any spacing and any case is what makes prose and a stderr message parse as labels. Recorded as a dependency rather than a conflict because the tolerant matcher is kept deliberately and requiem:ignore is the reconciliation.'
---

Prose discussing the label format scans as a label, so documentation explaining traceability creates phantom references. The same shape arrives from a third direction: any string whose text begins with the marker followed by a word — a stderr message reading "requiem: showing 6 of 90 pairs" — parses as a label named showing. Nine such messages and two prose comments existed in this repository at once. It was thought contained because those ids classify as dangling and audit ignores them, and that stopped being true the moment check --diff reported one as a decision covering a change: a reader was shown a decision named reporting that does not exist. Two remedies, both needed. The diff report skips a dangling reference, since a label naming nothing cannot cover anything. And a line carrying a lookalike gets requiem:ignore — on that exact line, which is the part easy to get wrong: a marker on the preceding line suppresses nothing, and a format constant or a documentation example keeps the marker inside its string with the ignore comment outside it. Requiring a comment context instead would need language-aware parsing, and a markdown fence showing a comment would still match.
