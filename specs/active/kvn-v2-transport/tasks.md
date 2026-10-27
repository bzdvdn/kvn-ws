# KVN v2: TCP-first stealth transport — Задачи

## Surface Map

| Surface | Tasks |
|---------|-------|
| src/internal/config/v2.go | T1.1, T3.2 |
| src/internal/v2/protocol/ | T1.2, T2.2 |
| src/internal/v2/transport/ | T1.2, T2.2 |
| src/internal/v2/server/ | T1.3, T2.2 |
| src/internal/v2/tcpstack/ | T2.1, T2.3 |
| src/internal/v2/client/ | T2.0, T2.2, T2.3, T4.1 |
| src/cmd/v2server/ | T1.3 |
| src/cmd/v2client/ | T2.3 |
| src/internal/webui/frontend/src/ | T3.1, T3.2 |
| src/internal/webui/ | T3.1, T3.2, T3.3 |
| src/internal/config/client.go | T3.2 |
| src/integration/v2/ | T5.1, T5.2 |
| scripts/bench-v2.sh, scripts/dpi-check.sh | T4.2 |
| CHANGELOG.md, docs/ru|en/v2.md, docs/ru|en/config.md | T5.3 |

## Implementation Context

- Цель MVP: Go v2 server/client + HTTP/2 per-flow streams + TCP split + stealth fallback + minimal multi-server UI.
- Инварианты: v2 wire несовместим с v1; v1 код/серверы не меняются; TCP не идёт raw IP; Bearer до stream; один фиксированный TLS-профиль.
- Stealth: v2 не имеет legacy path/custom markers; невалидный запрос получает fallback; ALPN `h2`; standard headers.
- UI: kvn-web запускает v2 client; protocol v1/v2; primary имя+endpoint+токен; Advanced Routing/DNS/TLS/Proxy; stealth всегда on.
- UDP MVP: best-effort stream, H3 datagrams — следующая фаза.
- Границы: Android/relay/ECH/CDN — позже.
- Proof signals: pcap без inner TCP, slow-flow isolation, fallback probe, JA3 series, v1 mixed deployment, benchmark v1 vs v2.
- Trace: `@sk-task`/`@sk-test` на новых декларациях.

## Фаза 1: v2 каркас

- [x] T1.1 Добавить v2 config schema — outcome: multi-server entries (name/endpoint/token), stealth defaults, без v1-полей. Touches: src/internal/config/v2.go
      Proof: code src/internal/config/v2.go V2ServerConfig
- [x] T1.2 Создать v2 protocol/transport skeleton — outcome: HTTP/2 stream, Bearer, fallback, фиксированный TLS/ALPN. Touches: src/internal/v2/protocol/, src/internal/v2/transport/
      Proof: code src/internal/v2/transport/transport.go Dial
- [x] T1.3 Создать v2 server entrypoint — outcome: listener, auth, target dial, fallback, stream limit. Touches: src/internal/v2/server/, src/cmd/v2server/
      Proof: code src/cmd/v2server/main.go main

## Фаза 2: TCP split

- [x] T2.0 Сделать v2 proxy-first клиент для раннего теста — outcome: SOCKS5/HTTP CONNECT через HTTP/2 stream до target, без TUN. Touches: src/internal/v2/client/, src/cmd/v2client/, configs/v2-client.example.yaml
      Proof: test src/internal/v2/client/client_test.go TestProxyClientEndToEnd
- [x] T2.1 Подключить userspace TCP stack — outcome: TCP flow из TUN терминируется локально через gVisor/netstack. Touches: src/internal/v2/tcpstack/
      Proof: code src/internal/v2/tcpstack/stack.go New
- [x] T2.2 Реализовать flow↔stream mapping — outcome: каждый TCP flow получает отдельный HTTP/2 stream, backpressure и лимит. Touches: src/internal/v2/protocol/, src/internal/v2/transport/, src/internal/v2/server/, src/internal/v2/client/
      Proof: code src/internal/v2/tcpstack/stack.go bridge
- [x] T2.3 Подключить v2 client/TUN — outcome: TUN направляет TCP в tcpstack; UDP пока дропается с явным ограничением (best-effort/H3 — T4.1). Touches: src/internal/v2/client/, src/internal/v2/tcpstack/, src/cmd/v2client/
      Proof: code src/internal/v2/client/client.go runTUN

## Фаза 3: UI

- [x] T3.1 Сделать kvn-web v2 UI — outcome: protocol selector v1/v2, primary 3 поля, список серверов; Advanced-вкладки Routing/DNS/TLS/Proxy доступны. Touches: src/internal/webui/frontend/src/, src/internal/webui/
      Proof: code src/internal/webui/frontend/src/TabbedForm.tsx protocol
- [x] T3.2 Подключить v2 engine к kvn-web backend — outcome: kvn-web запускает v2 client in-process по `protocol: v2`, сохраняет advanced-настройки, статус/логи. Touches: src/internal/webui/, src/internal/config/v2.go, src/internal/config/client.go, src/internal/v2/client/
      Proof: code src/internal/webui/handler_connect.go v2client
- [x] T3.3 Проверить lifecycle v2 из UI — outcome: connect/disconnect/reconnect v2 через kvn-web, TUN cleanup, ошибки без падения UI. Touches: src/internal/webui/state.go, src/internal/webui/handler_connect.go, src/internal/v2/client/
      Proof: test src/internal/webui/v2_lifecycle_test.go TestStopStaleClientReleasesV2Tun

## Фаза 4: UDP и проверки

- [ ] T4.1 Добавить UDP best-effort + H3 флаг — outcome: UDP идёт best-effort, при H3 — datagrams; путь виден в логах. Touches: src/internal/v2/client/, src/internal/v2/transport/, src/internal/v2/server/
- [ ] T4.2 Добавить benchmark/DPI-скрипты — outcome: v1 vs v2 throughput/p95/allocs и серия ClientHello/JA3/JA4. Touches: scripts/bench-v2.sh, scripts/dpi-check.sh

## Фаза 5: Валидация и docs

- [ ] T5.1 Покрыть v2 unit/integration — outcome: stream, slow-flow, auth/fallback, UDP best-effort, лимиты. Touches: src/internal/v2/, src/integration/v2/
- [ ] T5.2 Подтвердить v1 mixed deployment — outcome: v1 клиент/сервер работают рядом с v2; v2 не dial-ит v1. Touches: src/integration/v2/, src/integration/
- [ ] T5.3 Обновить docs/CHANGELOG — outcome: v2 архитектура, миграция, UI, ограничения UDP/Android/relay. Touches: CHANGELOG.md, docs/ru/v2.md, docs/en/v2.md, docs/ru/config.md, docs/en/config.md
- [ ] T5.4 Прогнать gate — outcome: Go race/vet, frontend build, benchmark-гейт зелёные. Touches: src/internal/v2/, src/internal/webui/, src/integration/v2/

## Покрытие критериев приемки

- AC-001 -> T2.1, T2.2, T5.1
- AC-002 -> T2.2, T5.1
- AC-003 -> T1.2, T1.3, T5.1
- AC-004 -> T1.2, T4.2
- AC-005 -> T3.1, T3.2, T3.3
- AC-006 -> T4.2, T5.4
- AC-007 -> T1.1, T5.2
- AC-008 -> T4.1, T5.1
- AC-009 -> T2.2, T5.1

## Заметки

- Data model: new v2 config; v1 schema не меняется.
- v2 может быть несовместим с v1; старые серверы остаются.
- Android/relay/H3 — отдельные фазы после MVP.
