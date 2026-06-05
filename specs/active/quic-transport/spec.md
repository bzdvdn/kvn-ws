# QUIC Transport

## Scope Snapshot

- In scope: замена WebSocket (TCP) на QUIC (UDP + TLS 1.3) как транспорта для туннеля, с обратной совместимостью через опцию транспорта.
- Out of scope: полный отказ от TCP WebSocket, миграция существующих клиентов/серверов.

## Цель

Пользователи с высоким RTT (>50ms) и/или потерями пакетов получают 3-10x прирост скорости туннеля за счёт устранения TCP-over-TCP meltdown. Одновременно улучшается маскировка трафика: QUIC на порту 443 неотличим от обычного браузерного QUIC.

## Основной сценарий

1. Пользователь указывает `transport: quic` в конфиге клиента (сервер включает поддержку QUIC на порту 443).
2. Клиент и сервер договариваются об использовании QUIC вместо WebSocket в рамках handshake.
3. Все tunnelled data, proxy-фреймы и контрольные сообщения передаются поверх QUIC stream.
4. При обрыве соединения QUIC 0-RTT reconnect быстрее восстанавливает сессию.
5. Если QUIC недоступен (UDP blocked), клиент автоматически падает на TCP/WebSocket.

## User Stories

- P1: Пользователь с плохим каналом (60ms RTT, 1% loss) получает скорость >5 Mbps (сейчас 0.08).
- P2: Пользователь в регионе с DPI не блокируется по сигнатуре WebSocket Upgrade.

## MVP Slice

Один bidirectional QUIC stream заменяет WebSocket-соединение. Только TUN mode (data frames). Proxy-frames — второй итерацией.

## First Deployable Outcome

Два бинарника `kvn-server` и `kvn-client` с флагом `--transport quic` (или `transport: quic` в YAML) успешно обмениваются Hello/data-фреймами через QUIC, и `iperf` через туннель показывает >5 Mbps.

## Scope

- `src/internal/transport/quic/` — новый пакет: dial/accept, QUIC stream wrapping
- `src/internal/config/` — поле `Transport` в конфиге (server + client)
- `src/internal/protocol/handshake/` — согласование транспорта в Hello
- `src/internal/bootstrap/client/` — выбор транспорта при dial
- `src/cmd/server/` — QUIC listener рядом с TCP
- `src/internal/tunnel/` — абстракция stream (замена `*websocket.WSConn` на интерфейс)

## Контекст

- Текущий транспорт: `gorilla/websocket` поверх TCP+TLS. Весь туннель завязан на `*websocket.WSConn`.
- `quic-go` — зрелая Go-библиотека QUIC (используется в production в Caddy, Syncthing).
- QUIC использует UDP, может потребоваться настройка firewall (UDP 443).
- Сервер должен слушать и TCP (для старых клиентов) и UDP (QUIC) на порту 443.

## Требования

- RQ-001 Клиент и сервер ДОЛЖНЫ поддерживать выбор транспорта (tcp/ws, quic) через конфиг.
- RQ-002 QUIC транспорт ДОЛЖЕН использовать TLS 1.3 с тем же сертификатом, что и TCP.
- RQ-003 При недоступности QUIC (UDP blocked) система ДОЛЖНА падать на TCP/WebSocket.
- RQ-004 QUIC транспорт НЕ ДОЛЖЕН снижать производительность на хороших каналах (<10ms RTT, 0% loss) по сравнению с WebSocket.
- RQ-005 Существующий WebSocket транспорт ДОЛЖЕН остаться без изменений.

## Вне scope

- Proxy frames поверх QUIC streams (MPTCP-стиль) — вторая итерация
- Миграция конфигов и скриптов установки — документируется отдельно
- Windows — QUIC должен работать, но оптимизация под Windows не в фокусе

## Критерии приемки

### AC-001 QUIC handshake и data flow

- Почему: базовый обмен данными через QUIC
- **Given** сервер с `transport: quic` и клиент с `transport: quic`
- **When** клиент подключается
- **Then** handshake проходит, data-фреймы передаются, `iperf` через туннель работает
- Evidence: логи показывают "QUIC transport established", iperf показывает throughput

### AC-002 Fallback на TCP

- Почему: не терять соединение при блокировке UDP
- **Given** сервер с `transport: quic` и клиент с `transport: quic`, UDP заблокирован
- **When** QUIC dial не удаётся
- **Then** клиент автоматически переключается на TCP/WebSocket
- Evidence: лог "QUIC dial failed, falling back to TCP", соединение работает через WS

### AC-003 Производительность на плохом канале

- Почему: основная цель фичи
- **Given** два хоста с 60ms RTT и 1% simulated packet loss
- **When** `iperf TCP` через туннель
- **Then** throughput > 5 Mbps (vs <1 Mbps на WebSocket)
- Evidence: iperf результат

### AC-004 Обратная совместимость

- Почему: старые клиенты не должны ломаться
- **Given** сервер с `transport: quic` (с TCP listener) и старый клиент без `transport` (ws)
- **When** старый клиент подключается
- **Then** соединение через WebSocket TCP работает штатно
- Evidence: handshake success, данные передаются

## Допущения

- quic-go v0.50+ стабилен на Go 1.25
- Сервер имеет открытый UDP 443
- TLS-сертификат одинаков для TCP и QUIC

## Краевые случаи

- UDP заблокирован файрволом → fallback на TCP
- QUIC 0-RTT reconnect при перезапуске клиента
- MTU discovery в QUIC (PMTUD) — может фрагментировать на уровне IP

## Открытые вопросы

- Использовать один bidirectional QUIC stream (как сейчас WS) или отдельные streams для data/control/proxy?
- Нужна ли опция `transport: auto` (try QUIC first, fallback TCP)?
- Как согласовывать транспорт в handshake — отдельный флаг в ClientHello или параллельный listener?
