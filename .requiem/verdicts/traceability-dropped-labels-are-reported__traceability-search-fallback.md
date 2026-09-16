---
a: traceability/dropped-labels-are-reported
b: traceability/search-fallback
verdict: not_related
decided_at: 2026-09-16T23:15:32.210589909Z
---

Worth stating rather than waving through, because it is the pair that looks closest to a conflict. search-fallback keeps labels a shortcut rather than a precondition, and dropped-labels never asks for a label that did not exist — with no labels there is nothing to drop and it never fires. The tool is exactly as useful at zero coverage as it was.
