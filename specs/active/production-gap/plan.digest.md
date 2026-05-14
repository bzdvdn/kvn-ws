DEC-001: client TLS trust задаётся явной config-surface без insecure default
DEC-002: mTLS semantics приводятся к явным режимам request/require/verify
DEC-003: operational endpoints используют shared token gate без новой подсистемы auth
DEC-004: release proof собирается через существующие Speckeep и test surfaces
Surfaces:
- client-tls: src/internal/config/client.go, src/internal/transport/tls/tls.go, src/cmd/client/main.go, configs/client.yaml, examples/client.yaml
- server-mtls: src/internal/config/server.go, src/internal/transport/tls/tls.go, src/cmd/server/main.go, configs/server.yaml, examples/server.yaml
- examples-secrets: examples/docker-compose.yml, examples/run.sh, examples/*.yaml, .gitignore
- operational-access: src/cmd/server/main.go, src/internal/admin/admin.go, src/internal/admin/admin_test.go, scripts/test-security.sh
- release-governance: specs/active/production-gap/tasks.md, specs/active/production-gap/verify.md, .speckeep/scripts/check-verify-ready.sh
- quality-proof: .github/workflows/ci.yml, scripts/test-security.sh, scripts/test-gate.sh
