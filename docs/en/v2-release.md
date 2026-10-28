# v2 2.0.0 — release checklist

- [x] v1 runtime removed; `kvn-client`, `kvn-server`, `kvn-web` are v2-only.
- [x] `kvn-web` v2-only, unified `~/.config/kvn/config.yaml`, v1 → `.v1.bak`.
- [x] Unique TUN IP per `client_id`, persisted restart-safe lease.
- [x] uTLS Chrome/Firefox/Safari + stealth/padding.
- [x] DPI lab run: `docs/en/v2-dpi-report.md` (PASS).
- [ ] Field DPI run in RU against the new server — release sign-off.
- [x] Android `kvn-v2-client` (`com.kvn.v2client`), v2 VpnService, AAR.
- [x] Android exclude-domain/direct TCP/UDP (`.ru`).
- [ ] Android CI AAR build/upload — before public APK.
- [x] REPOSITORY_MAP/CONTEXT updated.
- [x] CHANGELOG 2.0.0.

Artifacts: `bin/kvn-client`, `bin/kvn-server`, `bin/kvn-web`, `bin/kvn-desktop`, `bin/kvn-v2.aar`, Android APK.
