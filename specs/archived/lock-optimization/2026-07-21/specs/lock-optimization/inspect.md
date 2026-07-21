---
report_type: inspect
slug: lock-optimization
status: pass
docs_language: ru
generated_at: 2026-07-21
---

# Inspect Report: lock-optimization

## Scope

- snapshot: проверка spec на замену sync.Mutex → sync/atomic + sync.RWMutex для простых полей и read-heavy структур
- artifacts:
  - CONSTITUTION.md
  - specs/active/lock-optimization/spec.md

## Verdict

- status: pass

## Errors

- none

## Warnings

- none

## Questions

- none

## Suggestions

- **S1**: В AC-006 стоит явно упомянуть, что forward() читает конфиг-поля в два приёма (stream/routeDirect/origResolves и затем upstreams в fallback-пути). Оба захвата должны быть RLock. Сейчас это недвусмысленно, но можно уточнить. Не блокер.

## Traceability

- AC-001 ↔ RQ-001: Collector txBytes/rxBytes/reconnects → atomic
- AC-002 ↔ RQ-002: Collector latencyMs → atomic
- AC-003 ↔ RQ-003: Server.nextID → atomic.AddUint32
- AC-004 ↔ RQ-004: Relay.upstreamConn → atomic.Bool
- AC-005 ↔ RQ-005: upstreamSession.closed → atomic.Bool
- AC-006 ↔ RQ-006: Server.mu → RWMutex
- AC-007 ↔ RQ-007: SessionStreams.Load → RLock
- AC-008 ↔ RQ-008: Manager.Get → RLock
- RQ-009/RQ-010 сквозные: race + green tests — покрыты всеми AC

Все AC имеют observable proof (`go test -race ./...`). Покрытие полное.

## Next Step

- safe to continue to plan
