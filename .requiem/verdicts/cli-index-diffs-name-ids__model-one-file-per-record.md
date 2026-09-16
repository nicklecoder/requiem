---
a: cli/index-diffs-name-ids
b: model/one-file-per-record
verdict: not_related
decided_at: 2026-09-16T23:15:32.243825856Z
---

reindex names ids because a file path means nothing to the reader, which is true whatever the file layout is. The one-file rule is about addressability for discard, mv and update.
