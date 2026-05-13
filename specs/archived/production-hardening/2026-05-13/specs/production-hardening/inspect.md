---
report_type: inspect
slug: production-hardening
status: pass
docs_language: ru
generated_at: 2026-05-13
---

# Inspect Report: production-hardening

## Scope

- snapshot: проверка spec production-hardening — reconnect, keepalive, kill-switch, rate limiting, session expiry, BoltDB, Prometheus, health, SIGHUP, audit, CLI flags, stability gate
- artifacts:
  - CONSTITUTION.md
  - specs/active/production-hardening/spec.md

## Verdict

- status: pass

## Errors

- none

## Warnings

- none

## Questions

- AC-011: Описано «env-переменная переопределяет CLI flag». Стандартный precedence (pflag > viper): CLI > env > YAML. Рекомендуется явно указать в spec или зафиксировать в plan.

## Suggestions

- AC-007: `session_id` label на `kvn_errors_total` — высокое cardinality. Рекомендуется снять session_id с total counter, оставить только type + reason.
- AC-003 kill-switch: механизм реализации (nftables reject vs TUN level) лучше зафиксировать на plan-фазе.
- AC-004: уточнить, rate limiter на HTTP upgrade (до WebSocket handshake) или на уровне протокола — оба сценария разные.

## Traceability

- 12 AC покрывают все 11 задач Sprint 3 (3.1–3.11)
- AC-012 является meta-стабильность gate

## Next Step

- safe to continue to plan
