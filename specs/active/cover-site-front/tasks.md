# Cover-site front (Go-only) — Задачи

## Phase Contract

Inputs: `spec.md`, `plan.md`.
Outputs: упорядоченные задачи с `Touches:` и покрытием AC.
Stop if: задача требует внешнего reverse proxy/ECH/relay.

## Surface Map

| Surface | Tasks |
|---------|-------|
| src/internal/config/server.go | T1.1, T3.1 |
| src/internal/server/cover.go | T1.1 |
| src/internal/server/cover_assets.go | T1.1, T1.3 |
| src/internal/server/cover_assets/ | T1.3 |
| scripts/install-server.sh | T1.4 |
| configs/server.example.yaml | T1.4 |
| deploy/cover-site/README.md | T1.4 |
| src/internal/server/fallback.go | T1.1 |
| src/internal/server/cover_test.go | T1.2 |
| deploy/cover-site/ | T1.3 |
| src/internal/transport/server.go | T2.1 |
| src/internal/transport/transport_test.go | T2.2, T6.1 |
| src/cmd/server/main.go | T3.1 |
| src/internal/transport/tls/tls.go | T3.1, T3.2 |
| src/internal/transport/tls/tls_test.go | T3.2 |
| src/internal/transport/client.go | T4.1 |
| src/internal/client/client.go | T4.1 |
| src/android/app/src/main/kotlin/com/kvn/client/ui/MainViewModel.kt | T4.2 |
| src/android/app/src/main/kotlin/com/kvn/client/config/AppConfig.kt | T4.2 |
| src/android/app/src/test/java/com/kvn/client/config/AppConfigTest.kt | T4.2 |
| scripts/dpi-check.sh | T5.1 |
| docs/ru/v2.md, docs/en/v2.md | T5.2 |

## Implementation Context

- Цель MVP: cover-site + 404/MIME/Range + v2/h1/h2 routing + TLS 1.2/1.3 ALPN.
- Инварианты:
  - v2 — только `r.ProtoMajor == 2` и валидный Bearer; всё остальное → cover-site/404.
  - Проба не видит `octet-stream`, `X-KVN`, `pad=` и catch-all 200.
  - Нет `fallback.dir`/upstream → прежнее поведение (совместимость).
  - Серт/ключ читаются из файлов; self-signed не требуется, но не запрещён.
- Контракты: `fallback.dir`, `fallback.not_found`, `fallback.server_header`; `tls.sni_certs` (optional); client `tls.server_name`.
- Proof signals: unit-тесты handler/routing/TLS; `dpi-check.sh`; curl/openssl.
- Вне scope: nginx/Envoy, ECH, Reality, relay, CDN, traffic shaping, Android UI-поле.

## Фаза 1: Cover-site (MVP)

Цель: сервер отдаёт настоящий сайт и 404 вместо заглушки.

- [x] T1.1 Добавить `fallback.dir/not_found/server_header` и `NewCoverSite`: статика через `ServeContent`, MIME (`.m3u8`, `.ts`, `.m4s`, `.svg`, `.woff2`), Range, security headers, 404, без directory listing. Outcome: `/`, `/robots.txt`, `/favicon.ico`, `/media/*` отдаются корректно; неизвестный путь — 404. Touches: src/internal/config/server.go, src/internal/server/cover.go, src/internal/server/fallback.go
      Proof: code src/internal/server/cover.go newCoverHandler
- [x] T1.2 Unit-тесты cover handler: MIME/Range/404/headers/no-listing + fallback без dir. Outcome: поведение зафиксировано тестами. Touches: src/internal/server/cover_test.go
      Proof: test src/internal/server/cover_test.go TestCoverSiteServesFiles
- [x] T1.3 Встроить реалистичный cover-site (лендинг, login-форма, watch/видео, 404, about/terms/privacy, стили, favicon, sitemap) через `embed.FS`; `fallback.dir` переопределяет. Outcome: свежий сервер без настройки отдаёт валидный стриминговый сайт. Touches: src/internal/server/cover_assets.go, src/internal/server/cover_assets/, src/internal/server/cover.go
      Proof: code src/internal/server/cover_assets.go EmbeddedCoverFS
- [x] T1.4 Экспорт встроенного сайта (`--export-cover-site`) и интеграция в `install-server.sh`: экспорт в `/var/lib/kvn/cover`, `fallback.dir` в конфиге, `--domain/--tls-cert/--tls-key/--acme-email`. Outcome: установщик сразу поднимает сайт. Touches: src/cmd/server/main.go, scripts/install-server.sh, configs/server.example.yaml, deploy/cover-site/README.md
      Proof: code scripts/install-server.sh export-cover-site

## Фаза 2: Routing v2 vs site

Цель: проба не отличает VPN-эндпоинт от сайта.

- [x] T2.1 Роутить в v2 только h2 + валидный Bearer; для h1, невалидного токена и неизвестных путей — cover-site/404; сохранить legacy `/api/v2/stream`. Outcome: invalid token не даёт `octet-stream`/KVN-заголовков. Touches: src/internal/transport/server.go
      Proof: code src/internal/transport/server.go ServeHTTP
- [x] T2.2 Тесты routing: valid/invalid token, h2/h1, unknown path, отсутствие `pad=`/`X-KVN`/`octet-stream` на пробе. Outcome: регрессии ловятся. Touches: src/internal/transport/transport_test.go
      Proof: test src/internal/transport/routing_test.go TestRoutingInvalidTokenServesCover

## Фаза 3: TLS/ALPN/certs

Цель: TLS-поверхность как у обычного сайта.

- [x] T3.1 Сервер: TLS 1.2+1.3, ALPN `h2`,`http/1.1`, cert/key из файлов, optional `tls.sni_certs` через `GetCertificate`. Outcome: h1 и h2 проходят handshake, серт валиден для домена. Touches: src/cmd/server/main.go, src/internal/transport/tls/tls.go, src/internal/config/server.go
      Proof: code src/internal/transport/tls/tls.go NewPublicServerTLSConfig
- [x] T3.2 Тесты TLS: MinVersion/ALPN/SNI-выбор серта. Outcome: конфиг проверен тестами. Touches: src/internal/transport/tls/tls_test.go
      Proof: test src/internal/transport/tls/tls_test.go TestNewPublicServerTLSConfig

## Фаза 4: Клиент

Цель: клиент проверяет прод-серт и позволяет отделить SNI от endpoint.

- [x] T4.1 Клиент: ALPN `h2`,`http/1.1`; server_name из конфига; verify по умолчанию; не ломать существующие конфиги. Outcome: h2-подключение с проверкой серта и заданным SNI. Touches: src/internal/transport/client.go, src/internal/client/client.go
      Proof: code src/internal/transport/client.go NewClientWithOptions
- [x] T4.2 Android: optional `serverName` в `ConnectionConfig` и проброс в `tls.server_name`; обратная совместимость DataStore/QR. Outcome: конфиг без поля работает как раньше. Touches: src/android/app/src/main/kotlin/com/kvn/client/config/AppConfig.kt, src/android/app/src/main/kotlin/com/kvn/client/ui/MainViewModel.kt, src/android/app/src/test/java/com/kvn/client/config/AppConfigTest.kt
      Proof: code src/android/app/src/main/kotlin/com/kvn/client/ui/MainViewModel.kt v2ConfigJson

## Фаза 5: Диагностика и доки

- [x] T5.1 Расширить `dpi-check.sh`: h1/h2, случайные пути/404, favicon content-type, ALPN, issuer, invalid token, отсутствие KVN/`pad=`. Outcome: гейт ловит catch-all/h2-only/self-signed/маркеры. Touches: scripts/dpi-check.sh
      Proof: code scripts/dpi-check.sh probe_path
- [x] T5.2 Документация: домен/DNS/LE/certbot reload, cover-site, роутинг, `fallback.dir`, проверка. Outcome: администратор разворачивает без nginx. Touches: docs/ru/v2.md, docs/en/v2.md
      Proof: docs docs/ru/v2.md

## Фаза 6: Совместимость

- [x] T6.1 Тесты old-client↔new-server и new-client↔old-server, конфиг без новых полей. Outcome: v2 не ломается. Touches: src/internal/transport/transport_test.go
      Proof: test src/internal/transport/routing_test.go TestClientWithHTTP2OnlyALPNConnects

## Покрытие критериев приемки

- AC-001 -> T1.1, T1.2, T1.3, T1.4
- AC-002 -> T2.1, T2.2
- AC-003 -> T3.1, T3.2
- AC-004 -> T4.1, T4.2
- AC-005 -> T5.1, T5.2
- AC-006 -> T6.1

## Заметки

- Порядок: фаза 1-2 дают MVP; 3-4 — TLS/клиент; 5 — гейт; 6 — совместимость.
- Go: `go test ./src/internal/server ./src/internal/transport ./src/internal/config`; Android: `:app:testDebugUnitTest`.
- `dpi-check.sh` запускать на стенде с реальным доменом/сертом.
