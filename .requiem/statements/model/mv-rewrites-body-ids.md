---
id: mv-rewrites-body-ids
namespace: model
kind: rule
modality: must
status: active
provenance:
    type: dialogue
created_at: 2026-09-27T17:15:42.707882458Z
relationships:
    - to: model/body-id-references
      type: supersedes
      note: answers the question
    - to: model/see-instead-is-checked
      type: depends_on
      note: the same upkeep see_instead gets, without its validation
---

mv rewrites every occurrence of the moved statement's full id in the bodies of other statements and of rejections, matching only text exactly equal to that id and bounded by characters that cannot belong to an id, so a longer id or a file path that merely contains it is left alone. The rewritten statements are written and staged like any other mv change and listed in its output. Ids in bodies are not validated otherwise: a misspelled id matches nothing and stays as written. Why: agents write ids into bodies as plain text (about 30 in one field session), and a move orphaned every one of them silently. Matching only exact existing ids removes the false-match risk of treating anything shaped like namespace/slug as a reference, and covers the ids already written without new syntax.
