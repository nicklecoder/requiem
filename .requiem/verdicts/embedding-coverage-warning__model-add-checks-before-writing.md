---
a: embedding/coverage-warning
b: model/add-checks-before-writing
verdict: not_related
decided_at: 2026-09-16T23:15:32.307177758Z
---

Both concern the limits of an offline check, at different moments. add's duplicate check is lexical by design so a write path never hangs, and the coverage warning belongs to the semantic path it deliberately does not take.
