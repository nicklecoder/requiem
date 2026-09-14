---
id: comment-syntax
namespace: traceability
kind: rule
modality: must
status: active
provenance:
    type: dialogue
created_at: 2026-09-14T03:23:45.54778512Z
relationships:
    - to: traceability/code-labels
      type: refines
      note: labelling must not damage the file it labels
---

A label must use comment syntax valid for the file it is written into, including block syntax where a language has no line comment. Guessing is not acceptable: the default would write invalid syntax into a stylesheet, visible text into Markdown, and a build error into a Makefile, all in files requiem does not own. An unrecognised type is refused rather than guessed.
