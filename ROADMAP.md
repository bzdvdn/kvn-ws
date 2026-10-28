# Roadmap — kvn-ws 2.x

## 2.0.0 — release candidate

Цель: v2-only релиз для нескольких серверов и закрытой группы тестеров.

- [x] v2-only Go client/server/web, unified config, TUN leases.
- [x] Android v2 client, QR parity, Settings/AppPicker, direct DNS/routing.
- [x] uTLS Chrome/Firefox/Safari, adaptive padding, fallback.
- [x] ICMP echo proxy.
- [x] Windows WebView desktop, v1 cleanup, Linux/macOS install scripts.
- [ ] Release candidate `v2.0.0-rc.1`: stable Android signing key, multi-server smoke, tester APK.
- [ ] Final `v2.0.0`: после отчёта тестеров и решения по DPI-полю.

## 2.1 — DPI hardening

### ECH (Encrypted Client Hello) — P0

Скрыть настоящий SNI от пассивного DPI.

- `tls.ech.enabled`, `public_name`, `config_file`/DNS HTTPS RR discovery.
- Серверный `ech_keys_file`, TLS 1.3 only.
- Проброс `EncryptedClientHelloConfigList` через uTLS; fallback при ECH rejection.
- Внешний SNI должен быть правдоподобным доменом; IP сервера всё равно виден.
- Тесты: ECH accepted/rejected, внешний SNI, fallback, Android.

Ограничение: ECH не скрывает IP/порт и может сам блокироваться DPI.

### Cover traffic — P1

Убрать длинные паузы и удешевить traffic analysis.

- Opt-in idle H2 cover-stream **только до нашего сервера**; без случайных хостов.
- Jitter, payload distribution, `max_kbps`, `max_overhead_pct`.
- Остановка при реальном трафике; сервер эхо-возвращает, не делает dial.
- Android: off на metered/battery saver, отдельные счётчики.
- Тесты: бюджет, остановка, отсутствие метронома, overhead ≤5%.

Ограничение: не скрывает SNI/IP и не заменяет ECH/CDN.

### Дополнительно

- **Domain fronting/CDN — P2:** внешний SNI CDN, только если провайдер разрешает; не гарантируется.
- **H2 fingerprint parity — P2:** SETTINGS/priority/header order под Chrome.
- **Traffic shaping — P2:** burst-профили под HTTP/2/видео вместо ровного bitrate.
- **ECH + CDN — P2:** ECH за CDN, если CDN его поддерживает.

## 2.2 — Multi-server operations

- Fleet install/update: несколько серверов из одного release, уникальные token и `tun.subnet`.
- Health/session checks до подключения тестеров.
- Per-server QR и переключение без перезапуска.
- Метрики: connections, leases, RX/TX, errors; алерты на падение.
- Rollback: предыдущий binary/config, lease-file backup.

## Backlog

- iOS-клиент.
- Relay/terminator parity — только если появится запрос.
- Автоматический выбор сервера по latency/health.
- Cover traffic вне активного туннеля — не планируется.
