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

## 2.1 — Network hardening

### ECH (Encrypted Client Hello) — P0

Скрыть настоящий SNI от пассивного DPI.

- `tls.ech.enabled`, `public_name`, `config_file`/DNS HTTPS RR discovery.
- Серверный `ech_keys_file`, TLS 1.3 only.
- Проброс `EncryptedClientHelloConfigList` через uTLS; fallback при ECH rejection.
- Внешний SNI должен быть правдоподобным доменом; IP сервера всё равно виден.
- Тесты: ECH accepted/rejected, внешний SNI, fallback, Android.

Ограничение: ECH не скрывает IP/порт и может сам блокироваться DPI.

### Entry relay / two-hop — P1

Entry-сервер в сети, где DPI не блокирует, forwarding на exit-сервер за рубежом.

- **L4 passthrough — первый шаг:** TCP 443 + UDP 443 на entry → exit; TLS остаётся end-to-end.
  - `socat`/HAProxy/nginx stream или nftables DNAT.
  - Клиент ходит на entry, exit-сервер не светится напрямую.
  - DPI всё равно видит entry IP и настоящий SNI exit-домена; ECH нужен отдельно.
  - QUIC требует UDP-проброс, иначе fallback на HTTP/2.
- **v2-native relay — второй шаг:** entry терминирует v2 TLS/H2, проверяет token и открывает upstream v2 к exit.
  - Можно использовать entry-домен/сертификат и скрыть exit IP.
  - Нужны stream mapping, auth на entry, лимиты и health/rollback.
  - Добавляет RTT и второй hop; не делать default без тестов.
- **Метрики:** entry/exit latency, bytes, errors, QUIC availability.
- **Тесты:** TCP/UDP passthrough, QUIC, reconnect, auth, fallback, throughput.

Ограничение: relay не скрывает SNI сам по себе и не заменяет ECH/CDN.

### Cover traffic — P2

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
- Автоматический выбор сервера по latency/health.
- Cover traffic вне активного туннеля — не планируется.
