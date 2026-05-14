## Goal

Починить data path от TUN I/O до NAT в Docker — сквозной ping через туннель.

## Acceptance Criteria

| ID | Description | Verification |
|----|-------------|-------------|
| AC-001 | TUN write error fixed | Client log без `invalid offset` |
| AC-002 | TUN read returns valid packets | go test на mock TUN проходит |
| AC-003 | Ping через туннель | `ping -c 1 10.10.0.1` → 0% loss |
| AC-004 | Session cleanup | `go test -race` без data race |
| AC-005 | nftables in Docker | `which nft` в контейнере |
| AC-006 | NAT MASQUERADE works | Ping проходит через NAT |
| AC-007 | Production Dockerfile | ip + nft, образ < 50MB |
| AC-008 | Smoke test | run.sh → SUCCESS |

## Out of Scope

- IPv6 data path
- CI/CD pipeline
- Performance tuning
