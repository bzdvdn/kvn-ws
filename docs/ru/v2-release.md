# v2 — release checklist

## 2.0.2 — Android network fixes

- [x] Android: устранены зависания сети (forward lifecycle, keepalive/health, async direct DNS, bounded TUN stop, ctx propagation).
- [x] Android: устойчивость жизненного цикла (default-network callback, unlock health-gate, bounded QUIC, DNS из конфига, restart gate, cleanup).
- [x] CI-эквивалент локально: `go test -race ./src/...`, `go vet`, `gosec`, `golangci-lint`, `check-v2-only.sh`, `build.sh both`, AAR/APK.
- [x] `versionName` 2.0.2, CHANGELOG 2.0.2.
- [ ] Тег `v2.0.2` и проверка release workflow (нужны Android signing secrets).

## 2.0.0

- [x] v1 runtime удалён; `kvn-client`, `kvn-server`, `kvn-web` — только v2.
- [x] `kvn-web` v2-only, единый `~/.config/kvn/config.yaml`, v1 → `.v1.bak`.
- [x] Уникальный TUN IP на `client_id`, persisted lease, restart-safe.
- [x] uTLS Chrome/Firefox/Safari + stealth/padding.
- [x] DPI lab-прогон: `docs/ru/v2-dpi-report.md` (PASS).
- [x] Полевой DPI-прогон в сети тестера (RU, точный ISP не зафиксирован).
- [x] Android `kvn-v2-client` (`com.kvn.v2client`), v2 VpnService, AAR.
- [x] Android exclude-домены/direct TCP/UDP (`.ru`).
- [x] Android CI AAR build/upload.
- [x] REPOSITORY_MAP/CONTEXT обновлены.
- [x] CHANGELOG 2.0.0.

Артефакты: `bin/kvn-client`, `bin/kvn-server`, `bin/kvn-web`, `bin/kvn-desktop`, `bin/kvn-v2.aar`, Android APK.

## Release candidate

- [ ] Настроить GitHub secrets `ANDROID_KEYSTORE_BASE64`, `ANDROID_KEYSTORE_PASSWORD`, `ANDROID_KEY_ALIAS`, `ANDROID_KEY_PASSWORD` — иначе tag/release job не соберёт обновляемый APK.
- [ ] Собрать `v2.0.0-rc.1` и проверить Linux/Windows/Android артефакты.
- [ ] Развернуть 2+ сервера, уникальные token, проверить `GET /` session + TUN lease.
- [ ] Проверить переключение серверов в Android/kvn-web.
- [ ] Раздать APK тестерам и собрать отчёты по [инструкции тестера](v2-testers.md).
