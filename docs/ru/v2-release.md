# v2 2.0.0 — release checklist

- [x] v1 runtime удалён; `kvn-client`, `kvn-server`, `kvn-web` — только v2.
- [x] `kvn-web` v2-only, единый `~/.config/kvn/config.yaml`, v1 → `.v1.bak`.
- [x] Уникальный TUN IP на `client_id`, persisted lease, restart-safe.
- [x] uTLS Chrome/Firefox/Safari + stealth/padding.
- [x] DPI lab-прогон: `docs/ru/v2-dpi-report.md` (PASS).
- [ ] Полевой DPI-прогон в RU на новом сервере — release sign-off.
- [x] Android `kvn-v2-client` (`com.kvn.v2client`), v2 VpnService, AAR.
- [x] Android exclude-домены/direct TCP/UDP (`.ru`).
- [ ] Android CI AAR build/upload — перед публичным APK.
- [x] REPOSITORY_MAP/CONTEXT обновлены.
- [x] CHANGELOG 2.0.0.

Артефакты: `bin/kvn-client`, `bin/kvn-server`, `bin/kvn-web`, `bin/kvn-desktop`, `bin/kvn-v2.aar`, Android APK.
