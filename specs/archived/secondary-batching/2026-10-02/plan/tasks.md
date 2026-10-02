# Secondary Channel Batching Задачи

## Phase Contract

Inputs: `spec.md` (AC-001..007), `plan.md` (DEC-001..006).
Outputs: упорядоченные исполнимыми фазами задачи.
Stop if: AC-* нельзя привязать к задачам — можно.

## Surface Map

| Surface | Tasks |
|---------|-------|
| protocol/handshake.yaml | T1.1 |
| protocol/codegen/main.go | T1.1 |
| src/internal/protocol/handshake/handshake.go | T1.2 |
| src/internal/protocol/handshake/types_gen.go | T1.1 |
| src/internal/protocol/handshake/handshake_test.go | T1.2 |
| src/internal/tunnel/session.go | T2.1, T2.2 |
| src/internal/bootstrap/client/tun.go | T3.1 |
| src/internal/bootstrap/server/handler.go | T3.2 |
| src/integration/tunnel_integration_test.go | T4.1, T4.2 |
| src/android/.../protocol/HandshakeClient.kt | T5.1 |
| src/android/.../protocol/Handshake.kt | T1.1 |
| src/android/.../protocol/FrameCodec.kt | T5.2 |
| src/android/.../vpn/KvnVpnService.kt | T5.3 |
| src/android/.../protocol/HandshakeCodecTest.kt | T5.1 |
| src/android/.../vpn/KvnVpnServiceTest.kt | T5.3 |

## Implementation Context

- Цель MVP: Go-фаза (AC-001..005) — батчинг secondary-канала с обоюдным handshake-флагом; Android (AC-006..007) — та же механика.
- Инварианты/семантика: батчинг включается **только** при обоюдном флаге (`BatchSupport` в ClientHello и ServerHello); первичный канал и TCP-трафик не меняются (RQ-005); дефолт `obfuscation.padding.size` остаётся 512.
- Параметры батча (DEC-002): `secondaryBatchMaxBytes=4096`, `secondaryBatchHold=5ms` — константы в `session.go`.
- Контракты/протокол: тег `BatchTag=0x0E`, поле `BatchSupport` в ClientHello и ServerHello (аддитивно, старые декодеры пропускают тег); батч = одно WS-сообщение с конкатенацией стандартных кадров (4-байт заголовок: Type[0], Length uint16[2:4] + payload); приёмник циклически разбирает по `Length`.
- Proof signals: `go test -race ./src/integration/... ./src/internal/protocol/handshake/...`; Android `./gradlew testDebugUnitTest`.
- Вне scope: не менять первичный канал, дефолт padding, формат кадра для не-secondary каналов; фаза 5 не начинается до зелёных фаз 1–4.
- References: DEC-001..006, RQ-001..006, AC-001..007.

## Фаза 1: Handshake-флаг (Go + codegen)

Цель: wire-контракт `BatchSupport` зафиксирован и протестирован до реализации логики.

- [x] T1.1 Добавить `BatchTag=0x0E` и поле `BatchSupport` (bool) в `client_hello`/`server_hello` в `protocol/handshake.yaml`; перегенерировать Go и Kotlin типы (`go run ./protocol/codegen/`) — outcome: `types_gen.go` и `Handshake.kt` содержат `BatchSupport` и `BatchTag=0x0E`. Touches: protocol/handshake.yaml, protocol/codegen/main.go, src/internal/protocol/handshake/types_gen.go, src/android/app/src/main/kotlin/com/kvn/client/protocol/Handshake.kt
      Proof: code protocol/handshake.yaml BatchTag
      Proof: code src/internal/protocol/handshake/types_gen.go BatchSupport
      Proof: code src/android/app/src/main/kotlin/com/kvn/client/protocol/Handshake.kt BATCH_TAG
- [x] T1.2 Реализовать encode/decode `BatchSupport` в `EncodeClientHello`/`DecodeClientHello`/`EncodeServerHello`/`DecodeServerHello` (тег 0x0E, отсутствие тега = false) + codec round-trip тесты — outcome: round-trip сохраняет флаг, hello без тега декодируется как false. Touches: src/internal/protocol/handshake/handshake.go, src/internal/protocol/handshake/handshake_test.go
      Proof: test src/internal/protocol/handshake/handshake_test.go TestClientHelloBatchSupportRoundTrip
      Proof: test src/internal/protocol/handshake/handshake_test.go TestServerHelloBatchSupportRoundTrip
      Proof: test src/internal/protocol/handshake/handshake_test.go TestClientHelloNoBatchTagBackwardCompat

## Фаза 2: Ядро session.go

Цель: батчинг и разбор батчей в tunnel-сессии.

- [x] T2.1 Добавить `batchMode atomic.Bool` + `SetBatchMode(bool)`/`BatchEnabled()` в Session; в `tunToWS` для secondary-трафика накапливать зашифрованные/зафреймленные UDP-пакеты в pooled-буфер и флашить одним `WriteMessage` при `len>=4096` или по таймеру `5ms` — outcome: в батч-режиме UDP уходит одним WS-сообщением (AC-002, AC-005). Touches: src/internal/tunnel/session.go
      Proof: code src/internal/tunnel/session.go tunToWS
      Proof: code src/internal/tunnel/session.go SetBatchMode
- [x] T2.2 В `secondaryToTun` заменить одиночный `Decode` на цикл по `Length` (разбор всех кадров из одного WS-сообщения, с корректным release payload) — outcome: все N кадров батча записаны в TUN, однокадровые сообщения работают по-старому (AC-003). Touches: src/internal/tunnel/session.go
      Proof: code src/internal/tunnel/session.go secondaryToTun

## Фаза 3: Пропагация флага (Go)

Цель: обоюдное включение батч-режима на клиенте и сервере.

- [x] T3.1 Клиент: в `dialSecondaryChannel` слать `BatchSupport=true` в вторичный ClientHello; после ServerHello с `BatchSupport=true` вызвать `SetBatchMode(true)` на сессии — outcome: uplink батчится только после подтверждения сервера (AC-001). Touches: src/internal/bootstrap/client/tun.go
      Proof: code src/internal/bootstrap/client/tun.go dialSecondaryChannel
- [x] T3.2 Сервер: в `handleSecondaryStream` при `clientHello.BatchSupport` вызвать `SetBatchMode(true)` на `tunSess` и вернуть `BatchSupport=true` в ServerHello — outcome: downlink батчится только для клиентов с флагом (AC-001, AC-004). Touches: src/internal/bootstrap/server/handler.go
      Proof: code src/internal/bootstrap/server/handler.go handleSecondaryStream

## Фаза 4: Интеграционные тесты (Go)

Цель: доказать поведение и backward-compat.

- [x] T4.1 Интеграционный тест батч-режима: клиент+сервер с флагом — ≥2 UDP-пакета в одном WS-сообщении на secondary, все пакеты доставлены в TUN, max hold ≤5ms — outcome: AC-001, AC-002, AC-003, AC-005 зелёные. Touches: src/integration/tunnel_integration_test.go
      Proof: test src/integration/tunnel_integration_test.go TestTunnelSecondaryBatching
- [x] T4.2 Интеграционный тест backward-compat: hello без флага → сервер не батчит (1 пакет = 1 сообщение), туннель работает — outcome: AC-004 зелёный. Touches: src/integration/tunnel_integration_test.go
      Proof: test src/integration/tunnel_integration_test.go TestTunnelSecondaryNoBatchBackwardCompat

## Фаза 5: Android

Цель: тот же механизм на Kotlin, wire-совместимо с Go-сервером.

- [x] T5.1 HandshakeClient: encode/decode `BatchSupport` в ClientHello/ServerHello (тег 0x0E) + HandshakeCodecTest round-trip и backward-compat — outcome: флаг передаётся, отсутствие тега = false (AC-006, AC-007). Touches: src/android/app/src/main/kotlin/com/kvn/client/protocol/HandshakeClient.kt, src/android/app/src/main/kotlin/com/kvn/client/protocol/HandshakeCodecTest.kt
      Proof: code src/android/app/src/main/kotlin/com/kvn/client/protocol/HandshakeClient.kt
      Proof: test src/android/app/src/test/java/com/kvn/client/protocol/HandshakeCodecTest.kt testServerHelloBatchSupportDecode
- [x] T5.2 FrameCodec: цикл-разбор нескольких кадров из одного WS-сообщения — outcome: батч декодируется в N фреймов (AC-006). Touches: src/android/app/src/main/kotlin/com/kvn/client/protocol/FrameCodec.kt
      Proof: code src/android/app/src/main/kotlin/com/kvn/client/protocol/FrameCodec.kt
- [x] T5.3 KvnVpnService: вторичный канал — слать `BatchSupport` в hello, батчинг uplink UDP (аккумуляция+флаш) и разбор батчей во `handleSecondaryFrame`; тесты KvnVpnServiceTest — outcome: Android батчит и корректно принимает батчи от Go-сервера (AC-006, AC-007). Touches: src/android/app/src/main/kotlin/com/kvn/client/vpn/KvnVpnService.kt, src/android/app/src/test/java/com/kvn/client/vpn/KvnVpnServiceTest.kt
      Proof: code src/android/app/src/main/kotlin/com/kvn/client/vpn/KvnVpnService.kt

## Покрытие критериев приемки

- AC-001 -> T1.1, T1.2, T3.1, T3.2, T4.1
- AC-002 -> T2.1, T4.1
- AC-003 -> T2.2, T4.1
- AC-004 -> T3.2, T4.2
- AC-005 -> T2.1, T4.1
- AC-006 -> T5.1, T5.2, T5.3
- AC-007 -> T5.1, T5.3

## Заметки

- Фазы 1–4 — Go (MVP, AC-001..005); фаза 5 — Android (AC-006..007) только после зелёных фаз 1–4 (спека: фаза 2 не начинается до фазы 1).
- Кодек-тесты (T1.2) можно вести параллельно с ядром (T2.x); интеграционные (T4.x) — после T2.x/T3.x.
- Каждая закрытая задача обязана иметь `Proof:` строку под ней.