# Whitelist & Obfuscation Hardening — Модель данных

## Scope

- Связанные `AC-*`: `AC-001`, `AC-002`, `AC-003`, `AC-004`, `AC-005`, `AC-006`
- Связанные `DEC-*`: `DEC-001`, `DEC-002`, `DEC-003`, `DEC-004`
- Статус: `changed`

## Сущности

### DM-001 ObfuscationCfg

- Назначение: конфигурация обфускации и anti-DPI мер. Заменяет `Obfuscation bool` в `ClientConfig`.
- Источник истины: `config/client.go`
- Инварианты: если `enabled: false`, остальные поля игнорируются
- Связанные `AC-*`: все
- Связанные `DEC-*`: `DEC-002`
- Поля:
  - `enabled` - `bool`, required. Включает obfuscation. `obfuscation: true` → `{enabled: true}`
- Жизненный цикл: загружается из YAML/env при старте, сохраняется через Web UI

### DM-002 ClientTLSCfg (расширение)

- Назначение: существующий `ClientTLSCfg` получает новые поля.
- Источник истины: `config/client.go`
- Поля:
  - `utls` - `UTLSCfg`, optional. Настройки uTLS.
  - `sni` - `[]string`, optional. Список доменов для SNI. Выбирается рандомно при каждом подключении.
- Жизненный цикл: то же, что у `ClientConfig`

### DM-003 UTLSCfg

- Назначение: под-конфиг uTLS.
- Источник истины: `config/client.go`
- Поля:
  - `enabled` - `bool`. Включить uTLS для WS.
  - `fallback` - `bool`, default `true`. При ошибке uTLS — `crypto/tls`.
- Жизненный цикл: загружается из YAML/env при старте

### DM-004 PaddingCfg

- Назначение: конфигурация padding для WS.
- Источник истины: `config/client.go`
- Связанные `AC-*`: `AC-005`
- Поля:
  - `enabled` - `bool`
  - `size` - `int`, default `512`. Размер выравнивания.
- Жизненный цикл: загружается из YAML/env при старте

### DM-005 ServerConfig (расширение)

- Назначение: существующий `ServerConfig` получает поле `ws_paths`.
- Источник истины: `config/server.go`
- Связанные `AC-*`: `AC-003`
- Поля:
  - `ws_paths` - `[]string`, optional. Разрешённые WS paths. Default: `["/tunnel"]`.
- Жизненный цикл: загружается при старте сервера

## Связи

- `ClientConfig.Obfuscation` → `ObfuscationCfg` (one-to-one)
- `ClientConfig.TLS` → `ClientTLSCfg` → `UTLSCfg` (nested)
- `ObfuscationCfg.Padding` → `PaddingCfg` (nested, optional)

## Производные правила

- `obfuscation: true` (bool) в YAML → `ObfuscationCfg{Enabled: true}` (кастомный decoder)

## Переходы состояний

Нет — все поля immutable после загрузки конфига.

## Вне scope

- Persisted state (БД, файлы) не меняется
- Сериализация конфига для Web UI — существующий механизм (`mapstructure.Decode` + `yaml.Marshal`)
