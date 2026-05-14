# Tun Data Path — Задачи

## Phase Contract

Inputs: plan.md, spec.md.
Outputs: исполнимые задачи с Touches: и покрытием AC.
Stop if: нет — plan полный.

## Implementation Context

- DEC-001: Write offset=12 (virtioNetHdrLen) — TUN с IFF_VNET_HDR требует 12 байт virtio header
- DEC-002: Single-buf Read — убираем batch-коллизию shared buf
- DEC-003: MockTunDevice — unit-тесты без /dev/net/tun
- DEC-004: SetIP фикс — `ip addr add <ip>/<mask>` вместо `<subnet>/<mask>`
- Buffer headroom: writeHeadroom=12 (virtioNetHdrLen), read буфер 1500
- Размер образа: alpine + nftables + iproute2 = 40.5MB
- DEC-004: alpine + nftables + iproute2 — runtime образ для Docker
- DEC-005: privileged + /dev/net/tun — compose конфиг для TUN + NAT

## Surface Map

| Surface | Tasks |
|---------|-------|
| src/internal/tun/tun_test.go | T1.1 |
| src/internal/tun/tun.go | T2.1, T2.2 |
| src/cmd/client/main.go | T3.1 |
| src/cmd/server/main.go | T3.2 |
| src/internal/tun/ | T4.1 |
| Dockerfile | T5.1, T5.2, T5.3 |
| docker-compose.yml | T5.3 |
| examples/* | T6.2, T6.3 |

## Фаза 1: MockTunDevice

- [x] T1.1 Создать `tun_test.go` с `MockTunDevice`, реализующим `TunDevice` interface: хранение пакетов в памяти, `Read` забирает из очереди, `Write` добавляет в очередь, `Open`/`Close` no-op. Touches: src/internal/tun/tun_test.go

## Фаза 2: TUN Read/Write fix (MVP)

- [x] T2.1 Исправить `tunDevice.Read()`: убрать batch, читать одиночный пакет через `t.device.Read([][]byte{buf}, []int{0}, 0)`. Touches: src/internal/tun/tun.go
- [x] T2.2 Исправить `tunDevice.Write()`: аллоцировать `padded[4+pktLen]`, копировать пакет в `padded[4:]`, передавать в `t.device.Write([][]byte{padded}, 4)` с offset 4. Touches: src/internal/tun/tun.go

## Фаза 3: Forwarding loops

- [x] T3.1 Проверено: `tunDevice.Write()` сам управляет headroom, callers не требуют изменений. `tunToWS` использует буфер 1500 — достаточно для MTU. Touches: src/internal/tun/tun.go (не client/main.go)
- [x] T3.2 Проверено: `serverWSToTun` вызывает `dev.Write(f.Payload)` — headroom внутри tunDevice.Write. Touches: src/internal/tun/tun.go (не server/main.go)

## Фаза 4: Проверка

- [x] T4.1 Unit-тесты на MockTunDevice (7 тестов). `go test -race ./src/internal/tun/` — PASS. Touches: src/internal/tun/tun_test.go

## Фаза 5: Docker runtime — nftables + Dockerfile

- [x] T5.1 Добавить `nftables` в `apk add` в Dockerfile runtime stage. `which nft` → `/usr/sbin/nft`. Touches: Dockerfile
- [x] T5.2 Размер образа `go_kvn-server:latest` = 40.5MB < 50MB. Touches: Dockerfile
- [x] T5.3 `docker compose up` — сервер запускается без nftables warnings. Touches: Dockerfile, docker-compose.yml

## Фаза 6: Сквозной ping + verify

- [x] T6.1 `docker exec go_kvn-client-1 ping -c 2 10.10.0.1` — 0% loss. Touches: (изоляция: ping внутри контейнера, хост не трогает)
- [x] T6.2 examples/ — уже обновлены (Dockerfile с nftables, compose с privileged, конфиги с allow_empty и wss). Touches: examples/*.yml, examples/*.yaml
- [x] T6.3 `bash examples/run.sh` — SUCCESS (проверено ранее, изоляция: Docker-контейнеры). Touches: examples/run.sh

## Покрытие критериев приемки

- AC-001 (Write error fixed) → T2.2, T3.1, T3.2
- AC-002 (Read valid packets) → T2.1, T3.1
- AC-003 (Ping through tunnel) → T6.1
- AC-004 (Session cleanup) → T4.1
- AC-005 (nftables installed) → T5.1
- AC-006 (NAT works) → T6.1
- AC-007 (Dockerfile production) → T5.2
- AC-008 (Smoke test) → T6.3

## Заметки

- Фазы строго последовательные: T1 → T2 → T3 → T4 → T5 → T6
- T5 и T6 требуют Docker Engine на машине выполнения
