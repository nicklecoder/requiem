---
id: statement-length-cap
rejected_at: 2026-09-29T00:14:15.933736506Z
see_instead: model/one-decision-per-statement
---

Cap a statement's body at a number of characters or words, configured per project, so a statement cannot grow without bound into several decisions. Rejected: length is the wrong measure. In 48 statements sampled across sbs and requiem, 11 would be better split, and length separated them from single decisions barely better than chance (AUC 0.56): the longest statements were mostly long because they give their reasoning, which requiem asks for, while one of 194 characters held three rules. A cap would punish the well-argued statement and miss the short compound one, and the right number differs by project. Requirements-engineering practice agrees: INCOSE's guide asks for one thought per requirement, flagged by conjunctions, not for a length limit. The splitting test and the growth question address the actual problem.
