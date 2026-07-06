# KVN Web Config Update — План

## Phase Contract

Inputs: spec, inspect, минимальный repo-контекст (surfaces).
Outputs: plan, data-model stub.
Stop if: нет — spec прозрачна, inspect pass.

## Цель

Реализовать редактируемые exclude/include списки в Routing (chip + delete + add) через переиспользуемый компонент `ChipList`, добавить `proxy_connections` в Advanced tab, дедуплицировать строковые списки на бэкенде при сохранении и в mergeConfig.

## MVP Slice

Один implementation pass, покрывающий все 7 AC:
- AC-001, AC-002, AC-003 — chip-списки с add/delete
- AC-004, AC-007 — дедупликация при сохранении + порядок
- AC-005, AC-006 — proxy_connections в UI + сохранение

## First Validation Path

1. Собрать `npm run build` в frontend + `go build ./src/...`
2. Открыть kvn-web → Routing tab → видны chip-списки всех 6 exclude/include полей
3. Добавить/удалить элементы → сохранить → перезагрузить → изменения сохранены
4. Убедиться что дубликаты не сохраняются (прямая проверка YAML)
5. Advanced tab → proxy_connections поле с дефолтом 10

## Scope

- Routing tab: замена 6 placeholder-инпутов на компонент chip-списка с add/delete
- Advanced tab: добавление поля proxy_connections (number input)
- types.ts: добавление `proxy_connections`
- context.tsx: новые callback-функции для add/remove элемента string[]
- handler_config.go: дедупликация []string в handleSaveGlobalConfig, handleUpdateServer
- handler_connect.go: дедупликация в mergeConfig для слайсов Routing-списков
- Бэкенд НЕ меняет: структуру `ClientConfig`, `RoutingCfg`, логику defaults

## Performance Budget

- none: изменение UI, без измеримой нагрузки

## Implementation Surfaces

| Surface | Роль | Тип |
|---|---|---|
| `frontend/src/TabbedForm.tsx` | ChipList-компонент, рендеринг 6 полей + proxy_connections | существующая, расширение |
| `frontend/src/context.tsx` | Новые callback addStringItem/removeStringItem | существующая, расширение |
| `frontend/src/types.ts` | proxy_connections?: number | существующая, расширение |
| `handler_config.go` | Дедупликация []string при save | существующая, расширение |
| `handler_connect.go` | Дедупликация []string в mergeConfig | существующая, расширение |

## Bootstrapping Surfaces

- none: структура репозитория достаточна

## Влияние на архитектуру

- Локальное: новый React-компонент `ChipList`, 3 новых callback в контексте
- Никакого влияния на интеграции, API-контракты (JSON поля не меняются), rollout
- Data model Go не меняется

## Acceptance Approach

- AC-001 → ChipList рендерит routing-поля как chip; surfaces: TabbedForm; проверка: визуально
- AC-002 → ChipList Add handler; surfaces: TabbedForm + context; проверка: add + save + reload
- AC-003 → ChipList Remove handler; surfaces: TabbedForm + context; проверка: remove + save + reload
- AC-004 → Backend dedup при save; surfaces: handler_config, handler_connect; проверка: YAML без дублей
- AC-005 → proxy_connections в Advanced; surfaces: TabbedForm + types; проверка: визуально
- AC-006 → proxy_connections сохраняется и применяется; surfaces: handler_config + TabbedForm; проверка: connect log
- AC-007 → Order preservation при dedup; surfaces: handler_config; проверка: порядок в YAML

## Данные и контракты

- Go `ClientConfig.ProxyConnections` уже существует (client.go:48) — не меняется
- types.ts: добавить `proxy_connections?: number`
- RoutingCfg слайсы (`[]string`) не меняются
- JSON API контракты не расширяются — proxy_connections уже сериализуется как `proxy_connections`
- `data-model.md` — no-change stub для Go, types.ts expansion указана здесь

## Стратегия реализации

- DEC-001 Один ChipList-компонент для всех 6 string[] полей
  Why: исключить дублирование JSX; поля идентичны по структуре (label + chip-список + add), различаются только ключом доступа к serverConfig.routing.* и отображаемым именем.
  Tradeoff: компонент принимает массив строк и коллбэки, теряет полную кастомизацию под каждое поле; для данного scope это не нужно.
  Affects: TabbedForm.tsx
  Validation: все 6 полей рендерятся одинаково рабочими add/delete

- DEC-002 Дедупликация на бэкенде (save + mergeConfig) — единственный source of truth
  Why: клиент может отправить дубли из любой версии UI; бэкенд должен гарантировать консистентность данных. Фронтенд тоже предотвращает видимые дубли (не добавляет уже существующий элемент), но это UX-оптимизация, не гарантия.
  Tradeoff: дополнительная логика в 3 местах (2 handler + mergeConfig); цена мала (O(n) проход).
  Affects: handler_config.go (handleSaveGlobalConfig, handleUpdateServer), handler_connect.go (mergeConfig)
  Validation: AC-004, AC-007

- DEC-003 proxy_connections на вкладке Advanced, секция Tuning
  Why: Advanced — технические параметры производительности; General — базовые настройки. proxy_connections — low-level tuning параметр.
  Tradeoff: пользователь может не сразу найти; но это соответствует разделению вкладок.
  Affects: TabbedForm.tsx (renderAdvanced)
  Validation: AC-005

## Incremental Delivery

### MVP (Первая ценность)

Весь scope — один implementation pass: chip-списки + dedup + proxy_connections.
Критерий: все 7 AC проходят валидацию.

### Итеративное расширение

- none — фича едина и неделима

## Порядок реализации

1. ChipList компонент + callbacks в TabbedForm (TypeScript) — фронтенд без бэкенда не ломается, placeholder-инпуты остаются до перерисовки
2. proxy_connections в types.ts + Advanced tab
3. Дедупликация в handler_config.go (handleSaveGlobalConfig, handleUpdateServer)
4. Дедупликация в mergeConfig (handler_connect.go)
5. Сборка + ручная проверка

Шаги 1-2 можно параллелить. Шаги 3-4 зависят от шага 1 (знание структуры данных).

## Риски

- Отсутствие npm/yarn в среде сборки: `npm run build` может не сработать; mitigation: `go build ./...` использует embedded dist, можно предварительно собрать вручную или использовать pre-built dist.
- Регрессия в существующих SourceRule-полях (include_sources/exclude_sources): mitigation: не менять SrcRow-компонент, только добавлять ChipList рядом.

## Rollout и compatibility

- Специальных rollout-действий не требуется: новый код только расширяет существующий UI и бэкенд
- Старые конфиги без proxy_connections продолжают работать (дефолт 10 в Go)
- Старые конфиги с дублями в списках — после первого сохранения через UI дубли удаляются
- Breaking change: нет

## Проверка

- `go vet ./src/...` + `go build ./src/...`
- `npm run build` в `frontend` директории (или валидация pre-built dist)
- Ручная проверка по сценариям AC-001 — AC-007
- Чтение YAML после сохранения: `grep -A5 exclude_ranges ~/.config/kvn/config.yaml`

## Соответствие конституции

- нет конфликтов
- Trace-маркеры `@sk-task kvn-web-config-update#T*` будут проставлены на ChipList, proxy_connections поле, dedup-функцию

Готово к: /speckeep.tasks kvn-web-config-update
