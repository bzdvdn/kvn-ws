# Windows TUN Device Support — Модель данных

## Scope

- Связанные `AC-*`: none
- Связанные `DEC-*`: none
- Статус: `no-change`
- Причина: фича не добавляет и не меняет persisted entities, value objects, state transitions или contract-relevant payload shapes. Вся работа — platform-specific реализация существующего интерфейса `TunDevice`, который уже определён в `tun_common.go`. Новые типы (`LUID string`, `windows.GUID`) — transient и не покидают границ `tun_windows.go`.

## Revisit triggers

- появляется новое сохраняемое состояние (например, кеш NLA profiles)
- API/event payload shape меняется
- данные о TUN-адаптере нужно сохранять между перезапусками
