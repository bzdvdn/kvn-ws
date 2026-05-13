# Foundation — Summary

**Goal:** Go-модуль, скелет DDD-структуры, Docker, CI, конфигурация (viper) и логирование (zap) — foundation для всех последующих фич kvn-ws.

| AC | Описание | Критерий |
|----|----------|----------|
| AC-001 | Go-модуль и сборка | `go build ./src/...` exit 0 |
| AC-002 | Скелет internal-пакетов | все директории существуют |
| AC-003 | Docker образ | `docker build` успешен |
| AC-004 | docker-compose | оба сервиса в статусе Up |
| AC-005 | CI pipeline | test/lint/build проходят на PR |
| AC-006 | client.yaml парсинг | лог `config loaded` |
| AC-007 | server.yaml парсинг | лог `config loaded` |
| AC-008 | JSON-логи | stdout содержит `{"level":"info",...}` |
| AC-009 | Env override | `KVN_CLIENT_*` перезаписывает YAML |
| AC-010 | Graceful shutdown | SIGTERM → shutdown-лог → exit 0 |
| AC-011 | Бинарники в bin/ | `scripts/build.sh` → `bin/client`, `bin/server` |
| AC-012 | .gitignore | `git status` не показывает `bin/` |

**Out of scope:** любая логика TUN, WebSocket, туннелирования, routing, auth, тесты.
**Open questions:** none.

**Inspect verdict:** pass (0 errors, 0 warnings).
**Next step:** `/speckeep.plan foundation`
