---
status: no-change
reason: Оптимизации производительности не требуют изменения data model. sync.Pool, TCP_NODELAY, batch writes, compression, multiplex, MTU/PMTU — всё на уровне транспорта и конфига. Handshake получает опциональное поле MTU (uint16, default 1500), что является wire-форматом, а не data model.
---
# Data Model: performance-and-polish

**Status:** no-change

Core domain entities (Session, IPPool, Route, ACL) не меняются. Все изменения в infrastructure/transport слое.
