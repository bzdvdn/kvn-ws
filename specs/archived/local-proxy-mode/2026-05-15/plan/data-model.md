---
status: no-change
reason: Proxy-режим добавляет только новый тип фрейма (FrameTypeProxy=0x05) и wire-формат streamID+dst внутри payload. Core domain entities (Session, IPPool, Route, ACL) не меняются.
---
# Data Model: local-proxy-mode

**Status:** no-change

Wire-формат FrameTypeProxy: `[streamID:4][len:2][dst_len:2][dst][data]` — не data model, а transport protocol.
