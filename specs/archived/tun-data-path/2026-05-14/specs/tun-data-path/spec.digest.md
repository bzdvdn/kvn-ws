AC-001: TUN write error fixed — `invalid offset` устранён
AC-002: TUN read returns valid packet data — forwarding loops работают
AC-003: Data packet forwarded end-to-end — ping/ICMP проходит через туннель (требует NAT)
AC-004: Session cleanup on disconnect — goroutine leak устранён
AC-005: nftables установлен в Docker runtime
AC-006: NAT MASQUERADE работает — ping до gateway проходит
AC-007: Dockerfile production-ready — nftables + iproute2, образ < 50MB
AC-008: Smoke test — `bash examples/run.sh` → SUCCESS
