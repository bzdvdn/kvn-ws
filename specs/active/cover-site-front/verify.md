---
report_type: verify
slug: cover-site-front
status: pass
docs_language: ru
generated_at: 2026-11-02
---

# Verify Report: cover-site-front

## Scope

- snapshot: Go-сервер отдаёт встроенный cover-site и v2 на одном листенере; TLS/ALPN/404/login-форма/маршрутизация; установщик экспортирует сайт; dpi-check расширен.
- verification_mode: default
- artifacts:
  - CONSTITUTION.md
  - specs/active/cover-site-front/spec.md
  - specs/active/cover-site-front/plan.md
  - specs/active/cover-site-front/tasks.md
- inspected_surfaces:
  - src/internal/server/cover.go
  - src/internal/server/cover_assets.go (+ cover_assets/)
  - src/internal/server/fallback.go
  - src/internal/transport/server.go
  - src/internal/transport/tls/tls.go
  - src/cmd/server/main.go
  - src/internal/transport/client.go, src/internal/client/client.go
  - src/android/.../MainViewModel.kt
  - scripts/install-server.sh, scripts/dpi-check.sh

## Verdict

- status: pass
- archive_readiness: safe
- summary: все 6 AC подтверждены unit/integration-тестами и локальным `dpi-check` (PASS); Go/Android проверки зелёные. Полевые LE/DNS/active-probe и on-device проверки остаются за рамками evidence.

## Checks

- task_state: completed=13, open=0; `verify-task-state.sh cover-site-front` — OK.
- trace: `trace --tests` нашёл 4 test-proof; изменённые файлы размечены.
- acceptance_evidence:
  - AC-001 -> `TestCoverSiteServesFiles`, `TestCoverSiteRangeAndNotFound`, `TestFallbackEmbeddedSite`; `--export-cover-site` → 15 файлов; dpi-check: `/`, `/robots.txt`, `/favicon.ico`, `/login.html`, 404.
  - AC-002 -> `TestRoutingHTTP1ServesCover`, `TestRoutingInvalidTokenServesCover`, `TestRoutingValidTokenServesV2`; dpi-check active-probe → 404 text/html, без `octet-stream`/`pad=`.
  - AC-003 -> `TestNewPublicServerTLSConfig` (TLS1.2, ALPN h2+h1, SNI); dpi-check: ALPN http/1.1 и TLS 1.3 на лабораторном self-signed стенде (`DPI_INSECURE=1`).
  - AC-004 -> client ALPN `h2,http/1.1`, server_name pass-through (MainViewModel → `tls.server_name`), verify по умолчанию; `TestClientWithHTTP2OnlyALPNConnects` + существующие TLS-тесты.
  - AC-005 -> `scripts/dpi-check.sh` PASS: h1/h2, 404/catch-all, favicon, issuer, login form, invalid token.
  - AC-006 -> `TestClientWithHTTP2OnlyALPNConnects`, существующий suite, fallback без новых полей (`TestFallbackEmbeddedSite`).
- implementation_alignment:
  - v2 только h2 + валидный Bearer; h1/неизвестное/невалидное → cover-site/404.
  - Встроенный сайт в `embed.FS`, экспорт и `fallback.dir` override; установщик экспортирует в `/var/lib/kvn/cover`.
  - TLS 1.2+1.3, ALPN h2+h1, optional `tls.sni_certs`; LE через `--acme-email` или `--tls-cert/--tls-key`.

## Errors

- none

## Warnings

- Лабораторный dpi-check использует self-signed серт и `DPI_INSECURE=1`; production должен использовать LE/CA-серт.
- `--acme-email` (certbot) и systemd-установка не запускались в этом окружении; проверен `bash -n` и экспорт сайта.

## Questions

- none

## Not Verified

- Реальный домен, DNS, Let's Encrypt и полевой active-probe в целевой сети не проверялись.
- Android on-device: cover-site/ALPN/`server_name` и установка APK не проверялись (только `:app:testDebugUnitTest`).
- Отрицательные фикстуры dpi-check (catch-all/h2-only/self-signed) не автоматизированы отдельным тестом; скрипт содержит проверки и падает по логике.

## Next Step

- safe to archive
