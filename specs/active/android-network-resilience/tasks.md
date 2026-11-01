# Android: устойчивость сетевого жизненного цикла — Задачи

## Phase Contract

Inputs: `spec.md` (express-режим, plan отсутствует).
Outputs: упорядоченные задачи с `Touches:` и покрытием AC.
Stop if: задача не имеет измеримого outcome или coverage не сходится.

## Surface Map

| Surface | Tasks |
|---------|-------|
| src/android/app/src/main/kotlin/com/kvn/client/vpn/VpnPolicy.kt | T1.1, T2.1, T2.3, T3.1 |
| src/android/app/src/main/kotlin/com/kvn/client/vpn/KvnV2VpnService.kt | T1.1, T2.1, T2.3, T3.1 |
| src/android/app/src/test/java/com/kvn/client/vpn/VpnPolicyTest.kt | T1.2, T2.1, T2.3, T3.2 |
| src/internal/transport/quic.go | T2.2 |
| src/internal/transport/quic_test.go | T2.2 |
| src/internal/transport/client.go | T4.1 |
| src/internal/transport/transport_test.go | T4.1 |
| src/internal/client/client.go | T2.2, T4.1, T4.2 |
| src/internal/client/ping.go | T4.2 |
| src/internal/client/ping_test.go | T4.2 |

## Implementation Context

- Цель MVP: убрать reconnect-шторм при нескольких сетях (AC-001) и bounded QUIC-старт (AC-003).
- Инварианты/семантика:
  - Активная underlying-сеть — default network; повторный `onAvailable` без смены default не рестартит.
  - Каждая регистрация callback начинает с baseline: первый `onAvailable` не считается изменением.
  - Unlock пересоздаёт сессию только при неуспешном `health()`.
  - QUIC: `HandshakeIdleTimeout=3s` + внешний ctx ≤5s; при ошибке — HTTP/2 UDP.
  - DNS: серверы берутся из `dns_proxy.upstreams`; собственный пакет всегда disallowed в VPN.
- Контракты/протокол: `registerDefaultNetworkCallback`; `builder.addDnsServer(host)`; `transport.Client.Close()` → `CloseIdleConnections`.
- Proof signals: Kotlin unit-тесты `VpnPolicyTest`; Go-тесты `TestNewQUICClientBoundedOnBlackhole`, `TestClientCloseIdleConnections`, `TestPingConcurrencyBounded`.
- Вне scope: легаси-Kotlin транспорт, серверный протокол, split-tunnel сверх исключения своего пакета, latency-проба.

## Фаза 1: Network lifecycle (MVP)

Цель: переключение сетей не вызывает reconnect-шторм.

- [x] T1.1 Ввести `NetworkTracker` (baseline per registration, default-network switch) и перевести сервис на `registerDefaultNetworkCallback`; рестарт только при фактической смене active. Outcome: два onAvailable подряд не рестартят; смена default — один рестарт. Touches: src/android/app/src/main/kotlin/com/kvn/client/vpn/VpnPolicy.kt, src/android/app/src/main/kotlin/com/kvn/client/vpn/KvnV2VpnService.kt
      Proof: code src/android/app/src/main/kotlin/com/kvn/client/vpn/VpnPolicy.kt NetworkTracker
- [x] T1.2 Unit-тесты `NetworkTracker`: baseline, duplicate, switch, reset. Outcome: решения покрыты тестами. Touches: src/android/app/src/test/java/com/kvn/client/vpn/VpnPolicyTest.kt
      Proof: test src/android/app/src/test/java/com/kvn/client/vpn/VpnPolicyTest.kt NetworkTrackerTest

## Фаза 2: Unlock, QUIC, DNS

Цель: unlock не рвёт здоровую сессию, QUIC-старт ограничен, DNS применяется.

- [x] T2.1 Unlock по health: `ACTION_USER_PRESENT` запускает `health()` и рестартит только при ошибке; политика вынесена и покрыта тестом. Outcome: здоровая сессия не пересоздаётся. Touches: src/android/app/src/main/kotlin/com/kvn/client/vpn/VpnPolicy.kt, src/android/app/src/main/kotlin/com/kvn/client/vpn/KvnV2VpnService.kt, src/android/app/src/test/java/com/kvn/client/vpn/VpnPolicyTest.kt
      Proof: code src/android/app/src/main/kotlin/com/kvn/client/vpn/KvnV2VpnService.kt screenReceiver
- [x] T2.2 QUIC bounded: `HandshakeIdleTimeout=3s` и внешний ctx ≤5s в `RunTUNDevice`/`runTUN`; тест с blackhole UDP. Outcome: fallback на HTTP/2 UDP ≤5s. Touches: src/internal/transport/quic.go, src/internal/transport/quic_test.go, src/internal/client/client.go
      Proof: test src/internal/transport/quic_test.go TestNewQUICClientBoundedOnBlackhole
- [x] T2.3 DNS из конфига: парсер `dns_proxy.upstreams` (host:port → host) и применение в VpnService; собственный пакет всегда disallowed. Outcome: пользовательские DNS объявляются, direct-резолв не замыкается. Touches: src/android/app/src/main/kotlin/com/kvn/client/vpn/VpnPolicy.kt, src/android/app/src/main/kotlin/com/kvn/client/vpn/KvnV2VpnService.kt, src/android/app/src/test/java/com/kvn/client/vpn/VpnPolicyTest.kt
      Proof: code src/android/app/src/main/kotlin/com/kvn/client/vpn/VpnPolicy.kt VpnConfigParser

## Фаза 3: Lifecycle hardening

Цель: новый конфиг применяется без зависания и без параллельных рестартов.

- [x] T3.1 `RestartGate` (single-flight) вместо `restarting`; при `client != null` и изменившемся конфиге — рестарт, иначе эмит текущего состояния; `stopV2` сбрасывает конфиг. Outcome: нет «вечного Connecting» и двойных рестартов. Touches: src/android/app/src/main/kotlin/com/kvn/client/vpn/VpnPolicy.kt, src/android/app/src/main/kotlin/com/kvn/client/vpn/KvnV2VpnService.kt
      Proof: code src/android/app/src/main/kotlin/com/kvn/client/vpn/VpnPolicy.kt RestartGate
- [x] T3.2 Тест `RestartGate`: конкурентные `tryEnter` дают ровно одного победителя, `exit` разрешает повторный вход. Outcome: single-flight подтверждён. Touches: src/android/app/src/test/java/com/kvn/client/vpn/VpnPolicyTest.kt
      Proof: test src/android/app/src/test/java/com/kvn/client/vpn/VpnPolicyTest.kt RestartGateTest

## Фаза 4: Resource cleanup

Цель: завершение сессии освобождает транспортные ресурсы.

- [x] T4.1 `transport.Client.Close()` (закрыть idle-соединения) и вызов на выходе сессии; тест, что сервер видит закрытие соединения. Outcome: нет накопления idle h2-соединений. Touches: src/internal/transport/client.go, src/internal/transport/transport_test.go, src/internal/client/client.go
      Proof: code src/internal/transport/client.go Close
- [x] T4.2 Ограничить параллельные ICMP-пинги семафором; тест лимита. Outcome: число одновременных пингов ограничено. Touches: src/internal/client/client.go, src/internal/client/ping.go, src/internal/client/ping_test.go
      Proof: test src/internal/client/ping_test.go TestPingConcurrencyBounded

## Покрытие критериев приемки

- AC-001 -> T1.1, T1.2
- AC-002 -> T2.1
- AC-003 -> T2.2
- AC-004 -> T2.3
- AC-005 -> T3.1, T3.2
- AC-006 -> T4.1, T4.2

## Заметки

- Kotlin-сборка: `./gradlew :app:compileDebugKotlin`, тесты — `:app:testDebugUnitTest`.
- Go: `go test ./src/internal/client ./src/internal/transport`.
- AAR пересобирать не требуется: Go-изменения не меняют gomobile API.
