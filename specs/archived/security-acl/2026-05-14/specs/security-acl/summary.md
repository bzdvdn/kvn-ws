# Security & ACL — Summary

## Goal
Добавить на сервер KVNOS многоуровневую защиту: CIDR-фильтрацию, per-token bandwidth/session лимиты, Origin/Referer validation, Admin API и опциональный mTLS. Закрыть OWASP top-10 для WebSocket.

## Acceptance Criteria

| AC | Описание | Priority |
|---|---|---|
| AC-001 | CIDR-фильтрация — deny-список | P1 |
| AC-002 | CIDR-фильтрация — allow-список | P1 |
| AC-003 | Per-token bandwidth quota | P1 |
| AC-004 | Session limit per token (max_sessions) | P1 |
| AC-005 | Origin validation — валидный Origin | P1 |
| AC-006 | Origin validation — невалидный Origin | P1 |
| AC-007 | Admin API — список сессий (GET) | P1 |
| AC-008 | Admin API — принудительный disconnect (DELETE) | P1 |
| AC-009 | mTLS — успешная аутентификация | P2 (optional) |
| AC-010 | mTLS — отклонение без сертификата | P2 (optional) |
| AC-011 | Admin API — 401 без токена | P1 |

## Out of Scope
- Ротирование токенов через API
- История сессий
- WAF/DPI
- OAuth2/JWT/LDAP

## Artifacts
- `specs/active/security-acl/spec.md`
- `specs/active/security-acl/spec.digest.md`
- `specs/active/security-acl/summary.md`

## Inspect Status
- status: concerns (3 warnings, 0 errors)
- W-01: digest AC numbering mismatch
- W-02: token config requires migration from []string to structured per-token config
- W-03: CIDR "before TLS" vs current tls.Listen architecture — needs decision
- W-04: CheckOrigin requires upgrader refactoring
- W-05: chi router adds new dependency

## Blockers
- none — все зависимости решены

## Verify Status
- status: **pass** ✅
- archive_readiness: safe
- all 12 tasks completed, all 11 AC covered
- W-02, W-03 требуют документирования решений на plan
