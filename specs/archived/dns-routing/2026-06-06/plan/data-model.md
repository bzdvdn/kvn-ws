# DNS-зависимая маршрутизация — Модель данных

## Scope

- Связанные `AC-*`: `AC-001`, `AC-002`, `AC-003`, `AC-004`, `AC-005`, `AC-060`
- Статус: `no-change`

## No-Change Stub

- Статус: `no-change`
- Причина: фича не добавляет и не меняет persisted entities.
  - `config.RoutingCfg.ExcludeDomains`/`IncludeDomains` уже существуют — suffix-домены (`.ru`, `.ozon.ru`) кладутся туда же
  - `DomainMatcher.suffixes` и `RuleSet.suffixDomains` — in-memory только, не сериализуются
  - DNS question parser — stateless функция, без состояния
- Revisit triggers:
  - добавляется новый persisted-конфиг для `dns_intercept` toggle
  - suffix-домены требуют отдельного поля в YAML (например, `exclude_suffixes` вместо `exclude_domains`)
  - появляется кэш DNS question результатов
