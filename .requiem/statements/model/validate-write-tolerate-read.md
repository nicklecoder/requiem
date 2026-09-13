---
id: validate-write-tolerate-read
namespace: model
kind: rule
modality: must
status: active
provenance:
    type: dialogue
created_at: 2026-09-13T21:24:38.759420387Z
---

Closed enums are validated on write and tolerated on read: an unrecognized value reads as unset rather than failing the file. Without that asymmetry, adding a member later would make every older binary reject files a newer one wrote, which is the usual reason people avoid closing an enum at all.
