---
id: required-namespace-forces-scope-thinking
rejected_at: 2026-09-16T06:13:49.391912154Z
see_instead: retrieval/check-scope-defaults-to-corpus
---

Keep --namespace required on check and merely document the empty-string escape hatch, on the grounds that forcing the agent to name a scope makes it think about where a decision would live. Rejected: the thinking it forces is a guess, and a wrong guess returns an empty result that reads as 'nothing here states this' — a false all-clear produced by scope rather than by the corpus. Documenting --namespace "" also teaches an argument spelling that exists only as an artifact of the validation, where omitting the flag is the same thing agents already do on audit and list.
