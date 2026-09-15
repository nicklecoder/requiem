---
id: tokenizer-stoplist
rejected_at: 2026-09-13T21:25:03.906688145Z
see_instead: retrieval/df-filter
---

Configure the FTS5 tokenizer so stopwords never enter the index. Rejected: smallest index and fastest queries, but it changes the index format and bakes the list in at write time, so changing your mind means rebuilding.
