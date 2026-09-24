---
id: body-id-references
namespace: model
kind: question
status: proposed
provenance:
    type: dialogue
created_at: 2026-09-24T21:08:46.421509534Z
relationships:
    - to: model/see-instead-is-checked
      type: depends_on
      note: The same guarantee, extended from one field to free text.
---

Open question: should statement ids written in body text be validated on write and rewritten by mv, as see_instead is? One field session wrote about 30 ids into bodies as plain text, and nothing checks that they resolve or keeps them current when a statement moves. What hangs on it: finding ids in prose risks false matches on anything shaped like namespace/slug, so a checked reference probably needs its own syntax.
