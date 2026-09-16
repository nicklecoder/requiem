---
id: flag-every-removed-label-line
rejected_at: 2026-09-16T15:01:41.876203164Z
see_instead: traceability/dropped-labels-are-reported
---

Report every label on a removed line in the patch. Rejected: moving a function between two files removes the label from one and adds it to the other, which is the commonest refactor there is and would fire constantly. Rescanning the tree after the change answers it without counting: a label that moved is still there, and only a label that is gone is gone.
