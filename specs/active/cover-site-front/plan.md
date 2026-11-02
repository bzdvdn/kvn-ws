# Cover-site front (Go-only) План

## Phase Contract

Inputs: `spec.md`, конституция, минимальный repo-контекст.
Outputs: plan без изменения intent спеки.
Stop if: spec требует внешнего reverse proxy/ECH/relay (вне scope).

## Цель

Оставить один Go-бинарник, который на одном TLS-листенере отдаёт реалистичный статический сайт и v2-трафик. Работа живёт в `src/internal/server` (cover-site handler), `src/internal/transport` (routing v2 vs site), `src/cmd/server` (TLS/ALPN/certs), `src/internal/config` (поля fallback/tls), клиентском TLS-конфиге и `scripts/dpi-check.sh`.

## MVP Slice

- Встроенный cover-site (лендинг/login/404/видео) + `fallback.dir` override, v2-роутинг по валидному токену и h2, ALPN `h2`+`http/1.1`, TLS 1.2+1.3, cert/key из файлов, экспорт сайта установщиком.
- Закрывает AC-001, AC-002, AC-003; клиентская часть AC-004 — минимальная (ALPN + server_name pass-through).

## First Validation Path

`./scripts/dpi-check.sh` на стенде с cover-site + `curl --http1.1/--http2`, `openssl s_client`, Range-запрос к видео, invalid-token проба. Плюс `go test ./src/internal/server ./src/internal/transport`.

## Scope

- Статический cover-site и 404 на том же листенере.
- Роутинг v2/сайт и защита от h1-пробы на v2-пути.
- TLS/ALPN/серты из файлов, опциональные SNI-серты.
- Клиентский ALPN/verify/server_name.
- Расширение `dpi-check.sh` и документации по развёртыванию.

Граница, остающаяся нетронутой: формат v2-протокола (target/`:authority`, padding), серверный QUIC, Android UI сверх optional `server_name`.

## Performance Budget

- Cover-site: без регрессии v2 throughput; статика через `ServeContent` (Range).
- v2 routing добавляет один map-lookup токена (уже есть).
- `none` для строгих latency-бюджетов.

## Implementation Surfaces

- `src/internal/server/cover.go`, `cover_assets.go`, `cover_assets/` (new) — встроенный статический handler: MIME, 404, security headers, без directory listing; экспорт ассетов.
- `src/internal/server/fallback.go` — оставить upstream/минимальный fallback, добавить выбор `dir`.
- `src/internal/config/server.go` — `fallback.dir`, `fallback.not_found`, `tls.sni_certs` (optional).
- `src/internal/transport/server.go` — v2 только для h2 + валидного токена; иначе fallback/site.
- `src/cmd/server/main.go` — TLS 1.2+1.3, ALPN `h2`,`http/1.1`, `GetCertificate` для нескольких SNI.
- `src/internal/client/client.go`, `src/internal/transport/client.go` — ALPN `h2`,`http/1.1`.
- `src/internal/client/...`/Android JSON — optional `tls.server_name` pass-through.
- `scripts/dpi-check.sh`, `docs/ru|en/v2.md` — расширенный чек и инструкция.
- `scripts/install-server.sh` — экспорт встроенного сайта, `fallback.dir`, TLS/ACME опции.
- `deploy/cover-site/` — README по кастомизации.

## Bootstrapping Surfaces

- Новый пакет handler-а cover-site и поля конфига; существующий `transport.Server.fallback` уже даёт точку подключения.
- `deploy/cover-site/` с примером index/404/favicon/robots.

## Влияние на архитектуру

- Сервер остаётся одним процессом и листенером; отдельный поддомен/порт не вводится.
- Роутинг v2/сайт — в существующем `ServeHTTP`, без изменения протокола.
- Совместимость: старые клиенты (h2, обычный SNI/cert) и конфиги без новых полей работают; `fallback.dir` опционален.

## Acceptance Approach

- AC-001 -> `server/cover.go` + `config`; проверка `curl -I`, Range, 404, MIME; unit-тесты handler-а.
- AC-002 -> `transport.Server.ServeHTTP` (только h2 + valid token → v2; иначе site/404); `dpi-check` invalid-token; тест, что invalid token не даёт `octet-stream`/KVN-заголовков.
- AC-003 -> `cmd/server` TLS/ALPN/`GetCertificate`; `openssl s_client` (1.2/1.3), `curl --http1.1/--http2`, issuer.
- AC-004 -> client TLS config + Android JSON pass-through; unit-тест `NewClientTLSConfigFromSettings`/client options + integration h2 connect.
- AC-005 -> `dpi-check.sh`; прогон на стенде + негативные проверки (catch-all/h2-only/self-signed/KVN).
- AC-006 -> интеграционные тесты old/new combinations + существующий suite.

## Данные и контракты

- Data model: no change.
- Server config: аддитивные optional-поля (`fallback.dir`, `fallback.not_found`, `tls.sni_certs`).
- Client config: optional `tls.server_name` (уже есть в Go config), проброс из Android JSON; без новых обязательных полей.
- v2 protocol: без изменений; `:authority`/padding остаются как есть.

## Стратегия реализации

- DEC-001 Go-only, без reverse proxy
  Why: пользовательское решение; сохраняет full-duplex h2 без лишнего хопа.
  Tradeoff: серверный TLS/H2-фингерпринт остаётся Go-like; при необходимости прокси добавим позже.
  Affects: `cmd/server`, `internal/server`, `internal/transport`.
  Validation: `dpi-check` + полевой замер.
- DEC-002 Cover-site как `fallback` handler
  Why: существующий `transport.Server` уже маршрутизирует неизвестное/невалидное в fallback; минимум изменений.
  Tradeoff: нужно аккуратно не отдать v2-путь по h1; решается проверкой `ProtoMajor`.
  Affects: `transport/server.go`, `server/fallback.go`, `server/cover.go`.
  Validation: routing unit tests + `dpi-check`.
- DEC-003 Встроенная статика через `embed.FS` + `http.FileServer`
  Why: сайт работает «из коробки», готовые Range/MIME; `fallback.dir` остаётся override.
  Tradeoff: ассеты входят в бинарник; для кастомизации нужен `--export-cover-site`.
  Affects: `server/cover.go`, `deploy/cover-site`.
  Validation: unit tests + `curl -I`/Range.
- DEC-004 TLS 1.2+1.3, ALPN h2+http/1.1, cert/key из файлов
  Why: TLS 1.3-only/h2-only — отпечаток; реальные сайты поддерживают 1.2/h1.
  Tradeoff: v2 остаётся на h2/1.3; h1-запросы на v2-путь отдаём сайту.
  Affects: `cmd/server`, `transport/client.go`, `client/client.go`.
  Validation: `openssl`/`curl`/integration.
- DEC-005 Клиент: server_name pass-through, verify по умолчанию
  Why: endpoint и SNI разделены для будущих decoy-доменов; прод должен проверять серт.
  Tradeoff: Android UI-поле не добавляем в этой фиче (QR/config); Go config уже поддерживает.
  Affects: Android JSON, `client/connection`, `tlsprofile`.
  Validation: unit + integration.
- DEC-006 `pad=` не трогаем в этой фиче
  Why: маркер находится внутри авторизованного TLS-потока, не на пробной поверхности; passive DPI его не видит.
  Tradeoff: виден TLS-терминирующему промежуточному узлу; это вне scope (нет reverse proxy).
  Affects: `transport/client.go` (без изменений).
  Validation: `dpi-check` проверяет пробную поверхность, не авторизованный поток.

## Incremental Delivery

### MVP

- Cover-site handler + config, v2 routing, TLS/ALPN/certs, dpi-check. AC-001..003, 005.
- Критерий: стенд проходит `dpi-check`, v2 round-trip работает.

### Итеративное расширение

- Client server_name/ALPN + Android JSON (AC-004).
- Опциональные SNI-серты и docs (AC-003).
- Compatibility tests (AC-006).

## Порядок реализации

1. `server/cover.go` + fallback config (AC-001).
2. Routing h2/valid token (AC-002).
3. TLS/ALPN/GetCertificate (AC-003).
4. Client ALPN/server_name (AC-004).
5. `dpi-check` расширение (AC-005).
6. Compatibility tests (AC-006).

## Риски

- Статика ломает v2-запросы: mitigation — routing только по валидному токену и h2; тесты.
- HTTP/1.1 ALPN и `http2.Transport`: mitigation — сервер предпочитает h2 при его наличии; тест h1/h2.
- Directory traversal/listing: mitigation — `http.Dir`, отключить listing, 404.
- TLS 1.2 меняет поведение: mitigation — v2-клиент всё равно 1.3; тест.
- LE-серт и reload: mitigation — документировать certbot deploy-hook; `GetCertificate` из файлов.

## Rollout и compatibility

- Все новые поля optional; без `fallback.dir` — старое поведение (минимальный fallback/upstream).
- Старые клиенты продолжают использовать SNI/cert; сервер не требует новых полей.
- Операционно: домен/DNS/LE/certbot — вне сервера; после выката — `dpi-check` и полевой тест.

## Проверка

- Automated: unit-тесты cover handler (MIME/404/Range/headers), routing (valid/invalid token, h2/h1), TLS config (1.2/1.3/ALPN), client TLS (server_name/verify), compatibility.
- Manual: `dpi-check.sh`, `openssl s_client`, `curl --http1.1/--http2`, Range, invalid token.
- AC/DEC: покрыть AC-001..006 и DEC-001..006.

## Соответствие конституции

- нет конфликтов: v2-only, TLS 1.3/H2 сохраняются; fallback-сайт для неизвестных запросов остаётся; секреты/серты не коммитятся.
