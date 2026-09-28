---
id: one-decision-per-statement
namespace: model
kind: rule
modality: should
status: active
provenance:
    type: dialogue
created_at: 2026-09-28T17:46:37.522737686Z
---

A statement records one decision. The test for splitting is whether part of it could be rejected or superseded while the rest stands; if so it is two statements, the narrower one linked to the other with refines. The reasoning behind a decision is part of it, however long, and so is a list of the things it covers. Why: the statement is requiem's unit for everything it does (retrieval, superseding, rejecting, audit pairs, code labels), so a statement holding several decisions cannot have one retired, flagged or implemented apart from the rest. In 48 statements sampled across sbs and requiem, 11 would be better split, at every length: one of 194 characters held three rules while one of 1,259 was a single decision well argued. Length separated them barely better than chance (AUC 0.56), as did every other cheap signal tried, a small NLI model included (0.59-0.61), so the judgment is the author's, made at writing time, where nine of the eleven compound statements were born.
