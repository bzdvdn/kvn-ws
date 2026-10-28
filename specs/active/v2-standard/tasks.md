# v2 как эталонный релиз — задачи

## Surface Map

| Surface | Tasks |
|---------|-------|
| src/cmd/web/ | T1.1, T3.1 |
| src/internal/webui/server.go | T1.1 |
| tmp config.yaml | T1.2, T3.1 |
| src/internal/webui/ | T1.1, T1.2, T2.5 |
| src/internal/webui/frontend/src/ | T2.5 |
| src/internal/config/ | T1.2, T2.2, T2.3 |
| src/internal/config/client.go, src/internal/config/v2.go | T1.2, T1.3, T2.2, T2.3 |
| src/internal/tlsprofile/ | T1.3, T2.1 |
| src/internal/tcpstack/ | T2.1, T4.2 |
| src/mobile/ | T4.1, T4.2 |
| src/internal/tlsprofile/ | T1.3, T2.1 |
| src/internal/transport/ | T2.1, T2.2, T2.3, T2.4 |
| src/internal/transport/padding.go | T2.3 |
| src/internal/transport/transport_test.go | T2.1, T2.2, T2.3, T2.4 |
| src/internal/client/ | T2.1, T2.2, T2.3 |
| scripts/dpi-check.sh | T1.4, T5.1 |
| scripts/build.sh, scripts/install-*.sh, Dockerfile | T3.2, T3.3 |
| src/cmd/{client,server,relay}, src/internal/{bootstrap,tunnel,protocol,session,nat,acl,admin,metrics}/ | T3.2 |
| src/android/ | T4.1, T4.2, T4.3, T4.4 |
| docs/ru/, docs/en/, REPOSITORY_MAP.md, src/*/CONTEXT.md | T3.4, T5.3 |
| .github/workflows/ci.yml | T3.3, T5.2 |

## Implementation Context

- Цель MVP: v2-only 2.0.0 — один `kvn-web`, `~/.config/kvn/config.yaml` (единый), uTLS + stealth defaults, v1 удалён, Android на v2.
- Инварианты: v1 `config.yaml` не читается/не пишется v2 (кроме одноразового импорта v2-записей); `kvn-web-v2` не поставляется; v1-код не собирается.
- Stealth: браузерный TLS/HTTP2, без KVN-путей/заголовков, fallback; uTLS-профили Chrome/Firefox/Safari с fallback.
- Padding: adaptive для малых/control/QUIC; cover traffic и случайные endpoints — только opt-in, не default (`DEC-007`, `DEC-008`).
- Контракты: v2 stream path + target header профилируемы; сервер dual-accept старый/новый на один релиз.
- Android: DNS routing/fake-DNS сохраняется; QUIC-UDP не обязателен для MVP, HTTP/2 UDP fallback.
- Proof: тест/скрипт/лог с наблюдаемым результатом; без `Proof:` задача не закрыта.
- Вне scope: wire-format v2, relay-миграция, гарантированный обход любого DPI.

## Фаза 1: Контракты и TLS-основа

Цель: зафиксировать v2-only surface, конфиг и подключаемый TLS-профиль.

- [x] T1.1 Сделать `kvn-web` единственным v2 web-бинарем — outcome: без `kvn-web-v2`/auto-detect и v1-web; Touches: src/cmd/web/main.go, src/internal/webui/server.go, scripts/build.sh, README.md
      Proof: code src/cmd/web/main.go main
- [x] T1.2 Добавить v2-only config schema и defaults — outcome: `protocol=v2`, TUN/QUIC/uTLS + stealth/padding/path поля валидируются и сохраняются в `config.yaml`; Touches: src/internal/config/client.go, src/internal/config/v2.go, src/internal/webui/
      Proof: test src/internal/config/v2_test.go TestV2UTLSAndStealthValidation
- [x] T1.3 Выделить TLS-profile пакет — outcome: интерфейс Dialer + профили Chrome/Firefox/Safari + fallback, независимо тестируемые; Touches: src/internal/tlsprofile/, src/internal/transport/tls/
      Proof: test src/internal/tlsprofile/profile_test.go TestDialAllProfiles
- [x] T1.4 Расширить DPI-скрипт до воспроизводимого гейта — outcome: проверяет TLS/ALPN, H2 headers, случайные пути, active probe и fallback; Touches: scripts/dpi-check.sh
      Proof: code scripts/dpi-check.sh RESULT

## Фаза 2: Desktop v2 MVP

Цель: получить v2-only UI и клиент с uTLS + stealth/padding, пригодные для DPI-прогона.

- [x] T2.1 Подключить uTLS к v2-транспорту — outcome: ClientHello соответствует выбранному профилю, ALPN `h2`, при ошибке fallback на стандартный TLS; Touches: src/internal/tlsprofile/, src/internal/transport/client.go
      Proof: test src/internal/transport/transport_test.go TestStreamRoundTrip
- [x] T2.2 Убрать KVN-специфичный HTTP/2 surface — outcome: path + target header профилируются, сервер принимает старый и новый вариант, в defaults нет KVN-маркеров; Touches: src/internal/transport/, src/internal/client/, src/internal/config/
      Proof: test src/internal/transport/transport_test.go TestPaddedTCPStreamRoundTrip
- [x] T2.3 Добавить adaptive padding — outcome: TCP/UDP H2 малые/control-пакеты паддятся по настройке; QUIC padding вне MVP, т.к. QUIC optional; throughput default в бюджете; Touches: src/internal/transport/framing.go, src/internal/transport/stream.go, src/internal/transport/padding.go, src/internal/transport/server.go, src/internal/client/
      Proof: test src/internal/transport/transport_test.go TestPaddedUDPStreamRoundTrip
- [x] T2.4 Защитить fallback и убрать random endpoints — outcome: неизвестные пути получают fallback, клиент по умолчанию не ходит вне v2 endpoint; Touches: src/internal/transport/server.go, src/internal/server/, src/internal/client/, src/internal/transport/transport_test.go
      Proof: test src/internal/transport/transport_test.go TestUnknownPathFallback
- [x] T2.5 Обновить v2-only UI — outcome: нет v1-контролов, есть выбор uTLS-профиля и stealth/padding настроек, всё сохраняется в v2 store; Touches: src/internal/webui/frontend/src/, src/internal/webui/handler_config.go
      Proof: code src/internal/webui/frontend/src/TabbedForm.tsx stealth

## Фаза 3: Удаление v1 и релиз 2.0.0

Цель: оставить одну v2-кодовую базу и собрать 2.0.0.

- [x] T3.1 Удалить v2-алиасы и v1 UI-переключатели — outcome: `kvn-web` принудительно v2, protocol selector/старые поля отсутствуют; Touches: src/cmd/web/main.go, src/internal/webui/, src/internal/webui/frontend/src/
      Proof: code src/internal/webui/handler_connect.go handleConnect
- [x] T3.2 Удалить v1 runtime — outcome: v1 entrypoints/пакеты не компилируются и отсутствуют в дереве; Touches: src/cmd/{client,server,relay}, src/internal/{bootstrap,tunnel,protocol,session,nat,acl,admin,metrics}/
      Proof: code src/cmd/server/main.go main
- [x] T3.3 Обновить build/install/release/CI — outcome: собираются только v2-бинари, install/docs не предлагают v1; Touches: scripts/build.sh, scripts/install-*.sh, Dockerfile, .github/workflows/ci.yml
      Proof: chore scripts/build.sh build_server
- [x] T3.4 Оставить v1 только в документации релизов — outcome: есть инструкция скачать/запустить v1.4.1, нет обещания поддержки v1 в 2.0.0; Touches: docs/ru/, docs/en/, CHANGELOG.md
      Proof: docs docs/ru/v1-legacy.md

## Фаза 4: Android v2

Цель: перевести Android на v2, сохранив DNS routing/fake-DNS.

- [x] T4.1 Провести spike Go mobile/gVisor vs альтернативы — outcome: выбран и записан рабочий способ TUN/TCP на Android, известны размер/риски; Touches: src/android/, src/mobile/, docs/ru/v2.md
      Proof: docs docs/ru/v2.md Android
- [x] T4.2 Вынести v2 mobile core — outcome: Kotlin получает start/stop/status и TUN fd, TCP/UDP идут через v2; Touches: src/mobile/, src/android/app/src/main/kotlin/com/kvn/client/vpn/
      Proof: code src/android/app/src/main/kotlin/com/kvn/client/vpn/KvnV2VpnService.kt
- [x] T4.3 Перевести Android UI/конфиг на v2 — outcome: нет v1-engine/transport selector, конфиг v2, подключение работает; Touches: src/android/app/src/main/kotlin/com/kvn/client/{ui,config,transport}/
      Proof: code src/android/app/src/main/kotlin/com/kvn/client/ui/MainViewModel.kt v2ConfigJson
- [x] T4.4 Сохранить DNS routing/direct — outcome: DNS-запросы перехватываются в mobile core, exclude-домены резолвятся direct и получают direct TCP/UDP; include/default идут через v2; fake-IP не нужен, выдаётся реальный IP. Touches: src/internal/client/direct.go, src/mobile/, src/android/app/src/main/kotlin/com/kvn/client/vpn/KvnV2VpnService.kt
      Proof: code src/internal/client/direct.go handleDirectDNS

## Фаза 5: Проверка и документация

Цель: доказать 2.0.0 и оставить навигацию по v2-only репозиторию.

- [x] T5.1 Прогнать DPI/active-probe в lab — outcome: сохранён lab-отчёт с сетью/регионом, fallback и отсутствие KVN-маркеров; Touches: scripts/dpi-check.sh, docs/ru/v2.md
      Proof: docs docs/ru/v2-dpi-report.md
- [ ] T5.5 Полевой DPI-прогон в целевом регионе — outcome: отчёт с реальной сетью/регионом и fallback; Touches: docs/ru/v2-dpi-report.md, docs/en/v2-dpi-report.md
- [x] T5.2 Добавить CI/regression guard — outcome: тесты запрещают v1 entrypoints, v1-поля в v2 UI и запись в v1 config; Touches: .github/workflows/ci.yml, src/internal/webui/config_store_test.go, scripts/
      Proof: chore scripts/check-v2-only.sh
- [x] T5.3 Обновить REPOSITORY_MAP и CONTEXT — outcome: v2-модули основные, v1 только старый релиз; Touches: REPOSITORY_MAP.md, src/*/CONTEXT.md, README.md
      Proof: docs REPOSITORY_MAP.md
- [x] T5.4 Финальный релизный чеклист 2.0.0 — outcome: артефакты/подписи/доки/Android проверены, v1-бинарей нет; Touches: CHANGELOG.md, docs/ru/, docs/en/, .github/workflows/ci.yml
      Proof: docs docs/ru/v2-release.md

## Покрытие критериев приемки

- AC-001 -> T1.1, T3.2, T3.3, T5.4
- AC-002 -> T2.5, T3.1
- AC-003 -> T1.2, T5.2
- AC-004 -> T3.2, T3.3
- AC-005 -> T1.2, T5.2
- AC-006 -> T3.4, T5.4
- AC-007 -> T4.2, T4.3, T4.4
- AC-008 -> T5.3
- AC-009 -> T1.4, T5.1, T5.5
- AC-010 -> T1.3, T2.1, T2.5
- AC-011 -> T2.2, T2.3, T2.4, T2.5

## Заметки

- T3.2 — самая рискованная задача: удалять v1 только после зелёных desktop/Android тестов.
- T4.1 — gate: без успешного spike не начинать T4.2–T4.4.
- Stale `kvn-web-v2` адаптер удалить в T1.1, не оставлять «на всякий случай».
