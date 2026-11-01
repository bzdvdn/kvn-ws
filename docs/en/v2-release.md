# v2 — release checklist

## 2.0.4 — Android UDP/QUIC reliability

- [x] QUIC keepalive + dead-connection fallback to the HTTP/2 UDP stream.
- [x] Large datagrams → HTTP/2 fallback (client/server), `MaxQUICPayload=1100`.
- [x] Drop IP fragments, TUN MTU 1280, larger queues.
- [x] CI-equivalent locally: `go test -race`, `go vet`, `gosec`, `golangci-lint`.
- [x] `versionName` 2.0.4, CHANGELOG 2.0.4.
- [ ] Tag `v2.0.4`, update **both client and server**.

## 2.0.3 — Android QUIC fix

- [x] QUIC datagram read loop no longer depends on the startup timeout; regression test `TestQUICClientSurvivesDialContextCancel`.
- [x] CI-equivalent locally: `go test -race`, `go vet`, `gosec`, `golangci-lint`.
- [x] `versionName` 2.0.3, CHANGELOG 2.0.3.
- [ ] Tag `v2.0.3` and release workflow check.

## 2.0.2 — Android network fixes

- [x] Android: network stalls fixed (forward lifecycle, keepalive/health, async direct DNS, bounded TUN stop, ctx propagation).
- [x] Android: lifecycle resilience (default-network callback, unlock health gate, bounded QUIC, DNS from config, restart gate, cleanup).
- [x] CI-equivalent locally: `go test -race ./src/...`, `go vet`, `gosec`, `golangci-lint`, `check-v2-only.sh`, `build.sh both`, AAR/APK.
- [x] `versionName` 2.0.2, CHANGELOG 2.0.2.
- [ ] Tag `v2.0.2` and release workflow check (Android signing secrets required).

## 2.0.0

- [x] v1 runtime removed; `kvn-client`, `kvn-server`, `kvn-web` are v2-only.
- [x] `kvn-web` v2-only, unified `~/.config/kvn/config.yaml`, v1 → `.v1.bak`.
- [x] Unique TUN IP per `client_id`, persisted restart-safe lease.
- [x] uTLS Chrome/Firefox/Safari + stealth/padding.
- [x] DPI lab run: `docs/en/v2-dpi-report.md` (PASS).
- [x] Field DPI run in the tester network (RU, exact ISP not recorded).
- [x] Android `kvn-v2-client` (`com.kvn.v2client`), v2 VpnService, AAR.
- [x] Android exclude-domain/direct TCP/UDP (`.ru`).
- [x] Android CI AAR build/upload.
- [x] REPOSITORY_MAP/CONTEXT updated.
- [x] CHANGELOG 2.0.0.

Artifacts: `bin/kvn-client`, `bin/kvn-server`, `bin/kvn-web`, `bin/kvn-desktop`, `bin/kvn-v2.aar`, Android APK.

## Release candidate

- [ ] Configure GitHub secrets `ANDROID_KEYSTORE_BASE64`, `ANDROID_KEYSTORE_PASSWORD`, `ANDROID_KEY_ALIAS`, `ANDROID_KEY_PASSWORD`; tag/release jobs require them for an updatable APK.
- [ ] Build `v2.0.0-rc.1` and verify Linux/Windows/Android artifacts.
- [ ] Deploy 2+ servers with unique tokens; verify `GET /` session + TUN lease.
- [ ] Verify server switching in Android/kvn-web.
- [ ] Distribute the APK to testers and collect reports via the [tester guide](v2-testers.md).
