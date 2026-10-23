# Context — src/internal/nat

## Purpose

Настройка серверного NAT (MASQUERADE) для трафика, выходящего из TUN-интерфейса `kvn`: автоматический выбор nftables, при недоступности — fallback на iptables/iptables-legacy.

## Responsibilities

- Владеет: интерфейсом `Manager` (`Setup/Setup6/Teardown/Teardown6`), реализациями `NFTManager` и `IPTablesManager`, детектом бинарников.
- Не владеет: TUN-устройством (`tun`), IP-forwarding (`bootstrap/relay` включает его отдельно через sysctl), маршрутизацией пакетов.

## How it works

- `NewManager()` проверяет `nft --version`: успех — `NewNFTManager()`, иначе `NewIPTablesManager()`.
- nftables: `Setup` добавляет таблицу `ip kvn-nat`, chain `postrouting` с hook `srcnat` и правило `iifname "kvn" masquerade`; `Setup6` — то же в `ip6 kvn-nat`; teardown удаляет таблицу целиком.
- iptables: `Setup` создаёт цепочку `KVN_NAT`, правило `-i kvn -j MASQUERADE`, jump из `POSTROUTING`; teardown удаляет jump, чистит и удаляет цепочку; `Setup6/Teardown6` — через `ip6tables`.
- `detectIptables()` пробует `iptables`, затем `iptables-legacy`; `detectIp6tables()` — `ip6tables`, затем `ip6tables-legacy`.
- Все команды — `exec.Command`; nft-ошибки включают stderr в текст ошибки, iptables-ошибки — только exit status.
- Вызывается из `bootstrap/server` (`Setup`/`Setup6`) и при shutdown (`Teardown*`); relay использует собственный in-process NAT (`bootstrap/relay/nat.go`).

## Key files

- `src/internal/nat/nat.go` — `Manager`, `NewManager`, детект nft.
- `src/internal/nat/nftables.go` — `NFTManager`.
- `src/internal/nat/iptables.go` — `IPTablesManager`, детект бинарников.

## Public contracts

- `Manager interface { Setup() error; Setup6() error; Teardown() error; Teardown6() error }`.
- `NewManager() Manager`, `NewNFTManager() *NFTManager`, `NewIPTablesManager() *IPTablesManager`.
- Имена в системе: таблицы `kvn-nat` (ip/ip6), chain `KVN_NAT` (iptables), интерфейс `kvn`.

## Dependencies

- Только стандартная библиотека (`os/exec`, `strings`, `fmt`).

## Consumed by

- `src/internal/bootstrap/server` — `nat.NewManager()`, `Setup/Setup6` при старте, `Teardown*` при остановке.

## Gotchas

- Требуются root/`CAP_NET_ADMIN`; ошибки `Setup*` в сервере только логируются, сервер продолжает работу без NAT.
- `NewManager` выбирает nft по наличию бинарника, а не по работоспособности правил: nft может присутствовать, но таблица/хук — не примениться.
- `Setup` неидемпотентен: повторный запуск на существующей таблице/цепочке вернёт ошибку (`add table`/`-N`); после падения процесса остатки нужно убирать `Teardown*` вручную.
- `Teardown` iptables глотает ошибки (все три команды без проверки) и всегда возвращает `nil`; nft teardown возвращает ошибку, если таблицы нет.
- `Setup6` iptables использует ту же цепочку `KVN_NAT` в таблице `nat` для `ip6tables` — это отдельное пространство, конфликта с IPv4 нет, но имена в выводе одинаковые.
- Правило привязано к жёсткому имени интерфейса `kvn` (создаётся `tun.CreateTUN("kvn", ...)`); переименование TUN сломает NAT.
- Пакет ничего не знает про IPv6-включён ли он: `Setup6` вызывается только если в конфиге задан IPv6-пул.
