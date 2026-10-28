# v2 как эталонный релиз — план

## Phase Contract

Inputs: `spec.md`, текущий v2-код, web UI, Android-клиент.
Outputs: план cutover 2.0.0, uTLS + stealth, DPI-гейт и Android-переход.

## Цель

Выпустить 2.0.0 как v2-only продукт: один `kvn-web`, один v2-конфиг, только v2-бинари, браузерный TLS/HTTP2-профиль по умолчанию и Android на том же v2-ядре. v1 остаётся только в старых GitHub Releases и документации.

## MVP Slice

- v2-only desktop: `kvn-web`, `kvn-client`, `kvn-server`; v1 не собирается.
- `~/.config/kvn/config.yaml`; UI не показывает v1-поля.
- uTLS-профили Chrome/Firefox/Safari с выбором и fallback; stealth-заголовки/путь/fallback по умолчанию.
- DPI/active-probe автоматический прогон + один полевой регион.
- Закрывает `AC-001`–`AC-006`, `AC-008`–`AC-012`; Android (`AC-007`) идёт следующим инкрементом в том же релизе.

## First Validation Path

1. `kvn-web` на чистом XDG-конфиге: список пуст, создаётся v2-сервер, `~/.config/kvn/config.yaml` появляется.
2. Подключение через TUN, YouTube/скачивание, в логах uTLS-профиль и отсутствие v1-полей.
3. `scripts/dpi-check.sh` + active probe: fallback и отсутствие KVN-маркеров.
4. Android smoke: TUN + v2 + сохранённый DNS routing.

## Scope

- v2-only web UI/backend/config и release-артефакты.
- TLS-стек клиента: uTLS, профили, fallback; stealth surface v2.
- DPI/active-probe гейт и отчёт.
- Удаление v1-кода/entrypoint'ов из текущего дерева.
- Android v2 data plane и сохранение DNS routing.
- Windows installer: WebView desktop по умолчанию и cleanup v1.
- Android UX/QR: AppPicker, импорт настроек, читаемые Settings, QR interoperability.
- Docs/REPOSITORY_MAP/CONTEXT.

## Performance Budget

- v2 TUN: не хуже текущего v2 на том же сервере/сети; допуск на uTLS + stealth ≤ 10% по throughput.
- Server RSS при 512 streams: без роста более чем на 15% от текущего v2.
- Android: AAR/APK не должны расти более чем на 20% без явного решения; cold connect ≤ 5 с на LTE.
- Padding: adaptive padding не должен снижать throughput более чем на 10% в default-профиле; cover-traffic — только opt-in.
- `none` для DPI-скрипта по latency; важна воспроизводимость.

## Implementation Surfaces

- `src/cmd/web/`, `src/internal/webui/` — единственный v2 web entrypoint, v2-only UI и `config.yaml`.
- `src/internal/config/` — `ClientConfig`/webui schema: uTLS + stealth defaults, v2-only validation.
- `src/internal/transport/` — HTTP2 stream path + headers, uTLS dialer, fallback.
- `src/internal/transport/tls/` или новый `src/internal/tlsprofile/` — несколько uTLS-профилей.
- `src/internal/client/`, `src/internal/tcpstack/` — desktop v2 data plane; mobile-friendly API boundary.
- `src/android/` — VpnService + Go mobile core или выбранный fallback; DNS routing остаётся.
- `scripts/` (build/install/release), `scripts/install-web.ps1`, `scripts/dpi-check.sh`, CI.
- `src/android/app/src/main/kotlin/com/kvn/client/ui/` — Settings/AppPicker/QR; `src/internal/transport/icmp.go` — ICMP echo proxy.
- `docs/ru|en/`, `REPOSITORY_MAP.md`, `src/*/CONTEXT.md`.
- Удаляемые v1 surfaces: `src/cmd/{client,server,relay,gatetest,stability}`, `src/internal/bootstrap/{client,server,relay}`, `src/internal/{tunnel,protocol,session,nat,acl,admin,ratelimit,metrics}`, v1 transport/WS/QUIC/framing, v1 Android engine.

## Bootstrapping Surfaces

- Новый пакет TLS-профилей с интерфейсом `Dialer` и тестами ClientHello/ALPN.
- Общий контракт v2 data plane для desktop и Android (TUN fd in/out, config, lifecycle).
- v2-only config store и миграция существующих v2-записей.

## Влияние на архитектуру

- v1 перестаёт быть отдельным runtime; общие пакеты (config/routing/DNS/TUN/proxy) остаются, v1-only вырезаются.
- Web UI становится v2-only; `kvn-web-v2` не поставляется.
- TLS-слой получает профилировщик, но сохраняет fallback на стандартный TLS.
- Android получает v2 data plane; Kotlin UI/VpnService остаются, транспорт v1 удаляется.
- Меняется release-контракт: 2.0.0 не совместим с v1 по wire; старые v1-артефакты — только Releases.

## Acceptance Approach

- AC-001 → build/release script + smoke `kvn-web`: только v2-артефакты.
- AC-002 → frontend/backend тест: v2-only поля, нет protocol/transport/obfuscation/stealth-v1.
- AC-003 → `TestV2StoreIsolation`: v2 пишет только `config.yaml`.
- AC-004 → `git ls-files`/build output: v1 entrypoint'ы отсутствуют.
- AC-005 → `TestV2StoreMigratesOnlyV2Entries`.
- AC-006 → docs/release check: ссылки на v1 Releases, нет v1-бинарей.
- AC-007 → Android e2e/smoke: TUN + v2 + DNS routing.
- AC-008 → docs diff: REPOSITORY_MAP/CONTEXT v2-only.
- AC-009 → `scripts/dpi-check.sh` + active-probe отчёт, сеть/регион.
- AC-010 → unit/integration ClientHello для 3 профилей + fallback.
- AC-011 → перехват headers/path, adaptive padding (small/control/QUIC), active-probe fallback; default config.
- AC-012 → PowerShell parser + Windows smoke: default desktop, task replacement, v1 cleanup; `-Browser` legacy.

## Данные и контракты

- Data model: меняется v2 config schema (uTLS profile, stealth defaults, stream path + header profile); persistent model сервера/routing не меняется.
- v2 config: `~/.config/kvn/config.yaml`; v1 `config.yaml` read-only для миграции.
- HTTP2 surface: путь и target-заголовок становятся профилируемыми; сервер временно принимает старый и новый варианты, чтобы не ломать тестовых клиентов.
- API: `/api/platform` добавляет `v2_only`; `/api/config`/`/api/servers` работают только с v2 store.
- Android: JSON-конфиг v2 и TUN fd contract между Kotlin и Go core.

## Стратегия реализации

- DEC-001 Сначала desktop v2-only, затем Android
  Why: desktop-путь уже работает и даёт быстрый DPI/uTLS-фидбек; Android — самый рискованный кусок.
  Tradeoff: 2.0.0 не выходит, пока Android не готов; desktop можно тестировать раньше.
  Affects: `src/cmd/web`, `src/internal/webui`, `src/android`.
  Validation: desktop smoke + Android spike gate.

- DEC-002 `kvn-web` — единственный web-бинарь, без `kvn-web-v2`
  Why: пользователь не должен выбирать между двумя UI; v2 — эталон.
  Tradeoff: переходный алиас не сохраняется; документация должна явно объяснить замену.
  Affects: `src/cmd/web`, `scripts/build.sh`, install/docs.
  Validation: release artifact list содержит только `kvn-web`.

- DEC-003 uTLS: несколько профилей, один default, fallback
  Why: ClientHello — главный отпечаток; несколько профилей покрывают разные сети.
  Tradeoff: поддержка профилей и тестов; не все профили совместимы с HTTP2/ALPN.
  Affects: TLS profile package, v2 transport client, Android core.
  Validation: ClientHello/ALPN тесты для Chrome/Firefox/Safari + fallback.

- DEC-004 Stealth по умолчанию через браузерный HTTP2 surface
  Why: v1 padding/WS-мимикрия не переносится на HTTP2; нужно убрать KVN-пути/заголовки.
  Tradeoff: меняется stream path + target header; сервер должен支持 dual-accept на переходный период.
  Affects: `src/internal/transport`, config defaults, fallback handler.
  Validation: перехват запросов, active probe, DPI-отчёт.

- DEC-005 Android: общий Go v2 data plane через gomobile, с fallback-планом
  Why: переиспользует gVisor/TCP split и не требует писать TCP-стек на Kotlin.
  Tradeoff: размер AAR, сборка/отладка gomobile, Android lifecycle/fd.
  Affects: `src/internal`, `src/android`, build scripts.
  Validation: spike на устройстве/эмуляторе; если gomobile+gVisor не проходит — fallback на tun2socks.

- DEC-006 DPI-гейт двухуровневый
  Why: локальный CI детерминирован, реальный DPI — только поле.
  Tradeoff: релиз зависит от доступности целевой сети; без поля нельзя заявлять обход DPI.
  Affects: `scripts/dpi-check.sh`, CI, release checklist.
  Validation: сохранённый отчёт с сетью/регионом.

- DEC-008 Не слать случайный мусор на другие endpoints по умолчанию
  Why: случайные пути/хосты — аномалия для IDS/active-probe, создают лишний трафик и не маскируют TLS/H2-метаданные; обычный браузер так себя не ведёт.
  Tradeoff: отказываемся от дешёвого «шума»; если нужен cover traffic — только opt-in, с реалистичными destinations и rate limit.
  Affects: v2 client, config, DPI tests, docs.
  Validation: default-конфиг не генерирует запросы вне v2 endpoint; DPI/IDS отчёт без scan-паттернов.

- DEC-007 HTTP2 fingerprint + adaptive padding, не постоянный cover traffic
  Why: uTLS скрывает ClientHello, но Go HTTP2 SETTINGS/priority/header order и размеры пакетов остаются отпечатком; padding помогает только traffic-analysis, а не TLS-метаданным.
  Tradeoff: браузерный H2-профиль и padding требуют своего транспорта/тестов; aggressive padding/cover traffic режет скорость и батарею.
  Affects: v2 transport, TLS profile package, config defaults, DPI tests.
  Validation: перехват SETTINGS/header order, packet-size distribution, DPI-отчёт; default throughput в бюджете.

## Incremental Delivery

### MVP (Первая ценность)

- v2-only config/UI; удаление v1 из web/build.
- uTLS + stealth defaults + DPI script.
- Desktop smoke и `AC-001`–`AC-006`, `AC-008`–`AC-011`.

### Итеративное расширение

- Android spike → v2 TUN/TCP + DNS routing (`AC-007`).
- Android QUIC-UDP только после подтверждения необходимости; HTTP2 UDP fallback уже покрывает MVP.
- Windows installer: WebView default, замена `kvn-web` task и cleanup v1.
- Android UX/QR: AppPicker/import, читаемые Settings, QR/JSON round-trip.
- Финальный release 2.0.0 и docs/REPOSITORY_MAP/CONTEXT.

## Порядок реализации

1. Зафиксировать v2-only web/config и удалить `kvn-web-v2`/v1 UI.
2. Вынести TLS-профили, включить uTLS + stealth defaults, расширить DPI-скрипт.
3. Удалить v1 entrypoints/пакеты, обновить build/install/release.
4. Android spike; при успехе — Go mobile core и DNS routing.
5. Windows installer/Android UX/QR: desktop default, v1 cleanup, AppPicker и QR parity.
6. Финальный DPI-прогон, docs, REPOSITORY_MAP/CONTEXT, 2.0.0.

## Риски

- uTLS ломает HTTP2/ALPN или проверку сертификата.
  Mitigation: fallback на стандартный TLS, тесты ClientHello + реальный connect.
- Stealth path + header меняет контракт и ломает старых тестовых клиентов.
  Mitigation: dual-accept на сервере, feature flag на один релиз.
- Удаление v1 ломает пользователей.
  Mitigation: v1.4.1 остаётся в Releases, docs с инструкцией; 2.0.0 — major.
- Android gomobile+gVisor не соберётся/будет слишком большим.
  Mitigation: spike с критериями; fallback tun2socks/иная упаковка.
- Android DNS routing регрессирует.
  Mitigation: сохранить существующие FakeDns/DirectDeliverer тесты, e2e.
- DPI-полевой регион недоступен.
  Mitigation: автоматический гейт обязателен; без поля — только заявление об отсутствии отпечатков.
- Padding не решает TLS/H2-метаданные и может ухудшить скорость.
  Mitigation: adaptive padding только для малых/control/QUIC-пакетов, cover traffic opt-in, throughput-бюджет.
- HTTP2-фингерprint Go остаётся отличимым от браузера.
  Mitigation: spike на SETTINGS/priority/header order; если нельзя — явно документировать остаточный риск.
- Случайный/cover-трафик выглядит как сканирование и привлекает IDS.
  Mitigation: default — только v2 endpoint; cover traffic opt-in и реалистичный.
- Windows upgrade блокирует exe/старый task или оставляет v1.
  Mitigation: остановить процессы/снять tasks, очистить legacy paths/PATH, заменить task и запустить заново; проверить PowerShell parser + Windows smoke.
- Android Settings/QR расходятся с web-конфигом.
  Mitigation: QR/JSON round-trip tests, единые endpoint/TLS/routing поля, AppPicker counts без вывода сырых списков.

## Rollout и compatibility

- 2.0.0 — major; v1 wire не поддерживается.
- v1-бинари/конфиги не входят; v1.4.1 доступен в Releases.
- v2 config отдельный; миграция только v2-записей.
- Feature flag `legacy stream path + header` на сервере на один релиз.
- Monitoring: логи uTLS profile/fallback, DPI-отчёт, Android crash/connect rate.

## Проверка

- Go: unit/integration для config store, TLS profiles, transport, v2 client/server.
- UI: frontend build + API tests; ручная проверка v2-only DOM.
- Scripts: `dpi-check.sh`, release artifact list, `git ls-files` v1 guard.
- Android: unit + instrumentation/e2e TUN/DNS routing.
- Docs: REPOSITORY_MAP/CONTEXT link check.
- Manual: YouTube/20MB download, active probe, fallback page.

## Соответствие конституции

- нет конфликтов; v1 удаляется как отдельный runtime, общие пакеты сохраняются.
