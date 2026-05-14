Goal: закрыть production-блокеры из roadmap 2026-05-14, чтобы релизный путь стал безопасным, проверяемым и совместимым с SpecKeep.
Goal: фокус на TLS/mTLS, secrets hygiene, verify governance и финальных operational proofs без расширения scope до новых продуктовых фич.

| AC | Summary |
| --- | --- |
| AC-001 | Клиент принимает только доверенный серверный сертификат |
| AC-002 | Сервер реально проверяет client cert в production mTLS |
| AC-003 | Примеры и tracked files не содержат приватных ключей |
| AC-004 | SpecKeep verify path проходит release readiness |
| AC-005 | Smoke, access model и quality gates зафиксированы в verify |

Out of Scope:
- IPv6, performance-polish и новые transport/routing features
- Расширение admin API сверх минимального release hardening
- Полная release-документация, если она не нужна для закрытия blockers
- Нерелевантный пересмотр старых roadmap-этапов
