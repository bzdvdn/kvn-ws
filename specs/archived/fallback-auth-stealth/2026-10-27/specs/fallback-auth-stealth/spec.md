# Stealth fallback и Bearer-auth до upgrade

## Scope Snapshot

- In scope: новый WS-path с проверкой `Authorization: Bearer` до upgrade, fallback-сайт/reverse-proxy для неавторизованных, legacy path для старых клиентов.
- Out of scope: relay, QUIC, изменение data-path/padding, смена TLS-сертификатов.

## Цель

Клиент подключается как к обычному HTTPS/WebSocket-сервису: валидный токен в стандартном заголовке открывает туннель, невалидный/пустой запрос получает настоящий fallback-сайт с тем же сертификатом. Старые клиенты продолжают работать через legacy path, а handshake не получает лишний RTT.

## Основной сценарий

1. Новый клиент открывает TLS к домену и делает WS upgrade на stealth path с `Authorization: Bearer <token>`.
2. Сервер проверяет токен **до** upgrade; валидный → `101` и обычный KVN handshake.
3. Без токена/с неверным токеном → fallback-страница или reverse-proxy на реальный сайт; никаких tunnel-ошибок и `X-KVN-*`.
4. Старый клиент идёт на legacy path и авторизуется через ClientHello, как раньше.
5. Если новый клиент не получил `101` на stealth path, он откатывается на legacy path.

## User Stories

- P1: как пользователь, я хочу, чтобы активный prober видел обычный сайт, а не VPN.
- P2: как пользователь со старым клиентом, я не должен потерять соединение после обновления сервера.
- P3: как оператор, я хочу скрыть health/admin endpoints от внешнего мира.
- P4: как клиент в TCP-only сети, я не хочу зависеть от UDP/QUIC.

## MVP Slice

- `AC-001` + `AC-002` + `AC-003`: stealth path с Bearer, fallback и legacy-совместимость.
- Fallback reverse proxy, hide-health, client fallback, kvn-web/Android — следующими задачами.

## First Deployable Outcome

- Новый сервер принимает Bearer на stealth path; без токена отдаёт fallback; старый клиент на legacy path работает.

## Scope

- Сервер: `stealth.path`, `fallback.upstream`, `fallback.title`, проверка Bearer до upgrade, fallback для неавторизованных.
- Сервер: скрытие health/admin/metrics от внешних запросов в stealth-режиме.
- Клиент: `stealth.enabled`, `stealth.path`, `stealth.legacy_path`; Bearer на upgrade; fallback на legacy path.
- kvn-web: настройки клиента для stealth path/fallback path.
- Android: те же поля/Bearer/fallback.
- Тесты: auth/fallback/legacy/fallback-proxy/no-RTT/secret-logs.

## Контекст

- Сейчас токен проверяется после upgrade через ClientHello; невалидный WS получает tunnel-ошибку — это fingerprint.
- `ws_paths` уже allowlist; legacy path остаётся без изменений.
- Новый stealth path — отдельная поверхность, feature включается конфигом; дефолт выключен.
- `Authorization` находится внутри TLS и пассивному DPI не виден.
- Fallback должен использовать тот же TLS-сертификат и не раскрывать tunnel-ошибки.

## Зависимости

- `src/internal/bootstrap/server` (mux, upgrade, auth, fallback), `src/internal/config` (server/client/WebUI), `src/internal/bootstrap/client` (dial/fallback), `src/internal/webui`, `src/android`.

## Требования

- RQ-001 Stealth path ДОЛЖЕН проверять стандартный `Authorization: Bearer` до WebSocket upgrade.
- RQ-002 Невалидный/отсутствующий токен ДОЛЖЕН получать fallback (200/сайт), а не 401/tunnel error.
- RQ-003 Legacy path и ClientHello-auth ДОЛЖНЫ работать без изменений; дефолт остаётся legacy.
- RQ-004 Fallback ДОЛЖЕН поддерживать статическую страницу и reverse-proxy upstream, использовать тот же сертификат, не логировать и не пересылать `Authorization` upstream.
- RQ-005 Новый handshake НЕ ДОЛЖЕН добавлять лишний RTT: Bearer идёт в том же upgrade-запросе.
- RQ-006 Health/admin/metrics НЕ ДОЛЖНЫ быть доступны внешним неавторизованным запросам в stealth-режиме.
- RQ-007 Клиент ДОЛЖЕН откатываться на legacy path, если stealth path не подтверждён.
- RQ-008 kvn-web и Android ДОЛЖНЫ сохранять/применять новые поля; токены не логируются.
- RQ-009 Все сценарии mixed-version ДОЛЖНЫ работать: old client↔new server, new client↔old server.

## Вне scope

- Relay bridge/terminator.
- QUIC/HTTP3.
- Изменение padding/очередей/data-path.
- CDN/domain fronting/ECH.
- Автоматическая ротация токенов.

## Критерии приемки

### AC-001 Bearer открывает туннель

- Почему важно: авторизация до upgrade — основа stealth.
- **Given** сервер с `stealth.path` и валидным токеном
- **When** клиент делает WS upgrade с `Authorization: Bearer <valid>`
- **Then** сервер отвечает `101`, handshake завершается, туннель работает
- Evidence: integration-тест с реальным WS.

### AC-002 Невалидный токен получает fallback

- Почему важно: prober не должен видеть tunnel-ошибку.
- **Given** stealth path и отсутствующий/неверный токен
- **When** приходит WS upgrade или обычный GET
- **Then** ответ — fallback-сайт/HTML 200, без `101`, `X-KVN-*` и tunnel-auth ошибки
- Evidence: httptest для обоих запросов.

### AC-003 Legacy path не сломан

- Почему важно: старые клиенты не должны пострадать.
- **Given** старый клиент без Bearer и legacy path
- **When** он подключается и проходит ClientHello-auth
- **Then** туннель работает как раньше; stealth-настройки не влияют
- Evidence: существующие integration/handler тесты + mixed-version.

### AC-004 Fallback static и reverse proxy

- Почему важно: fallback должен выглядеть как настоящий сайт.
- **Given** `fallback.title` или `fallback.upstream`
- **When** приходит неавторизованный запрос
- **Then** отдаётся титульная HTML или ответ upstream; `Authorization` не пересылается и не логируется; upstream недоступен → обычный 502 HTML
- Evidence: httptest с fake upstream и проверкой заголовков.

### AC-005 Нет лишнего RTT

- Почему важно: скрытность не должна ухудшать скорость.
- **Given** legacy и stealth handshake
- **When** выполняется серия подключений
- **Then** stealth handshake не делает дополнительный сетевой round-trip; latency в пределах шума
- Evidence: benchmark/integration замер Dial.

### AC-006 Клиент откатывается на legacy

- Почему важно: новый клиент должен работать со старым сервером.
- **Given** новый клиент с `stealth.enabled: true`, сервер без stealth path
- **When** stealth upgrade не даёт `101`
- **Then** клиент повторяет подключение на `legacy_path` без падения
- Evidence: mixed-version integration-тест.

### AC-007 Настройки и секреты

- Почему важно: фичу должно быть можно включить без ручного JSON.
- **Given** kvn-web/Android/CLI конфиг
- **When** заданы stealth/fallback поля
- **Then** они сохраняются, валидируются и применяются; токен не попадает в логи
- Evidence: config/UI roundtrip + лог-тест.

### AC-008 Health/admin скрыты

- Почему важно: `/healthz`/`/metrics` выдают инфраструктуру.
- **Given** stealth-режим
- **When** внешний запрос идёт на health/admin/metrics
- **Then** он получает fallback, а не JSON/pprof; loopback-доступ сохраняется
- Evidence: httptest с внешним/loopback RemoteAddr.

### AC-009 Android parity

- Почему важно: Android тоже должен уметь stealth path/Bearer/fallback.
- **Given** Android config с stealth-полями
- **When** выполняется подключение
- **Then** Bearer уходит в upgrade, fallback на legacy работает, настройки сохраняются
- Evidence: Android unit/integration-тест.

## Допущения

- Fallback-сайт/upstream реально существует и использует тот же домен/сертификат.
- Токен передаётся только по TLS; пассивный DPI его не видит.
- Legacy path остаётся доступным до миграции всех клиентов; затем оператор может его отключить.
- Новый stealth path опционален; дефолт — legacy behavior.

## Критерии успеха

- SC-001 Неавторизованный probe не получает tunnel-специфичных ответов.
- SC-002 Stealth handshake latency не хуже legacy более чем на 5%.
- SC-003 Mixed-version матрица проходит.

## Краевые случаи

- Пустой/битый Bearer, несколько заголовков.
- Fallback upstream недоступен/таймаут.
- Legacy path отключён, а старый клиент пришёл.
- TLS без SNI / неверный SNI.
- Токен в логах/ошибках — запрещён.
- Upstream fallback возвращает redirect/HTML/JSON — пробрасывается как есть.

## Открытые вопросы

- none
