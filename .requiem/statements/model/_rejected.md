---
id: kind-as-closed-enum
rejected_at: 2026-09-13T21:25:03.909656016Z
see_instead: model/modality-closed
---

Give kind defined semantics as a closed enum — requirement, rule, design, invariant, preference. Rejected: it welds shut the axis that genuinely resists closure. 'Session tokens are never stored in plaintext' is defensibly a rule, an invariant, a requirement or a security constraint, and different agents would pick differently. Normative strength is the axis that closes cleanly.
<!-- requiem:entry -->
---
id: repeat-link-appends
rejected_at: 2026-09-14T13:22:27.914610047Z
see_instead: model/relationship-unique-per-pair
---

Let link append another relationship of the same type between a pair, each keeping its own note. Rejected: a note records the reason for one verdict, two same-typed notes on one pair have no defined reading, and the index's (from, to, type) key turned the second entry into a reindex failure.
<!-- requiem:entry -->
---
id: fail-reindex-on-duplicate-relationship
rejected_at: 2026-09-14T13:22:27.918256938Z
see_instead: model/relationship-unique-per-pair
---

Keep duplicate relationships a hard error on read, with a message naming the offending file. Rejected: one hand-edited file would still take the whole index and audit down until someone fixed it by hand, and it contradicts tolerating on read what is validated on write.
