---
id: coverage-envelope
rejected_at: 2026-09-13T21:25:03.912777826Z
see_instead: cli/diagnostics-stderr
---

Wrap audit and check output in {coverage, results} so the shortfall is machine-readable. Rejected: unambiguous and impossible to miss, but it breaks the no-envelope convention for exactly two commands, so every caller must unwrap and the CLI becomes inconsistent. stderr reaches an agent without touching stdout.
<!-- requiem:entry -->
---
id: traceability-percentage
rejected_at: 2026-09-13T21:25:03.916002613Z
see_instead: principles/retrieval-not-judge
---

Report a traceability coverage percentage so requirements without implementing code are visible. Rejected: a number people manage toward turns retrieval infrastructure into a compliance scorecard, which is the failure mode of every requirements-traceability tool. Adoption can gate whether a signal is meaningful without ever being a score.
