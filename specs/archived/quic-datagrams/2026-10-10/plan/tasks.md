# QUIC Datagrams Задачи

## Phase Contract

Inputs: `specs/active/quic-datagrams/plan.md`, `spec.md`.
Outputs: упорядоченные задачи с `Touches:`, Surface Map и покрытием AC.
Stop if: задачи расплывчаты или AC не сопоставляется задачами.

## Surface Map

| Surface | Tasks |
|---------|-------|
| src/internal/transport/quic/conn.go | T1.1, T1.2 |
| src/internal/transport/quic/dial.go | T1.1 |
| src/internal/transport/quic/listen.go | T1.1 |
| src/internal/transport/quic/quicfactory.go | T1.1 |
| src/internal/transport/quic/obfuscated.go | T3.3 |
| src/internal/transport/transport.go | T1.1 |
| src/internal/bootstrap/client/tun.go | T2.1 |
| src/internal/bootstrap/server/handler.go | T2.2 |
| src/internal/tunnel/session.go | T3.1, T3.2 |
| src/internal/config/client.go | T3.4 |
| src/internal/config/server.go | T3.4 |
| src/internal/transport/quic/quic_test.go | T1.3, T3.5 |
| src/internal/tunnel/session_test.go | T3.5 |
| src/integration/tunnel_integration_test.go | T4.1 |
| docs/ru/config.md, docs/en/config.md | T4.2 |
| src/internal/webui/frontend/src/types.ts | T4.3 |
| src/internal/webui/frontend/src/TabbedForm.tsx | T4.3 |
| src/internal/webui/handler_connect.go | T4.3 |

## Implementation Context

- Цель MVP: real-time UDP идёт unreliable QUIC datagrams (нет ретрансмита/HOL), TCP/control — stream; capability аддитивна, старые peer → stream, Android/WS не затронуты.
- Инварианты/семантика:
  - датаграмма несёт сырой IP-пакет без фрейминга; reader-loop датаграмм пишет в TUN и не роняет сессию на потерях.
  - контроль/TCP всегда по stream; датаграммы только для UDP.
  - capability кодируется значением поля `Transport` (`quic` ↔ `quic-dgram`) — новых полей/тегов нет.
- Ошибки/лимиты: пакет больше предела datagram frame → stream-фоллбэк; потеря/дубликат/перестановка датаграмм допустимы.
- Контракты/протокол: формат handshake-фреймов не меняется; ServerHello.Transport эхом подтверждает режим.
- Границы scope: не трогаем WS/Android, схему аутентификации, фреймовый формат, padding.
- Proof signals: unit round-trip датаграммы; integration UDP-датаграмма + TCP stream; тест потерь без HOL; тест старого peer → stream.
- References: DEC-001..DEC-005 в plan.md.

## Фаза 1: Datagram API транспорта (MVP-1)

Цель: QUIC-обёртка умеет слать/принимать датаграммы и знает свой лимит.

- [x] T1.1 Datagram API на `QUICConn` + `EnableDatagrams` — outcome: `SendDatagram`/`ReceiveDatagram` и признак поддержки; датаграммы включены в `quic.Config` клиента и сервера; Touches: src/internal/transport/quic/conn.go, src/internal/transport/quic/dial.go, src/internal/transport/quic/listen.go, src/internal/transport/quic/quicfactory.go, src/internal/transport/transport.go
      Proof: code src/internal/transport/quic/conn.go SendDatagram
      Proof: code src/internal/transport/quic/listen.go Listen
- [x] T1.2 Лимит размера датаграммы — outcome: пакет больше предела возвращает понятную ошибку/флаг для stream-фоллбэка; Touches: src/internal/transport/quic/conn.go
      Proof: code src/internal/transport/quic/conn.go DatagramTooLarge
- [x] T1.3 Тесты datagram API — outcome: unit round-trip датаграммы и проверка лимита; Touches: src/internal/transport/quic/quic_test.go
      Proof: test src/internal/transport/quic/quic_test.go TestQUICDatagramRoundTrip
      Proof: test src/internal/transport/quic/quic_test.go TestQUICDatagramTooLarge

## Фаза 2: Capability-негоциация (MVP-2)

Цель: включать datagram-режим только по обоюдной поддержке.

- [x] T2.1 Клиент объявляет capability — outcome: при `transport=quic` и включённом флаге ClientHello несёт `Transport="quic-dgram"`; Touches: src/internal/bootstrap/client/tun.go, src/internal/transport/quic/conn.go
      Proof: code src/internal/bootstrap/client/tun.go handshakeSession
- [x] T2.2 Сервер включает и подтверждает — outcome: по `Transport=="quic-dgram"` сервер включает датаграммы и эхом ставит `ServerHello.Transport="quic-dgram"`; иначе — как раньше; Touches: src/internal/bootstrap/server/handler.go
      Proof: code src/internal/bootstrap/server/handler.go handleStream
- [x] T2.3 Тесты негоциации — outcome: «new+new → dgram», «старый peer → stream, без ошибок»; Touches: src/internal/transport/quic/quic_test.go, src/internal/transport/quic/conn.go
      Proof: test src/internal/transport/quic/quic_test.go TestDatagramCapableMarker
      Proof: test src/internal/transport/quic/quic_test.go TestDatagramTransportHandshakeRoundTrip

## Фаза 3: Datagram-путь сессии (MVP-3)

Цель: UDP уходит датаграммой, входящие датаграммы попадают в TUN.

- [x] T3.1 Отправка UDP датаграммой — outcome: при наличии datagram-capability UDP идёт `SendDatagram` (сырой IP-пакет), иначе stream; Touches: src/internal/tunnel/session.go, src/internal/transport/transport.go
      Proof: code src/internal/tunnel/session.go sendDatagram
- [x] T3.2 Reader-loop датаграмм → TUN — outcome: отдельная горутина читает датаграммы и пишет IP-пакет в TUN, не роняя сессию на потерях; Touches: src/internal/tunnel/session.go
      Proof: code src/internal/tunnel/session.go startDatagramReader
- [x] T3.3 Паритет обфускации — outcome: датаграмма обфусцируется/деобфусцируется так же, как stream, либо режим гейтится на выключенную обфускацию; Touches: src/internal/transport/quic/obfuscated.go
      Proof: code src/internal/transport/quic/obfuscated.go SendDatagram
      Proof: test src/internal/transport/quic/quic_test.go TestObfuscatedDatagramRoundTrip
- [x] T3.4 Конфиг-флаг `udp_datagrams` (default on) — outcome: флаг на клиенте и сервере управляет режимом; Touches: src/internal/config/client.go, src/internal/config/server.go, src/internal/bootstrap/client/tun.go, src/internal/bootstrap/server/handler.go
      Proof: code src/internal/config/client.go UDPDatagramsEnabled
      Proof: code src/internal/config/server.go UDPDatagramsEnabled
- [x] T3.5 Тесты datagram-пути — outcome: unit-тест выбора datagram/stream и одного round-trip; Touches: src/internal/tunnel/session_test.go, src/internal/transport/quic/quic_test.go
      Proof: test src/internal/tunnel/session_test.go TestSendDatagramSelection

## Фаза 4: Проверка

Цель: доказать end-to-end и совместимость.

- [x] T4.1 Integration: UDP датаграммой + TCP stream + потеря без HOL — outcome: e2e-тест фиксирует счётчики и задержку; Touches: src/integration/tunnel_integration_test.go
      Proof: test src/integration/tunnel_integration_test.go TestQUICDatagramAndStreamIsolation
      Proof: test src/integration/tunnel_integration_test.go TestQUICDatagramLossDoesNotBlockStream
- [x] T4.2 Regression + docs — outcome: WS-тесты зелёные, docs ru/en описывают `udp_datagrams`; Touches: docs/ru/config.md, docs/en/config.md, src/internal/transport/quic/quic_test.go
      Proof: docs docs/ru/config.md
      Proof: docs docs/en/config.md
- [x] T4.3 kvn-web: настройка `udp_datagrams` (UI + merge) — outcome: чекбокс на вкладках Advanced/Global, значение прокидывается в конфиг клиента; Touches: src/internal/webui/frontend/src/types.ts, src/internal/webui/frontend/src/TabbedForm.tsx, src/internal/webui/handler_connect.go
      Proof: code src/internal/webui/frontend/src/TabbedForm.tsx udp_datagrams
      Proof: code src/internal/webui/handler_connect.go mergeConfig

## Покрытие критериев приемки

- AC-001 -> T1.1, T3.1, T3.2, T4.1, T4.3
- AC-002 -> T3.1, T3.2, T4.1
- AC-003 -> T2.1, T2.2, T2.3, T4.1
- AC-004 -> T2.3, T4.2, T4.3

## Заметки

- Порядок: транспорт → негоциация → сессия; T1/T2 независимы.
- Proof для каждой закрытой задачи — строкой `Proof:` под задачей при реализации.
