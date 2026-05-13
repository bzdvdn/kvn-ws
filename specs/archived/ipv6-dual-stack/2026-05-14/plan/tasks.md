# IPv6 & Dual-Stack — Задачи

## Implementation Context

- **Цель MVP:** IPv6 пул + handshake + TUN + NAT → клиент получает fd00:: адрес, ping6 проходит (AC-001..004).
- **Инварианты:** IPv4-пул не меняется; старый клиент без IPv6 не ломается; wire format меняется (major bump).
- **Аллокация IPv6:** /112 подсеть (65534 адреса), random offset + bitmap, без линейного сканирования.
- **Handshake wire:** `SessionID(16) + FamilyByte(1) + IPLen(1) + IPBytes(n) × 2` (сначала IPv4, потом IPv6 опционально).
- **NAT:** nftables `add table ip6 kvn-nat`, `add chain ip6 kvn-nat postrouting`, `add rule ... masquerade`.
- **Packet parsing:** первый ниббл IP-заголовка определяет семейство (4=v4, 6=v6).
- **DNS:** `net.Resolver.LookupNetIP(ctx, "ip", domain)` для A + AAAA.
- **Границы scope:** не делаем DNS64/NAT64, IPv6-only транспорт, ICMPv6 PMTUD, DHCPv6/SLAAC.
- **Proof signals:** `ip -6 addr show dev kvn`, `ping6 -c 1 fd00::1`, `nft list table ip6 kvn-nat`.

## Surface Map

| Surface | Tasks |
|---------|-------|
| `src/internal/config/server.go` | T1.1 |
| `src/internal/config/client.go` | T1.1 |
| `src/internal/session/session.go` | T1.2, T2.1, T2.3 |
| `src/internal/session/bolt.go` | T1.2 |
| `src/internal/protocol/handshake/handshake.go` | T2.1 |
| `src/internal/tun/tun.go` | T2.2 |
| `src/internal/nat/nftables.go` | T2.3 |
| `src/cmd/server/main.go` | T2.3 |
| `src/cmd/client/main.go` | T2.2, T2.3, T3.3 |
| `src/internal/routing/router.go` | T3.1 |
| `src/internal/dns/resolver.go` | T3.2 |
| `src/internal/session/session_test.go` | T4.1 |
| `src/internal/protocol/handshake/handshake_test.go` | T4.1 |
| `src/internal/nat/nftables_test.go` | T4.1 |
| `src/internal/routing/router_test.go` | T4.1 |
| `docker-compose.test.yml`, `scripts/test-gate.sh` | T4.2 |

## Фаза 1: Основа

Цель: конфиг + структура IPv6 пула.

- [x] T1.1 Добавить `PoolIPv6 PoolCfg` в `NetworkCfg` сервера; клиентский `IPv6 bool` (уже есть, dead) включить в использование. Touches: src/internal/config/server.go, src/internal/config/client.go
- [x] T1.2 Реализовать `IPPool` для IPv6 с аллокацией через random offset в /112 подсети; добавить `NewBoltStore6` для IPv6-аллокаций. Touches: src/internal/session/session.go, src/internal/session/bolt.go

## Фаза 2: MVP Slice

Цель: core IPv6 connectivity — pool + handshake + TUN + NAT (AC-001..004).

- [x] T2.1 Расширить `ServerHello.Encode/Decode` на length-prefixed IP; протолкнуть `AssignedIPv6` через handshake (client посылает `ipv6: true`, server назначает из IPv6 пула). Touches: src/internal/protocol/handshake/handshake.go, src/internal/session/session.go
- [x] T2.2 Реализовать настройку IPv6 адреса на TUN-интерфейсе и интеграцию в клиентский main (чтение конфига `ipv6: true`, установка адреса, добавление маршрута). Touches: src/internal/tun/tun.go, src/cmd/client/main.go
- [x] T2.3 Добавить IPv6 NAT (nftables table `ip6 kvn-nat` masquerade) в `NFTManager`; инициализировать второй `IPPool` и `Session.AssignedIPv6` в серверном main. Touches: src/internal/nat/nftables.go, src/internal/session/session.go, src/cmd/server/main.go, src/cmd/client/main.go

## Фаза 3: Основная реализация

Цель: dual-stack routing, DNS AAAA, kill-switch (AC-005).

- [x] T3.1 Реализовать `parseDstIP6()` в router и диспетчеризацию по версии IP (первый ниббл). Touches: src/internal/routing/router.go
- [x] T3.2 Переключить DNS-резолвер с `"ip4"` на `"ip"` сеть (A + AAAA). Touches: src/internal/dns/resolver.go
- [x] T3.3 Добавить IPv6 kill-switch (nftables table `ip6 kvn-kill` с policy drop). Touches: src/cmd/client/main.go

## Фаза 4: Проверка

Цель: automated coverage + gate test.

- [x] T4.1 Написать unit-тесты: IPv6 pool аллокация/релиз, handshake encode/decode roundtrip, NAT setup, packet parser. Touches: src/internal/session/session_test.go, src/internal/protocol/handshake/handshake_test.go, src/internal/routing/routing_test.go
- [x] T4.2 Обновить gate test (docker-compose.test.yml + test-gate.sh) для dual-stack сценария: ping6 + nft assertions. Touches: docker-compose.test.yml, scripts/test-gate.sh

## Покрытие критериев приемки

- AC-001 -> T1.2, T2.2, T4.1
- AC-002 -> T1.2, T4.1
- AC-003 -> T2.3, T4.1
- AC-004 -> T2.1, T2.2, T2.3, T4.2
- AC-005 -> T3.1, T3.2, T4.1
