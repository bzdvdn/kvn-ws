# Local Proxy Mode — SOCKS5/HTTP прокси на localhost

## Scope Snapshot

- In scope: Режим работы клиента kvn-ws как локального прокси-сервера (SOCKS5 + HTTP CONNECT), перенаправляющего трафик через WebSocket-туннель. Единая конфигурация и бинарник для Windows, Linux, macOS.
- Out of scope: TUN-режим (существующий), серверная часть, модификация handshake/сессий/маршрутизации.

## Цель

Пользователи kvn-ws на любой ОС (Windows, macOS, Linux) могут использовать туннель без root/админ прав, настроив приложения/систему на локальный прокси. Единый бинарник с двумя mode: `tun` и `proxy`.

## Основной сценарий

1. Пользователь запускает клиент с `--mode proxy` (или `mode: proxy` в конфиге)
2. Клиент подключается к WS-туннелю (handshake, auth — существующая логика)
3. Клиент открывает SOCKS5 listener на `127.0.0.1:2310`
4. Приложения настраиваются на `127.0.0.1:2310` как SOCKS5 прокси
5. Входящие TCP-соединения через SOCKS5 оборачиваются в WS-фреймы и отправляются на сервер
6. Сервер извлекает TCP-поток, соединяется с целевым хостом и возвращает данные обратно через WS
7. CIDR/domain exclusion (routing rules) работают так же, как в TUN-режиме

## User Stories

- P1: Как пользователь Windows, я хочу запустить kvn-ws без установки TUN-драйвера, чтобы получить VPN-туннель.
- P2: Как DevOps, я хочу одинаковый конфиг для всех ОС, чтобы не плодить разные инструкции.

## MVP Slice

SOCKS5 listener + WS forwarding + mode flag. Закрывает AC-001, AC-003, AC-004, AC-006.

## First Deployable Outcome

`kvn-ws --mode proxy --config configs/client.yaml` поднимает SOCKS5 на localhost:2310, curl через прокси (`--socks5 127.0.0.1:2310`) проходит через туннель.

## Scope

- SOCKS5 (RFC 1928) listener на localhost
- HTTP CONNECT handler для HTTPS
- Mode flag `--mode proxy|tun` в клиенте
- Прокси-конфиг: mode, порт, bind address, опциональная auth
- TCP-stream поверх WS-фреймов (новый frame type)
- Серверный TCP-forwarder (из WS-фрейма к целевому хосту)
- CIDR/domain exclusion для прокси-режима
- Тесты: SOCKS5 round-trip, HTTP CONNECT, mode switching, exclusion rules

## Контекст

- Репозиторий: `kvn-ws` (github.com/bzdvdn/kvn-ws), Go 1.22+, gorilla/websocket v1.5.3
- Существующий TUN-режим: клиент открывает TUN, читает IP-пакеты, шлёт как Data фреймы
- Новый proxy-режим: клиент открывает TCP-listener, принимает TCP-соединения, шлёт как TCP-фреймы (новый FrameTypeProxy)
- Сервер: новый handler для FrameTypeProxy — соединяется с целевым хостом:портом, форвардит данные
- Routing rules (CIDR/domain exclusion) работают одинаково в обоих режимах

## Требования

- RQ-001 Клиент ДОЛЖЕН поддерживать выбор режима через поле `mode` в конфиге (`proxy|tun`) и флаг `--mode` для переопределения.
- RQ-002 В режиме `proxy` клиент ДОЛЖЕН открывать SOCKS5 listener на адресе:порту из конфига.
- RQ-003 В режиме `proxy` клиент ДОЛЖЕН поддерживать HTTP CONNECT для HTTPS-трафика.
- RQ-004 Клиент ДОЛЖЕН опционально требовать username/password для SOCKS5 (RFC 1929).
- RQ-005 Клиент ДОЛЖЕН оборачивать TCP-соединения в новый тип фрейма `FrameTypeProxy` и отправлять через WS-туннель.
- RQ-006 Сервер ДОЛЖЕН обрабатывать `FrameTypeProxy` — устанавливать TCP-соединение к указанному хосту:порту и форвардить данные.
- RQ-007 CIDR/domain exclusion из конфигурации маршрутизации ДОЛЖНЫ работать в proxy-режиме (direct-трафик минует туннель).
- RQ-008 Весь код ДОЛЖЕН компилироваться и работать на Windows, Linux, macOS без дополнительных зависимостей.

## Вне scope

- UDP через SOCKS5 (только TCP, UDP остаётся в TUN-режиме)
- TUN-режим (не меняется)
- Серверная часть (кроме нового FrameTypeProxy handler)
- Существующие протоколы handshake, сессий, аутентификации, IP-пула

## Критерии приемки

### AC-001 SOCKS5 listener принимает TCP-соединения

- Почему это важно: основа прокси-режима — клиенты должны подключаться через SOCKS5.
- **Given** клиент запущен в режиме `proxy`
- **When** клиент устанавливает SOCKS5 TCP-соединение к localhost:port
- **Then** соединение принято, прокси-хендшейк выполнен (RFC 1928)
- Evidence: тест SOCKS5 round-trip с эхо-сервером

### AC-002 HTTP CONNECT handler

- Почему это важно: многие приложения используют HTTP CONNECT, а не SOCKS5.
- **Given** клиент запущен в режиме `proxy`
- **When** HTTP-клиент шлёт CONNECT request на localhost:port
- **Then** CONNECT обработан, соединение проброшено через туннель
- Evidence: тест с curl через CONNECT или http.Client

### AC-003 Mode выбор в конфиге

- Почему это важно: пользователь выбирает режим в конфиге, pflag для переопределения.
- **Given** конфиг клиента содержит `mode: proxy` или `mode: tun`
- **When** клиент запущен
- **Then** клиент работает в соответствующем режиме (pflag `--mode` переопределяет config)
- Evidence: test с mock config проверяет вызов нужной ветки для каждого mode

### AC-004 Config port/bind

- Почему это важно: избежать конфликтов портов.
- **Given** конфиг клиента содержит `proxy_listen: 127.0.0.1:2310`
- **When** клиент запущен в proxy-режиме
- **Then** listener открыт именно на этом адресе
- Evidence: тест проверяет, что listener активен на указанном адресе

### AC-005 Auth на прокси

- Почему это важно: доступ к прокси могут получить неавторизованные процессы на localhost.
- **Given** конфиг содержит `proxy_auth: { username: user, password: pass }`
- **When** SOCKS5 клиент подключается без/с неверными credentials
- **Then** соединение отклоняется
- Evidence: тест SOCKS5 с неверным паролем получает отказ

### AC-006 Cross-platform

- Почему это важно: пользователи всех ОС должны иметь одинаковый опыт.
- **Given** Go-сборка без CGO
- **When** бинарник запущен на Windows, Linux, macOS
- **Then** proxy-режим работает без дополнительных зависимостей
- Evidence: CI собирает на всех платформах; smoke-test на Linux

### AC-007 CIDR/domain exclusion

- Почему это важно: исключения маршрутизации должны работать в обоих режимах.
- **Given** конфиг содержит `routing.include_domains` или `routing.exclude_ranges`
- **When** целевой хост совпадает с exclude правилом
- **Then** DNS резолвится (если домен), Route() решает direct/tunnel, соединение идёт напрямую или через туннель
- Evidence: тест с mock DNS проверяет Route() + excludes для доменных имён

## Допущения

- SOCKS5 и HTTP CONNECT — только TCP; UDP через SOCKS5 не поддерживается
- Для Windows не требуется установка WinTUN или любого драйвера
- FrameTypeProxy (новый, 0x05) не конфликтует с существующими типами фреймов
- Routing rules (CIDR/domain) переиспользуются без изменений из `internal/routing`
- Go-совместимость на всех целевых ОС через GOOS=windows/linux/darwin

## Критерии успеха

- SC-001 Прокси-режим проходит smoke-test на Linux за <5с
- SC-002 Обратная совместимость: TUN-режим не сломан
- SC-003 Build на 3 платформах без CGO-зависимостей

## Краевые случаи

- SOCKS5 с большим количество одновременных соединений — лимит на listener
- CONNECT к невалидному хосту — корректная ошибка клиенту
- Разрыв WS-туннеля в proxy-режиме — reconnect с пересозданием listener
- Exclusion + proxy одновременное использование — прямой трафик не идёт через туннель
- Занятый порт — клиент падает с понятной ошибкой

## Открытые вопросы

- `none` — дополнительных уточнений не требуется
