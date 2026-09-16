---
id: link-replace-flag
rejected_at: 2026-09-16T15:20:23.450584803Z
see_instead: model/relationships-are-removable
---

Give link a --replace flag that drops whatever relationships already join the pair and writes the new one, so retyping is a single command. Rejected: a pair legitimately carries two relationships of different types, so replace has to guess which ones the caller meant to destroy — retyping conflicts_with to refines would also silently take out a depends_on edge recorded months earlier for unrelated reasons. unlink then link says which edge is going, and costs one more command in exchange for never guessing.
