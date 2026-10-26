# Детерминированный приоритет domain-правил

## Scope Snapshot

- In scope: `RuleSet.MatchDomain` выбирает include/exclude по специфичности, а не по порядку map.
- Out of scope: приоритет IP/CIDR-правил, DNS resolver, relay DNS-перехват.

## Цель

Пользователь split-tunnel получает предсказуемую маршрутизацию доменов: более конкретное правило всегда важнее общего, а при равной специфичности побеждает exclude. Это позволяет точечно вернуть домен в туннель (например, `ktalk.ru` при общем `.ru` exclude) без случайного поведения.

## Основной сценарий

1. В конфиге есть общий exclude `.ru` и точечный include `ktalk.ru`.
2. DNS-трекер сопоставляет IP с `ktalk.ru`; `MatchDomain` выбирает include, потому что exact-правило специфичнее suffix.
3. Для `hh.ru` срабатывает `.ru` exclude; для домена без правил — `RouteNone`.
4. При конфликте одинаковой специфичности (например, `.ru` в include и exclude) результат стабильно exclude.

## User Stories

- P1: как пользователь, я хочу исключить весь `.ru`, но оставить отдельные сервисы в туннеле.
- P2: как оператор, я хочу, чтобы одинаковый конфиг давал одинаковый маршрут при каждом запуске.
- P3: как разработчик, я хочу явное правило специфичности вместо случайного порядка map.

## MVP Slice

- `AC-001` + `AC-002` + `AC-003`: exact > suffix, длинный suffix > короткий, tie → exclude.
- `AC-004` — детерминизм на повторных вызовах.

## First Deployable Outcome

- Unit-тесты `MatchDomain` покрывают матрицу конфликтов; существующие routing-тесты зелёные.

## Scope

- `src/internal/routing/rule_set.go`: детерминированный выбор exact/suffix правил.
- Тесты `src/internal/routing/routing_test.go`.
- Документация routing-семантики в `docs/ru|en/architecture.md` при необходимости.

## Контекст

- `exactDomains` и `suffixDomains` — map по `RouteAction`; текущий `MatchDomain` перебирает map, порядок недетерминирован.
- Для IP/CIDR правила уже ordered: exclude проверяется первым; менять это вне scope.
- Suffix-правила начинаются с `.`; exact-правила — без точки.
- `Route` использует `MatchDomain` через DNS tracker.

## Зависимости

- `src/internal/config.RoutingCfg` (`IncludeDomains`, `ExcludeDomains`) и `src/internal/dns.Tracker` — без изменений контрактов.

## Требования

- RQ-001 Exact-правило ДОЛЖНО иметь приоритет над любым suffix-правилом.
- RQ-002 Среди suffix-правил ДОЛЖНО побеждать самое длинное совпадение.
- RQ-003 При равной специфичности (exact+exact или одинаковый suffix) ДОЛЖЕН побеждать exclude (`RouteDirect`).
- RQ-004 Результат `MatchDomain` НЕ ДОЛЖЕН зависеть от порядка итерации map.
- RQ-005 Поведение без конфликтов ДОЛЖНО остаться прежним.

## Вне scope

- Приоритет CIDR/IP-правил и `RuleSet.Route` для IP без домена.
- Изменение формата конфига, DNS tracker и resolver.
- Relay DNS-маршрутизация.
- Case-insensitive нормализация доменов, если её не было раньше.

## Критерии приемки

### AC-001 Exact важнее suffix

- Почему это важно: точечный include должен перебивать общий exclude.
- **Given** `ExcludeDomains: [".ru"]`, `IncludeDomains: ["ktalk.ru"]`
- **When** `MatchDomain("ktalk.ru")`
- **Then** результат `RouteServer`, а `hh.ru` — `RouteDirect`
- Evidence: unit-тест.

### AC-002 Длинный suffix важнее короткого

- Почему это важно: более конкретная зона должна побеждать общую.
- **Given** include `.ktalk.host` и exclude `.host`
- **When** `MatchDomain("board.ktalk.host")`
- **Then** результат `RouteServer`; для `example.host` — `RouteDirect`
- Evidence: unit-тест.

### AC-003 Tie → exclude

- Почему это важно: при равной специфичности безопаснее не пускать в туннель.
- **Given** один и тот же домен/suffix есть и в include, и в exclude
- **When** `MatchDomain` вызывается для совпадающего домена
- **Then** результат `RouteDirect`
- Evidence: unit-тест для exact+exact и suffix+suffix.

### AC-004 Результат детерминирован

- Почему это важно: map-итерация давала случайный маршрут между запусками.
- **Given** конфликтующий набор правил
- **When** `MatchDomain` вызывается 1000 раз
- **Then** все результаты одинаковы
- Evidence: unit-тест с циклом.

### AC-005 Нет регресса без конфликтов

- Почему это важно: существующие split-tunnel конфиги должны работать как раньше.
- **Given** только exclude или только include
- **When** выполняются существующие routing-тесты
- **Then** все проходят
- Evidence: `go test ./src/internal/routing/`.

## Допущения

- Домены приходят в нижнем регистре; отдельная нормализация не требуется.
- Suffix-правило с точкой матчит домен и поддомены, как сейчас (`strings.HasSuffix`).
- IP/CIDR-порядок остаётся прежним: exclude раньше include.

## Критерии успеха

- SC-001 `MatchDomain` не использует итерацию map для выбора действия.
- SC-002 Нет новых аллокаций на горячем пути сверх текущих.

## Краевые случаи

- Домен без точки и без правил → `RouteNone`.
- Пустой список правил → `RouteNone`.
- Одинаковый suffix в обоих действиях → `RouteDirect`.
- Exact include и suffix exclude → include.
- Exact exclude и suffix include → exclude.

## Открытые вопросы

- none
