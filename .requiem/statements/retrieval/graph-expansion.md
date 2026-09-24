---
id: graph-expansion
namespace: retrieval
kind: rule
modality: must
status: active
provenance:
    type: dialogue
created_at: 2026-09-24T21:06:41.520753794Z
relationships:
    - to: principles/no-silent-success
      type: refines
      note: A contradicting rule both retrievers miss otherwise produces an answer that reads as 'nothing conflicts'.
    - to: cli/bare-stdout
      type: depends_on
      note: Why neighbours are appended to the one array rather than returned as a section of their own.
---

After its ranked hits, check appends up to five records, and never more than --limit, one recorded edge away from the top three hits, each marked match_kind graph with rank 0 and a via field naming the edge as written. Edges of type conflicts_with and supersedes are followed first, then duplicates, see_instead (a rejection turned down in favour of a hit, or the decision a rejected hit points at), refines and depends_on, and neighbours obey the same status and namespace scope as direct hits. Lexical and semantic search both measure resemblance to the draft's wording, and a rule the draft contradicts is usually worded from the other side, so it can miss both lists, leaving fusion nothing to promote; the graph is the one retriever that does not depend on wording. Expansion is skipped under --tags, which narrows to records carrying them, and is kept out of add's duplicate gate, since a record reached only through an edge is not evidence that a draft repeats it.
