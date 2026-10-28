# v2 2.0.0 — release checklist

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
