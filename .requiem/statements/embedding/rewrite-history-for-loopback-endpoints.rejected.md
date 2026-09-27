---
id: rewrite-history-for-loopback-endpoints
rejected_at: 2026-09-26T07:31:16.741210666Z
see_instead: embedding/local-endpoint-overlay
---

Rewrite git history with git filter-repo to remove the embedding endpoints config.yaml committed before the local overlay existed. Rejected: the values were loopback only — localhost:11434, Ollama's default, and a 127.0.0.1 test endpoint — which reveal nothing about anyone's network. The rewrite also cannot finish the job: GitHub keeps each PR's refs/pull/N/head pointing at the original commits, and only GitHub Support can purge them. Its cost is real: force-pushing a protected public main, new SHAs from the first config onward, and a fetch-and-reset on every clone, where one plain pull merges the old history back. The overlay stops machine-specific values reaching git from now on, which is the part that matters. A committed secret is different: rotate it first, then rewrite and ask GitHub Support to purge.
