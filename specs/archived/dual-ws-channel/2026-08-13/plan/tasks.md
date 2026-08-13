# Dual WebSocket Channel — Задачи

## Phase Contract

Inputs: `plan.md`, `data-model.md`, `spec.md` для уточнения `AC-*`.
Outputs: упорядоченные исполнимые задачи с `Touches:` и покрытием критериев.
Stop if: задачи расплывчаты или хотя бы один AC непривязываем к исполнимой задаче.

## Surface Map

| Surface | Tasks |
|---------|-------|
| protocol/handshake.yaml | T1.1 |
| src/internal/protocol/handshake/types_gen.go (generated) | T1.1 |
| src/internal/protocol/handshake/handshake.go | T1.2 |
| src/internal/tunnel/demux.go | T2.1 |
| src/internal/tunnel/session.go | T2.2, T3.1, T3.3 |
| src/internal/bootstrap/server/server.go | T3.1 |
| src/internal/bootstrap/server/handler.go | T3.1 |
| src/internal/config/client.go | T3.2 |
| src/internal/bootstrap/client/tun.go | T3.2 |
| src/internal/bootstrap/client/dial.go | T3.2 |
| src/integration/tunnel_integration_test.go | T4.1 |
| src/internal/tunnel/session_test.go | T4.2 |

## Implementation Context

- Цель MVP: UDP-пакеты идут отдельным WS-каналом (secondary) от TCP, что убирает HoL VoIP-медиа; один `tunnel.Session` покрывает клиент и сервер.
- Границы приемки: AC-001…AC-008.
- Ключевые правила: tag-based handshake расширяется, а не меняется (DEC-001); серверный реестр активных `*tunnel.Session` по `session_id` (DEC-002); классификация протокола в `tunToWS` — единственная точка на обе роли (DEC-003); каналы поднимаются последовательно primary→secondary, фоллбэк без конфига (DEC-004); буфер secondary 300 мс / 1024 пакета (DEC-005); включение — флаг `multi_channel` (DEC-006).
- Инварианты: secondary привязывается только при совпадении токена (RQ-002); оба канала делят один `sessionCipher` (RQ-008); идентичный WSConfig/uTLS/padding для консистентного fingerprint (RQ-007); control/proxy/DNS/keepalive остаются на primary.
- Контракты/протокол: `ChannelTag=0x0C`, `SessionTag=0x0D` в ClientHello (TLV, декодер пропускает неизвестные теги — backward compat, `handshake.go:83-93`); сгенерированные файлы не редактировать вручную, перегенерировать codegen'ом.
- Proof signals: `go test -race ./src/integration/... ./src/internal/tunnel/... ./src/internal/protocol/handshake/...` PASS; интеграционный тест считает кадры по стримам; ручной check: звонок через kvn-web в TUN с `multi_channel: true`.
- Вне scope: proxy-режим, Android (OkHttp), QUIC, новые фрейм-типы, изменение auth-схемы, persisted data model (no-change).

## Фаза 1: Протокол (первым, foundation)

Цель: ClientHello несёт вторичность + session_id, wire-совместимо в обе стороны.

- [x] T1.1 Добавить теги `ChannelTag=0x0C`, `SessionTag=0x0D` и поля `Channel`, `SessionId` в ClientHello в `protocol/handshake.yaml`; перегенерировать go-типы. Touches: protocol/handshake.yaml, src/internal/protocol/handshake/types_gen.go
  Proof: code src/internal/protocol/handshake/types_gen.go ClientHello
- [x] T1.2 Обновить `EncodeClientHello`/`DecodeClientHello`: сериализация/парсинг новых тегов; отсутствие тегов = primary; неизвестные теги пропускаются (backward compat, AC-007). Touches: src/internal/protocol/handshake/handshake.go
  Proof: code src/internal/protocol/handshake/handshake.go EncodeClientHello

## Фаза 2: Ядро tunnel.Session (MVP)

Цель: мульти-стрим в `tunnel.Session` + классификация протокола — покрывает AC-002/AC-003.

- [x] T2.1 Добавить `parseIPProto(buf []byte) (isUDP bool)` в demux: IPv4 proto field [9], IPv6 next-header [6]; экстеншн-хеадеры — fallback на primary, без паники на коротких пакетах. Touches: src/internal/tunnel/demux.go
  Proof: code src/internal/tunnel/demux.go parseIPProto
- [x] T2.2 Реализовать в `Session`: поле `secondary StreamConn` + `SetSecondary`; в `tunToWS` классификация по `parseIPProto` → UDP шлёт в secondary (если установлен), иначе в primary; вторичный read-loop (FrameTypeData → decrypt → `tunDev.Write`) с общим `sessionCipher`. Touches: src/internal/tunnel/session.go
  Proof: code src/internal/tunnel/session.go SetSecondary

## Фаза 3: Сервер и клиент (MVP)

Цель: поднятие и привязка вторичного канала на обеих ролях + буфер + флаг.

- [x] T3.1 Сервер: реестр `tunnelSessRefs sync.Map` на `Server` (key=session_id, запись до `tunSess.Run`, delete в defer после Run); ветка вторичного handshake в `handleStream`: `Channel=="secondary"` → `sm.Get(id)`, проверка совпадения токена с `tokenName` сессии, `SetSecondary`, отклонение при несовпадении (AC-001). Touches: src/internal/bootstrap/server/server.go, src/internal/bootstrap/server/handler.go
  Proof: code src/internal/bootstrap/server/handler.go handleSecondaryStream
- [x] T3.2 Клиент: флаг `MultiChannel bool` в клиентском конфиге; в `runSession` после primary-handshake (если флаг) — dial второго стрима (переиспользовать `dialStream`), hello с `Channel="secondary"`+`SessionId`; при ошибке/закрытии — продолжить на primary без изменения конфига (AC-004); callback `SetSecondary`. Touches: src/internal/config/client.go, src/internal/bootstrap/client/tun.go, src/internal/bootstrap/client/dial.go
  Proof: code src/internal/bootstrap/client/tun.go dialSecondaryChannel
- [x] T3.3 Буфер secondary до готовности primary: вторичный read-loop ждёт `primaryReady` guard лимит 300 мс / 1024 пакета, затем flush в TUN или drop (AC-005). Touches: src/internal/tunnel/session.go
  Proof: code src/internal/tunnel/session.go secondaryToTun

## Фаза 4: Проверка

Цель: доказать работу и защитить от регрессий.

- [x] T4.1 Интеграционный тест двухканального round-trip в `tunnel_integration_test.go`: два WS на тестовый сервер, secondary привязывается по `session_id`, UDP-кадр доходит по secondary, TCP — по primary; возвратный UDP-ответ приходит по secondary; канал с чужим токеном отклонён; вторичный handshake failure не ломает primary (AC-001/002/003/004). Touches: src/integration/tunnel_integration_test.go
  Proof: test src/integration/tunnel_integration_test.go TestTunnelDualChannelRoundtrip
- [x] T4.2 Unit-тесты: классификация протокола, буфер (копится до ready, flush после, drop при переполнении), crypto cross-channel (шифровка secondary-read дешифруется общим cipher), backward compat handshake hello со всеми/без тегов (AC-005/006/007); `go vet` + `go test -race ./src/...` (AC-008). Touches: src/internal/tunnel/session_test.go
  Proof: test src/internal/tunnel/session_test.go TestParseIPProto

## Покрытие критериев приемки

- AC-001 -> T3.1, T4.1
- AC-002 -> T2.2, T4.1
- AC-003 -> T2.2, T4.1
- AC-004 -> T3.2, T4.1
- AC-005 -> T3.3, T4.2
- AC-006 -> T3.2, T4.2
- AC-007 -> T1.2, T4.2
- AC-008 -> T4.1, T4.2

## Заметки

- Порядок строгий: Фаза 1 (протокол) первой — от неё зависят все остальные; Фазы 2 и 3 независимы по поверхностям и могут идти параллельно.
- Приоритет протокольному backward-compat (T1.2, AC-007) — старые клиенты/серверы не должны ломаться.
- Сгенерированные файлы (`types_gen.go`) только перегенерируются codegen'ом, редактируются вручную только YAML.
- Trace-маркеры: `@sk-task dual-ws-channel#T*.N` над owning declaration в коде, `@sk-test dual-ws-channel#T*.N` в тестах (не на package/import/file-header).