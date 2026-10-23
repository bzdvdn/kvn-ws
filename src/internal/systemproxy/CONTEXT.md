# Context — src/internal/systemproxy

## Purpose

Включает и восстанавливает системный HTTP/HTTPS-прокси клиента: переменные окружения процесса плюс платформенные настройки (systemd override на Linux, `networksetup` на macOS, реестр Windows), чтобы приложения без своей proxy-настройки шли через локальный SOCKS/HTTP-listener KVN.

## Responsibilities

- Владеет: `State` (сохранение/восстановление `HTTP(S)_PROXY`/`NO_PROXY` в обоих регистрах), интерфейсом `PlatformManager` и тремя платформенными реализациями, `NOProxyBuilder`.
- Не владеет: самим proxy-listener'ом (`proxy`), решением включать ли системный прокси (`config.SystemProxy`), маршрутизацией (`routing`).

## How it works

- `New(cfg)` создаёт `State` с платформенным менеджером (build-tag: linux/darwin/windows, иначе stub); сам `cfg` не используется.
- `State.Set(ctx, logger, addr, noProxy)`: recovery-эвристика — если текущий `HTTP_PROXY` уже равен нашему URL, считает его «осиротевшим» после краха и сбрасывает все шесть переменных; затем сохраняет оригиналы (`LookupEnv`) и выставляет `HTTP_PROXY/http_proxy/HTTPS_PROXY/https_proxy = http://<addr>`, а `NO_PROXY/no_proxy` — только если `noProxy != ""`.
- После env вызывается `platform.Set`; его ошибка **логируется, но не возвращается** — env-часть считается успешной.
- `State.Restore` возвращает каждую переменную к сохранённому значению или удаляет её, затем вызывает `platform.Restore` (ошибки тоже только warn).
- `NOProxyBuilder(routing)`: CIDR из `exclude_ranges` (валидные `netip.Prefix`), IP из `exclude_ips`, домены из `exclude_domains` — домены без ведущей точки получают её (`.example.com`); всё склеивается через запятую.
- Linux: пишет `/etc/systemd/system/kvn-client.service.d/system-proxy.conf` с `Environment=...` (best-effort; ошибки создания/записи → warn и `nil`), `Restore` удаляет файл.
- macOS: определяет активный network service, сохраняет web/secure-web proxy, включает оба через `networksetup`, `Restore` возвращает сохранённые значения или выключает.
- Windows: пишет `ProxyEnable/ProxyServer/ProxyOverride` в `Internet Settings` (HKCU или USERS\<SID> при elevated) и дергает `InternetSetOption` для применения; `Restore` возвращает прежние значения.

## Key files

- `src/internal/systemproxy/systemproxy.go` — `State`, `Set`/`Restore`, `NOProxyBuilder`, env-константы.
- `src/internal/systemproxy/proxy_linux.go` — systemd override.
- `src/internal/systemproxy/proxy_darwin.go` — `networksetup`.
- `src/internal/systemproxy/proxy_windows.go` — реестр + WinINet refresh.
- `src/internal/systemproxy/proxy_stub.go` — заглушка для прочих ОС.

## Public contracts

- `New(cfg *config.ClientConfig) *State`, `(*State).Set(ctx, logger, addr, noProxy string) error`, `(*State).Restore(ctx, logger) error`.
- `NOProxyBuilder(cfg *config.RoutingCfg) string` — значение для `NO_PROXY`.
- `PlatformManager interface { Set(ctx, logger, addr, noProxy) error; Restore(ctx, logger) error }`.
- Переменные: `HTTP_PROXY`, `http_proxy`, `HTTPS_PROXY`, `https_proxy`, `NO_PROXY`, `no_proxy`; URL прокси — `http://<addr>`.

## Dependencies

- `src/internal/config` (тип `RoutingCfg`), `go.uber.org/zap`, `golang.org/x/sys/windows/registry` (Windows).

## Consumed by

- `src/internal/bootstrap/client/client.go` — в proxy-режиме при `system_proxy: true`, с `defer Restore`.

## Gotchas

- `Set`/`Restore` меняют переменные окружения **всего процесса**: дочерние процессы наследуют прокси, а параллельные клиенты в одном процессе будут конфликтовать.
- Ошибки платформенного слоя не пробрасываются — `Set` вернёт `nil`, даже если системный прокси не включился; проверять только логи.
- Linux-менеджер пишет unit-override `kvn-client.service`, но не делает `systemctl daemon-reload`/`restart` — файл влияет только на будущие запуски сервиса.
- macOS сохраняет настройки в момент `Set`; если между `Set` и `Restore` пользователь изменил прокси, `Restore` перезапишет его старыми значениями.
- Windows при отсутствии прав пишет в HKCU; при elevated — в профиль активного пользователя (`USERS\<SID>`), и `InternetSetOption` вызывается в обоих случаях.
- `NOProxyBuilder` добавляет ведущую точку к доменам без неё; домены с уже ведущей точкой сохраняются как есть; невалидные CIDR/IP молча отбрасываются.
- Recovery-эвристика срабатывает только при точном совпадении `HTTP_PROXY` с нашим URL — прокси, оставшийся от другой копии/порта, не распознаётся.
- Если `noProxy` пуст, существующий `NO_PROXY` не перезаписывается, но и не очищается — восстановление вернёт исходное значение.
