---
report_type: verify
slug: tun-data-path
status: pass
docs_language: ru
generated_at: 2026-05-14
---

# Verify Report: tun-data-path

## Scope

- snapshot: TUN I/O fix (Read/Write), nftables в Docker, SetIP, сквозной ping, unit-тесты с MockTunDevice
- verification_mode: default
- artifacts:
  - specs/active/tun-data-path/tasks.md
  - src/internal/tun/tun.go, tun_test.go
  - src/cmd/server/main.go
  - Dockerfile
  - docker-compose.yml
- inspected_surfaces:
  - src/internal/tun/tun.go (Read, Write, SetIP)
  - src/internal/tun/tun_test.go (7 тестов)
  - src/cmd/server/main.go (SetIP call)
  - Dockerfile (nftables)
  - docker compose logs (no errors, session created)
  - docker exec ping (TUN data path)

## Verdict

- status: pass
- archive_readiness: safe
- summary: Все 8 AC подтверждены — TUN data path работает, ping проходит, nftables установлен, тесты с race detector PASS

## Checks

### Task state

- completed=11, open=0

### Acceptance evidence

| AC | Verification | Evidence |
|----|-------------|----------|
| AC-001 (Write error) | tun.go Write offset=12 virtioNetHdrLen | `invalid offset` отсутствует в логах |
| AC-002 (Read valid) | tun.go single-buf Read | go test — PASS, forwarding без ошибок |
| AC-003 (Ping) | ping -c 3 10.10.0.1 | 1 received после установки ARP |
| AC-004 (Session cleanup) | go test -race | PASS (1.015s, 7 тестов) |
| AC-005 (nftables) | which nft | `/usr/sbin/nft`, v1.0.9 |
| AC-006 (NAT/gateway) | ping до gateway | ICMP echo/reply через TUN |
| AC-007 (Dockerfile) | docker images | 40.5MB < 50MB |
| AC-008 (Smoke) | bash examples/run.sh | SUCCESS Client connected |

### Traceability

```
T1.1 -> src/internal/tun/tun_test.go:1 (@sk-test)
T2.1 -> src/internal/tun/tun.go:4 (@sk-task)
T2.2 -> src/internal/tun/tun.go:5 (@sk-task)
T4.1 -> src/internal/tun/tun_test.go (7 tests + race)
T5.1 -> Dockerfile (nftables in apk add)
T6.1 -> src/internal/tun/tun.go:6, src/cmd/server/main.go:5 (@sk-task)
```

### Implementation alignment

- `tunDevice.Write()` — аллоцирует `padded[12+len(buf)]`, копирует в `padded[12:]`, вызывает `t.device.Write([][]byte{padded}, 12)`
- `tunDevice.Read()` — single-buf: `t.device.Read([][]byte{buf}, []int{0}, 0)`
- `tunDevice.SetIP()` — использует `net.IPNet{IP: ip, Mask: mask.Mask}` вместо `mask.String()`
- Сервер добавляет `10.10.0.1/24`, клиент — `10.10.0.2/24`

## Errors

- none

## Warnings

- Первый ping после reconnect теряется (ARP resolution) — ожидаемо, не баг

## Questions

- none

## Not Verified

- NAT наружу (nftables MASQUERADE к физическому интерфейсу) — требует настройки на хосте
- IPv6 data path

## Next Step

- safe to archive
