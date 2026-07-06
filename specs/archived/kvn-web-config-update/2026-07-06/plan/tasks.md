# KVN Web Config Update — Задачи

## Phase Contract

Inputs: plan, spec, data-model stub.
Outputs: исполнимые задачи с Touches и покрытием AC.
Stop if: нет — plan конкретен, scope ясен.

## Surface Map

| Surface | Tasks |
|---------|-------|
| `frontend/src/types.ts` | T1.1 |
| `frontend/src/TabbedForm.tsx` | T2.1, T2.2 |
| `frontend/src/context.tsx` | T2.1 |
| `handler_config.go` | T3.1 |
| `handler_connect.go` | T3.2 |

## Implementation Context

- Цель MVP: заменить 6 placeholder-инпутов в Routing на chip-списки с add/delete, добавить proxy_connections field, дедуплицировать строки на бэкенде
- Инварианты/семантика:
  - Все 6 полей (exclude_ranges, include_ranges, exclude_ips, include_ips, exclude_domains, include_domains) — `[]string` в Go, `string[]` в TS
  - proxy_connections — `int` в Go, `number` в TS; дефолт 10 (на бэкенде)
  - Дедупликация: stable (первое вхождение остаётся)
  - ChipList — переиспользуемый компонент, не затрагивает SrcRow
- Контракты/протокол:
  - `GET /api/config` → `proxy_connections` уже в JSON (сериализуется из Go)
  - `PUT /api/config/global` и `PUT /api/servers/{name}` принимают `proxy_connections`
  - Сохранённые списки в YAML не содержат дублей
- Границы scope:
  - Не меняем SrcRow (include_sources / exclude_sources)
  - Не добавляем валидацию CIDR/IP на фронтенде
  - Не добавляем drag-and-drop
- Proof signals: `npm run build` проходит; chip-списки рендерятся; add/delete работают; дубли не сохраняются; proxy_connections = N → в логах `"slots": N`

## Фаза 1: Основа (types.ts)

Цель: подготовить TypeScript-типы для proxy_connections.

- [x] T1.1 Добавить `proxy_connections?: number` в `ClientConfig` в types.ts
  Touches: `frontend/src/types.ts`
  AC: AC-005, AC-006
  Trace: `@sk-task kvn-web-config-update#T1.1` над полем `proxy_connections` в типе ClientConfig

## Фаза 2: MVP (UI)

Цель: ChipList-компонент + proxy_connections поле — вся пользовательская видимость.

- [x] T2.1 Реализовать ChipList-компонент и добавить в renderRouting для 6 exclude/include полей
  - ChipList: принимает `items: string[]`, `onAdd: (val: string) => void`, `onRemove: (idx: number) => void`, `label: string`, `placeholder: string`
  - Рендерит chip-блоки с ✕ и input для добавления (Enter / кнопка +)
  - Пустой ввод игнорируется
  - Добавление дубликата на фронтенде: no-op (не добавляет в локальный state)
  - Добавить callback'и `onAddRoutingString(list, val)` и `onRemoveRoutingString(list, idx)` в context.tsx и передать в TabbedFormProps
  - Заменить 6 placeholder-блоков в renderRouting на ChipList
  - Имена полей в UI: "CIDR", "IPs", "Domains" для Include и Exclude (как сейчас, но теперь рабочие)
  Touches: `frontend/src/TabbedForm.tsx`, `frontend/src/context.tsx`
  AC: AC-001, AC-002, AC-003
  Trace: `@sk-task kvn-web-config-update#T2.1` над объявлением компонента ChipList

- [x] T2.2 Добавить поле proxy_connections в renderAdvanced, секция Tuning
  - `<input type="number">` со значением из `serverConfig.proxy_connections ?? 10`
  - onChange: `props.onUpdateServer("proxy_connections", parseInt(e.target.value) || 10)`
  Touches: `frontend/src/TabbedForm.tsx`
  AC: AC-005, AC-006
  Trace: `@sk-task kvn-web-config-update#T2.2` перед инпутом proxy_connections в renderAdvanced

## Фаза 3: Backend deduplication

Цель: гарантировать, что сохранённые и смерженные списки не содержат дублей.

- [x] T3.1 Добавить дедупликацию []string в handleSaveGlobalConfig и handleUpdateServer
  - Вспомогательная функция `dedupStrings(s []string) []string` — stable (первое вхождение), O(n)
  - Применять ко всем routing []string полям в `handleSaveGlobalConfig` (cfg.ClientConfig.Routing) и `handleUpdateServer` (sv.Routing) перед сохранением
  Touches: `src/internal/webui/handler_config.go`
  AC: AC-004, AC-007
  Trace: `@sk-task kvn-web-config-update#T3.1` над `dedupStrings`

- [x] T3.2 Добавить дедупликацию []string в mergeConfig для routing-списков
  - После строки `merged.Routing = server.Routing` (или при построении merged.Routing с нуля) применить dedupStrings ко всем 6 routing полям
  Touches: `src/internal/webui/handler_connect.go`
  AC: AC-004, AC-007
  Trace: `@sk-task kvn-web-config-update#T3.2` над блоком дедупликации в mergeConfig

## Фаза 4: Проверка

Цель: доказать, что фича работает, через сборку и ручную проверку.

- [x] T4.1 Собрать проект и проверить по AC-001 — AC-007
  - `npm run build` (в frontend) + `go vet ./src/...` + `go build ./src/...`
  - Открыть kvn-web → Routing tab: 6 полей как chip-списки
  - Добавить/удалить элементы → сохранить → перезагрузить → изменения сохранены
  - Добавить дубли → убедиться, что в YAML их нет
  - Advanced tab → проверить proxy_connections поле, изменить, сохранить, проверить в YAML и в логах при connect
  Touches: `frontend/`, `src/`, `~/.config/kvn/config.yaml` (проверка)
  AC: AC-001, AC-002, AC-003, AC-004, AC-005, AC-006, AC-007

## Покрытие критериев приемки

- AC-001 → T2.1, T4.1
- AC-002 → T2.1, T4.1
- AC-003 → T2.1, T4.1
- AC-004 → T3.1, T3.2, T4.1
- AC-005 → T1.1, T2.2, T4.1
- AC-006 → T1.1, T2.2, T3.1, T4.1
- AC-007 → T3.1, T3.2, T4.1

## Заметки

- T2.1 — самая крупная задача (ChipList + 6 полей + контекст), не дробится: все 6 полей идентичны, один компонент
- T1.1 и T2.2 можно выполнять параллельно
- T3.1 и T3.2 можно выполнять параллельно (разные файлы)
- Trace-маркеры: `@sk-task kvn-web-config-update#T<phase>.<index>` над owning function/component/type declaration

## Trace proof

- T1.1 → `frontend/src/types.ts:20` (@sk-task kvn-web-config-update#T1.1)
- T2.1 → `frontend/src/TabbedForm.tsx:125` (@sk-task kvn-web-config-update#T2.1 — ChipList)
- T2.1 → `frontend/src/context.tsx:43` (@sk-task kvn-web-config-update#T2.1 — AppState)
- T2.1 → `frontend/src/context.tsx:330` (@sk-task kvn-web-config-update#T2.1 — addRoutingString)
- T2.2 → `frontend/src/TabbedForm.tsx:378` (@sk-task kvn-web-config-update#T2.2 — proxy_connections field)
- T3.1 → `handler_config.go:302` (@sk-task kvn-web-config-update#T3.1 — dedupRoutingStrings)
- T3.2 → `handler_connect.go:228` (@sk-task kvn-web-config-update#T3.2 — dedup in mergeConfig)

## Map update

Map update: no — изменения локальные (только существующие файлы), структура репозитория не изменилась.

Готово к: /speckeep.verify kvn-web-config-update
