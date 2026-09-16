---
id: coverage-envelope
rejected_at: 2026-09-13T21:25:03.912777826Z
see_instead: cli/diagnostics-stderr
---

Wrap audit and check output in {coverage, results} so the shortfall is machine-readable. Rejected: unambiguous and impossible to miss, but it breaks the no-envelope convention for exactly two commands, so every caller must unwrap and the CLI becomes inconsistent. stderr reaches an agent without touching stdout.
