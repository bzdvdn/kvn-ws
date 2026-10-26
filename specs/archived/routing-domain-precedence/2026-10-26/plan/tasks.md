# Детерминированный приоритет domain-правил — Задачи

## Surface Map

| Surface | Tasks |
|---------|-------|
| src/internal/routing/rule_set.go | T1.1 |
| src/internal/routing/routing_test.go | T2.1, T2.2 |
| docs/ru/architecture.md, docs/en/architecture.md | T3.1 |
| src/internal/routing/ | T3.2 |

## Implementation Context

- Цель MVP: `MatchDomain` выбирает правило детерминированно: exact > suffix, длинный suffix > короткий, tie → exclude.
- Инварианты: IP/CIDR-порядок не меняется; формат конфига и DNS tracker не меняются.
- Семантика: exact include/exclude проверяются первыми (exclude при равенстве); среди suffix побеждает максимальная длина; при равной длине — `RouteDirect`.
- Ошибки: нет; неизвестный домен → `RouteNone`.
- Контракты: `MatchDomain(string) RouteAction` без изменения сигнатуры.
- Границы: не трогаем resolver, relay DNS, IP/CIDR precedence.
- Proof signals: матрица unit-тестов на конфликты + 1000 повторных вызовов + существующие routing-тесты.
- Trace: `@sk-task`/`@sk-test` на изменённых декларациях.

## Фаза 1: Реализация

- [x] T1.1 Сделать `MatchDomain` детерминированным — outcome: exact > suffix, longest suffix, tie → exclude; нет итерации map для выбора. Touches: src/internal/routing/rule_set.go
      Proof: code src/internal/routing/rule_set.go MatchDomain

## Фаза 2: Валидация

- [x] T2.1 Покрыть матрицу приоритетов — outcome: exact/suffix/longest/tie сценарии дают ожидаемый RouteAction. Touches: src/internal/routing/routing_test.go
      Proof: test src/internal/routing/routing_test.go TestRuleSetMatchDomainPrecedence
- [x] T2.2 Подтвердить детерминизм и отсутствие регресса — outcome: 1000 вызовов стабильны, существующие routing-тесты зелёные. Touches: src/internal/routing/routing_test.go
      Proof: test src/internal/routing/routing_test.go TestRuleSetMatchDomainDeterministic

## Фаза 3: Документация и gate

- [x] T3.1 Описать приоритет доменов — outcome: ru/en docs фиксируют exact > suffix, longest suffix, tie → exclude. Touches: docs/ru/architecture.md, docs/en/architecture.md
      Proof: docs docs/ru/architecture.md
- [x] T3.2 Прогнать gate — outcome: `go test -race ./src/internal/routing/` и `go vet ./...` зелёные. Touches: src/internal/routing/
      Proof: test src/internal/routing/routing_test.go TestRuleSetMatchDomainPrecedence

## Покрытие критериев приемки

- AC-001 -> T1.1, T2.1
- AC-002 -> T1.1, T2.1
- AC-003 -> T1.1, T2.1
- AC-004 -> T1.1, T2.2
- AC-005 -> T2.2, T3.2

## Заметки

- Data model: no change; contracts-файлы не создаются.
