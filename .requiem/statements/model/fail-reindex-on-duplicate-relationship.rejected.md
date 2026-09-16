---
id: fail-reindex-on-duplicate-relationship
rejected_at: 2026-09-14T13:22:27.918256938Z
see_instead: model/relationship-unique-per-pair
---

Keep duplicate relationships a hard error on read, with a message naming the offending file. Rejected: one hand-edited file would still take the whole index and audit down until someone fixed it by hand, and it contradicts tolerating on read what is validated on write.
