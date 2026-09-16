---
id: unlink-both-directions
rejected_at: 2026-09-16T15:20:23.452187565Z
see_instead: model/relationships-are-removable
---

Have unlink <a> <b> remove edges in both directions, since someone resolving an audit-flagged pair may not remember which way the link was written. Rejected: relationships are directional and a command that quietly deletes an edge the caller did not name is the wrong kind of helpful — b refines a is a different claim from a refines b. Naming the reverse edge in the error costs the caller one retry and never destroys something they did not ask about.
