---
id: shell-command-template
rejected_at: 2026-09-13T21:25:03.874201355Z
see_instead: embedding/configured-endpoint
---

Store a shell command template in config and shell out per statement to obtain a vector. Rejected: maximum provider flexibility, but fragile quoting around the text placeholder, a shell-injection surface, platform-dependent behaviour on Windows, and harder to test than an HTTP call.
