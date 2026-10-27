# Stealth surface hardening — План

## Phase Contract

Inputs: `spec.md`, текущие WS/TLS/config surfaces.
Outputs: `plan.md`; wire-формат не меняется, дефолты остаются legacy.

## Цель

Добавить opt-in hardening поверхности без изменения контрактов: random padding для малых кадров, decoy API, jitter и профиль заголовков. Главный инвариант — старый клиент и старый сервер продолжают работать без изменений.

## MVP Slice

- `padding.mode: random` + mixed-version тесты (`AC-001`, `AC-002`, `AC-003`).
- Decoy API и jitter/headers — следующими инкрементами.

## First Validation Path

1. `go test -race ./src/internal/transport/websocket/ ./src/integration/`.
2. Mixed-version тест: random-клиент ↔ fixed-сервер и наоборот.
3. `go test -bench` для random vs fixed: throughput/alloc regression ≤5%.

## Scope

- `config.PaddingCfg`: `mode: fixed|random` (default fixed).
- `transport/websocket`: random padding policy; ручка `PaddingMode` в `WSConfig`/`FactoryConfig`.
- `bootstrap/server`: decoy JSON для не-WS запросов на tunnel path и `/`.
- `bootstrap/client`: opt-in headers profile и jitter keepalive/reconnect.
- Тесты mixed-version, decoy, jitter, benchmark.

## Performance Budget

- Throughput random ≥95% от fixed на том же профиле.
- Allocs/op random не выше fixed +10%.
- Jitter не увеличивает средний keepalive-интервал.
- Decoy-ответы не на горячем пути.

## Implementation Surfaces

- `src/internal/config/client.go` — `PaddingCfg.Mode`, defaults/validation; новые поля headers/jitter.
- `src/internal/transport/transport.go`, `websocket/wsfactory.go`, `websocket/websocket.go` — `PaddingMode`, политика записи, handshake headers.
- `src/internal/bootstrap/client/dial.go`, `tun.go` — прокидывание mode/headers/jitter.
- `src/internal/bootstrap/server/server.go`, `handler.go` — mode из конфига, decoy handlers.
- `src/internal/transport/control` или bootstrap — jitter helper.
- `src/internal/webui/frontend/src/types.ts`, `TabbedForm.tsx` — per-server/global поля.
- `src/internal/webui/handler_connect.go` — mergeConfig новых полей.
- Тесты: `websocket_test.go`, `client_test.go`, `server`/`integration`, benchmark.

## Bootstrapping Surfaces

- none: пакеты и конфиг-структуры уже есть.

## Влияние на архитектуру

- Wire-формат padding не меняется: `[4B len][payload][tail]`.
- Reader по-прежнему использует только length prefix, поэтому `fixed` и `random` совместимы при включённом padding.
- `padding.enabled: false` остаётся единственным «off»; новый режим не отключает padding.
- Дефолт `fixed` сохраняет байт-в-байт поведение.

## Acceptance Approach

- AC-001 → random policy в `WriteMessage` + unit roundtrip/размеры.
- AC-002 → mixed-version integration: random↔fixed в обе стороны.
- AC-003 → существующие websocket/integration тесты + проверка дефолта.
- AC-004 → httptest: GET без Upgrade + валидный WS на том же пути.
- AC-005 → unit-тест jitter-границ + keepalive-тест.
- AC-006 → httptest с захватом заголовков; old-server mixed test.
- AC-007 → handshake-тест без `Sec-WebSocket-Protocol` при multiplex=false.
- AC-008 → benchmark random vs fixed.
- AC-009 → alias no-pad mixed-version unit/integration.
- AC-010 → subprotocol alias negotiation unit.

## Данные и контракты

- Data model: no change.
- Wire contract: без изменений; добавляется только конфиг-режим padding.
- `PaddingCfg.Mode` default `fixed`; unknown → `fixed` + warn.
- `WSConfig.PaddingMode`; `FactoryConfig.PaddingMode`.
- `PaddingCfg.NoPadHeaders []string` (default `["X-KVN-NoPad"]`), `MultiplexSubprotocols []string` (default `["kvn-ws-mux"]`).
- Клиент отправляет все no-pad имена; сервер принимает любое из своего списка и отвечает совпавшим именем; клиент проверяет ответ по любому из своих имён.
- `contracts/*` и `data-model.md` не создаются.

## Стратегия реализации

- DEC-001 Random padding как режим без нового wire-формата
  Why: старый reader уже игнорирует tail, поэтому не нужна негоциация режима; достаточно общего `padding.enabled`.
  Tradeoff: random и fixed могут работать в паре, но `off` по-прежнему требует согласия.
  Affects: `websocket.go`, `wsfactory.go`, `transport.go`, config.
  Validation: mixed-version roundtrip + benchmark.

- DEC-002 Дефолт `fixed` и запрет менять `padding.enabled`
  Why: старые конфиги/клиенты не должны заметить обновление.
  Tradeoff: hardening не включится сам; нужен явный opt-in.
  Affects: `SetClientDefaults`, server config load.
  Validation: AC-003.

- DEC-003 Decoy только для не-WS запросов
  Why: активное пробирование видит HTTP-ответ, а WS-клиенты не должны затронуться.
  Tradeoff: decoy-ответы должны быть согласованы с реальным API.
  Affects: `server.buildMux`, `handleTunnel`.
  Validation: AC-004.

- DEC-004 Jitter только по конфигу, с жёсткими границами
  Why: ровный cadence — fingerprint, но большой jitter ломает keepalive.
  Tradeoff: серверный keepalive по умолчанию остаётся ровным.
  Affects: client keepalive/reconnect, helper.
  Validation: AC-005.

- DEC-005 Headers profile opt-in и без Origin по умолчанию
  Why: строгий origin-check старого сервера не должен отклонять нового клиента.
  Tradeoff: профиль неполный, пока оператор не настроит Origin.
  Affects: `DialContext`, client config.
  Validation: AC-006.

- DEC-006 Настраиваемые alias-списки вместо удаления маркеров
  Why: `X-KVN-NoPad`/`kvn-ws-mux` внутри TLS и не видны пассивному DPI; alias-списки убирают конкретные строки для HTTP-слоя/CDN, сохраняя старые имена дефолтом.
  Tradeoff: клиент отправляет несколько имён; сервер отвечает совпавшим; при отсутствии подтверждения остаётся padded.
  Affects: config, `WSConfig`, `DialContext`, `Accept`, handshake tests.
  Validation: AC-009, AC-010, mixed-version.

## Incremental Delivery

### MVP

- Random padding + config + mixed-version тесты.
- Готовность: AC-001, AC-002, AC-003, AC-008.

### Расширение

- Decoy API, jitter, headers profile.
- Готовность: AC-004, AC-005, AC-006, AC-007.

## Порядок реализации

1. Config/WSConfig `PaddingMode` + random policy + тесты.
2. Mixed-version integration + benchmark.
3. Decoy handlers.
4. Jitter helper + client wiring.
5. Headers profile + subprotocol test.
6. Docs/CHANGELOG + gate.

## Риски

- Старый peer валидирует общий размер padded-сообщения (не подтверждено кодом).
  Mitigation: дефолт fixed; mixed-version тесты.
- Random padding даёт новый размерный fingerprint.
  Mitigation: диапазон/порог настраиваемые; benchmark и анализ распределения.
- Доп. заголовки ломают строгий origin-check.
  Mitigation: opt-in, Origin не ставим по умолчанию.
- Jitter вызывает ложные timeout'ы.
  Mitigation: clamp, тест границ.
- Decoy-ответы конфликтуют с реальными API.
  Mitigation: только не-WS и только согласованные пути.

## Rollout и compatibility

- Сервер можно обновлять первым: старые клиенты получают fixed.
- Клиент можно обновлять первым: старый сервер игнорирует mode/headers; padding остаётся fixed.
- Новые режимы включаются конфигом; дефолт не меняется.
- Мониторинг: размеры WS-сообщений, доля decoy-ответов, keepalive timeout'ы.
- Docs: `docs/ru|en/config.md`, `docs/ru|en/architecture.md`, CHANGELOG.

## Проверка

- Unit: padding random/fixed, jitter bounds, headers, subprotocol.
- Integration: mixed-version random↔fixed в обе стороны; decoy+WS.
- Benchmark: fixed vs random throughput/allocs.
- Commands: `go test -race ./src/internal/transport/websocket/ ./src/internal/bootstrap/server/ ./src/internal/bootstrap/client/ ./src/integration/`, `go vet ./...`.
- Покрывает AC-001–AC-008, DEC-001–DEC-006.

## Соответствие конституции

- нет конфликтов: wire-контракт не меняется, дефолты legacy, DDD-границы сохранены, trace-маркеры на новых декларациях.
