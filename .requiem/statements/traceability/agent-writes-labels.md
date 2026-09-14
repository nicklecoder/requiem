---
id: agent-writes-labels
namespace: traceability
kind: design
modality: must_not
abstract: true
status: active
provenance:
    type: dialogue
created_at: 2026-09-14T04:14:32.180911828Z
relationships:
    - to: traceability/comment-syntax
      type: supersedes
      note: requiem no longer writes labels, so it no longer needs to know any file's comment syntax
---

Requiem must not insert label comments into source files. Comment syntax can only be guessed from a file extension, and the extension is not enough: a .m file is MATLAB or Objective-C depending on its contents, and a wrong guess writes invalid syntax into a file requiem does not own. Whoever edits the file already knows its language, so the label is their comment to write. This also keeps requiem out of the unbounded job of tracking comment syntax for every language it might meet.
