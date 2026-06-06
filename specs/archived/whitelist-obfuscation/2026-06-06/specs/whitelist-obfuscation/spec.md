# Whitelist & Obfuscation Hardening

## Scope Snapshot

- In scope: комплексное усиление защиты VPN-туннеля от DPI и whitelist-блокировок для WS и QUIC транспортов — как на клиенте, так и согласованные изменения на сервере.
- Out of scope: смена транспорта (HTTP/2, gRPC), деплой/инфраструктурные изменения, обфускация серверного ответа.

## Цель

Пользователь получает VPN, который устойчив к Deep Packet Inspection (DPI) и блокировкам по IP-белым спискам. Успех измеряется тем, что туннель остаётся рабочим в сети, где:
- TLS ClientHello от Go детектится (JA3) и блокируется
- Размеры пакетов анализируются для выявления туннелей
- SNI в TLS handshake фильтруется
- WebSocket endpoint по пути `/tunnel` заблокирован
- QUIC stream имеет сигнатурный 8-байтовый nonce plaintext в начале

## Основной сценарий

1. Клиент конфигурирует защиту: включает uTLS, кастомный WS path, padding, SNI.
2. После запуска клиент подключается через WS или QUIC к серверу:
   - TLS handshake использует uTLS с имитацией Chrome/Firefox вместо Go stdlib — JA3 отпечаток неотличим от браузерного.
   - SNI в handshake кастомизирован (напр. `www.google.com`), а не равен адресу сервера.
   - WebSocket endpoint идёт по кастомному пути (напр. `/api/v1/events`), не `/tunnel`.
   - QUIC stream не содержит сигнатурных plaintext-байт — весь payload (не только длина) обфусцирован.
3. Трафик в туннеле дополняется padding до кратного размера — DPI не может анализировать объём пересылаемых данных.
4. Результат: соединение выглядит как обычный браузерный HTTPS-трафик.

## User Stories

- P1 (MVP): WS transport с uTLS (Chrome fingerprint) + кастомный WS path + базовый padding. Работает даже если Go-шный ClientHello заблокирован.
- P2: QUIC transport — усиленная обфускация (XOR всего payload, убрать plaintext nonce) + кастомное SNI.
- P3: Продвинутый padding (рандомизация размера, межпакетные интервалы).

## MVP Slice

- P1: uTLS + кастомный WS path для WS транспорта.
- Закрывает: AC-001 (uTLS для WS), AC-003 (кастомный WS path).

## First Deployable Outcome

После первого pass:
- WS клиент подключается с uTLS (Chrome fingerprint)
- Сервер принимает подключение на кастомном WS path
- Можно проверить через curl/wireshark, что ClientHello не Go-шный

## Scope

- uTLS интеграция для WS транспорта (`gorilla/websocket` + `utls`) — только клиент
- Кастомный WebSocket endpoint path — клиент задаёт в URL сервера, сервер проверяет по allowlist-у (`server.ws_paths`)
- SNI per-connection в TLS конфиг — клиент, для WS и QUIC
- Усиление QUIC обфускации — клиент + сервер (`obfuscated.go`)
- Padding / нормализация размера сообщений — клиент (добавление) + сервер (отбрасывание)
- Конфигурационные поля для новых опций в `config.ClientConfig`, `config.ServerConfig` и Web UI

## Контекст

- Текущий стек: Go 1.25, gorilla/websocket, quic-go, zap.
- uTLS (`github.com/refraction-networking/utls`) совместим с `crypto/tls` на уровне интерфейса.
- quic-go использует `tls.Config` — uTLS.ClientSessionState несовместим, требуется тестирование.
- WS BatchWriter уже существует (`websocket.go:42-112`) — padding можно добавить в него.
- В конфиге уже есть `tls.verify_mode` и `tls.server_name` — `sni` будет отдельным полем.
- Go-шный ClientHello уникален: cipher suites order, extensions order. Это известный сигнатурный вектор (JA3/JA3S).
- Сервер сейчас слушает фиксированный path `/tunnel` в `server.go` — сервер получит allowlist `server.ws_paths: ["/tunnel", "/api/events"]`.
- Клиент не требует отдельного поля `ws_path` — path извлекается из URL `server: wss://example.com/custom-path`.
- QUIC обфускация реализована в `obfuscated.go` на обеих сторонах — менять синхронно.
- Сейчас `Obfuscation` в конфиге — `bool`. Меняем на блок: `obfuscation: { enabled: true }` (аналогично `crypto: { enabled: true, key: ... }`). В блок можно будет добавлять поля без breaking changes.

## Требования

- RQ-001 Система ДОЛЖНА поддерживать uTLS для WS транспорта с имитацией Chrome 120+ fingerprint.
- RQ-002 Клиент ДОЛЖЕН использовать path из URL сервера (например `wss://example.com/api/v1/events` → path `/api/v1/events`). Отдельное поле `ws_path` не требуется.
- RQ-003 Система ДОЛЖНА поддерживать кастомный SNI через список доменов `tls.sni: ["www.cloudflare.com", "www.google.com"]`. SNI выбирается рандомно из списка при каждом подключении. Работает для WS (TCP/TLS) и QUIC (quic-go `tls.Config.ServerName`). Если список пуст — используется `tls.server_name` или адрес сервера (текущее поведение).
- RQ-003a При использовании кастомного SNI с самоподписанным сертификатом сервера `verify_mode` ДОЛЖЕН быть `insecure` (иначе проверка CN vs SNI убьёт handshake). DPI не сверяет SNI с сертификатом — это безопасно.
- RQ-004 Система ДОЛЖНА добавить опциональный padding payload до кратного размера (256/512/1024 байт) для WS транспорта.
- RQ-005 Конфиг `obfuscation` меняется с `bool` на блок: `obfuscation: { enabled: true, ... }`. Совместимость: `obfuscation: true` эквивалентно `obfuscation: { enabled: true }`.
- RQ-006 QUIC обфускация ДОЛЖНА быть усилена: XOR всего payload (не только длины), удаление plaintext nonce.
- RQ-007 Новые опции ДОЛЖНЫ быть доступны в Web UI (настройки Transport и TLS).
- RQ-008 Система ДОЛЖНА сохранять обратную совместимость: без новых опций поведение идентично текущему.
- RQ-009 uTLS ДОЛЖЕН применяться только для WS; для QUIC сохранить существующий `crypto/tls` (до проверки совместимости).
- RQ-010 Сервер ДОЛЖЕН принимать список разрешённых WS path из конфига (`server.ws_paths: ["/tunnel", "/api/events"]`). Path, не входящий в список, отвечает 404 до WebSocket upgrade.
- RQ-011 Сервер ДОЛЖЕН отбрасывать padding из входящих WS сообщений. Формат: длина оригинального сообщения (4 байта big-endian) + оригинальные данные + padding-байты.
- RQ-012 QUIC обфускация на сервере ДОЛЖНА быть обновлена синхронно с клиентом (XOR всего payload, без plaintext nonce).

## Вне scope

- uTLS для QUIC/quic-go (требует отдельного исследования совместимости).
- Обфускация/маскировка серверного ответа (исходящий трафик от сервера к клиенту).
- HTTP/2 или gRPC транспорт.
- Многопоточный/мультиплексный padding control.
- Интеграция с внешними прокси (TOR, Shadowsocks, chain proxy).
- Мониторинг DPI-детектов и метрики по ним.
- Автоматическая синхронизация конфига WS path между клиентом и сервером (path задаётся в URL клиента, сервер валидирует по allowlist).

## Критерии приемки

### AC-001 uTLS для WS транспорта

- Почему это важно: Go-шный ClientHello блокируется DPI по JA3; uTLS имитирует браузер.
- **Given** клиент сконфигурирован с `transport: "tcp"` и `tls.utls.enabled: true`
- **When** клиент выполняет TLS handshake с сервером
- **Then** ClientHello содержит типичный Chrome fingerprint (cipher suites, extensions order, ALPN)
- Evidence: wireshark/tcpdump показывает `JA3: 771,4865-4866-4867-...` (Chrome), а не Go-шный набор
- **And** соединение успешно устанавливается (WebSocket upgrade проходит)

### AC-002 uTLS fallback

- Почему это важно: uTLS может не поддержать все серверные кривые; нужен fallback на crypto/tls.
- **Given** uTLS handshake не удался (ошибка сертификата/timeout)
- **When** `tls.utls.fallback` включён (default: true)
- **Then** клиент повторяет handshake с `crypto/tls` без uTLS
- Evidence: в логе `utls handshake failed, falling back to crypto/tls`

### AC-003 Кастомный WS path

- Почему это важно: блокировка `/tunnel` — самый дешёвый вектор для whitelist-фильтрации.
- Server touch: `src/internal/bootstrap/server/server.go` — хендлер проверяет входящий path по allowlist `server.ws_paths`. Если path не в списке — 404.
- **Given** клиент указывает `server: wss://example.com/api/events`, а в конфиге сервера `ws_paths: ["/tunnel", "/api/events"]`
- **When** клиент открывает WebSocket соединение на `wss://example.com/api/events`
- **Then** HTTP Upgrade request идёт на `GET /api/events`, сервер проверяет path по allowlist и пропускает upgrade
- Evidence: tcpdump показывает HTTP-запрос с path `/api/events`; сервер успешно завершает upgrade
- **And** при запросе на `/tunnel` (входящий в allowlist по умолчанию) соединение также работает
- **And** при запросе на `/secret` (не в allowlist) сервер возвращает 404 без upgrade

### AC-004 Кастомный SNI (из списка)

- Почему это важно: whitelist-блокировка по SNI обходится подстановкой разрешённого домена. Рандомный выбор из списка не даёт DPI связать все подключения с одним SNI.
- **Given** в конфиге задан `tls.sni: ["www.cloudflare.com", "www.google.com", "www.github.com"]` и `tls.verify_mode: "insecure"`
- **When** клиент выполняет 3 TLS handshake (3 переподключения) по WS или QUIC
- **Then** каждый handshake содержит случайный SNI из списка (с высокой вероятностью разные)
- Evidence: tcpdump показывает разные SNI в разных сессиях; handshake успешен
- **And** если `verify_mode: "verify"`, handshake падает с ошибкой certificate verification
- **And** если SNI не задан (nil/пустой список), используется `tls.server_name` или адрес сервера (текущее поведение)
- **And** внутри одной сессии SNI фиксирован (не меняется между 0-RTT попытками в QUIC)

### AC-005 Padding для WS

- Почему это важно: фиксированный размер пакетов не позволяет DPI анализировать объём трафика.
- Server touch: серверный `ReadMessage()` должен уметь отбрасывать padding. Формат фрейма: `[4B big-endian payload length][payload][padding bytes]`.
- **Given** `transport.padding.enabled: true`, `transport.padding.size: 512`
- **When** клиент отправляет сообщение размером <512 байт
- **Then** сообщение упаковывается во фрейм: длина (4B) + payload + padding до 512 байт
- Evidence: wireshark показывает длину пакета 512 (+header overhead)
- **And** на стороне сервера padding отбрасывается, оригинальное сообщение восстанавливается по префиксу длины
- **And** сообщения >512 байт дополняются до ближайшего кратного 512 (ceil(hdr+payload, 512))

### AC-006 Усиленная QUIC обфускация

- Почему это важно: текущий XOR только длины + 8 байт plaintext nonce — тривиальный сигнатурный паттерн.
- Server touch: `src/internal/transport/quic/obfuscated.go` — серверная часть меняется синхронно с клиентской.
- Механизм nonce: TLS Exporter (RFC 5705) — `tls.ConnectionState.ExportKeyingMaterial("kvn-obfuscation", nil, 8)`. После handshake обе стороны независимо выводят 8-байт nonce без передачи по сети. Работает при любом `verify_mode` (включая `insecure`), т.к. master secret одинаковый.
- **Given** клиент использует QUIC транспорт с `quic.obfuscate: true`
- **When** клиент отправляет сообщение
- **Then** весь payload XOR-ится с nonce (не только длина), nonce не передаётся в plaintext
- Evidence: в начале QUIC stream нет читаемых 8 байт; payload выглядит как случайные байты
- **And** сервер корректно деобфусцирует сообщение без потери данных

## Допущения

- Сервер уже обслуживает HTTPS на стандартном 443 порту (не требует изменений).
- uTLS библиотека стабильна и совместима с Go 1.25 (проверить при реализации).
- Для QUIC uTLS-совместимость не гарантируется — QUIC остаётся на `crypto/tls`.
- WS padding: фрейм `[4B payload_len][payload][random padding до кратного]`. Compression удалён из проекта.
- DNS-запросы клиента не требуют отдельного кастомного SNI (используют стандартный системный DNS).
- QUIC nonce: TLS Exporter (RFC 5705) — `ExportKeyingMaterial("kvn-obfuscation", nil, 8)`. 0 байт на wire, уникален на сессию, работает при любом `verify_mode`.

## Критерии успеха

- SC-001 Соединение устанавливается через WS+uTLS в сети, где Go-шный ClientHello блокируется (симулировать через iptables/nftables, блокируя JA3 Go).
- SC-002 Ручная проверка через wireshark/tcpdump: JA3 Chrome, не Go.
- SC-003 Размер WS сообщений после padding — строго кратен заданному (для сообщений < размера padding).

## Краевые случаи

- **uTLS + TLS 1.2 сервер**: если сервер не поддерживает TLS 1.3, uTLS всё равно должен работать (configuration зависит от набора).
- **uTLS + mTLS**: проверить, что uTLS поддерживает клиентские сертификаты (client_auth).
- **Кастомный WS path + CDN**: если перед сервером CDN (Cloudflare, nginx), path должен быть разрешён в конфигурации CDN.
- **Padding + Multiplex**: мьюльтиплексные сообщения могут пересекаться с padding — проверить, корректно ли BatchWriter их обрабатывает.
- **QUIC nonce rotation**: при переподключении nonce должен генерироваться заново.
- **SNI + verify_mode**: кастомный SNI работает только с `verify_mode: insecure`. При `verify: verify` сертификат сервера не совпадёт с SNI-доменом → handshake error.

## Открытые вопросы

1. uTLS версия — решено: `HelloChrome_Auto`. Всегда актуальный fingerprint без maintenance.
2. HelloGolang — решено: не добавляем. Не несёт пользы для anti-DPI.
3. Padding — решено: внутри BatchWriter. Единый write path для WS.
4. Cipher suites — решено: фиксированный `HelloChrome_Auto`. Рандомизация создаст аномальный JA3.
5. QUIC nonce — решено: TLS Exporter (RFC 5705). `ExportKeyingMaterial("kvn-obfuscation", nil, 8)` через `tls.ConnectionState`. 0 байт на wire.
6. WS path — решено: path в URL клиента, сервер проверяет по allowlist `server.ws_paths`.
