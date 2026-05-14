# Tun Data Path + Docker Production

## Scope Snapshot

- In scope: TUN I/O (read/write), nftables/NAT в Docker, production-ready Dockerfile, сквозной ping.
- Out of scope: IPv6, производительность, CI/CD.

## Цель

После handshake клиент получает IP, но forwarding сразу падает с `tun write: invalid offset`. Пользователь не может передать ни одного пакета через туннель. Цель — починить read/write forwarding loops, чтобы ping с клиента на gateway (10.10.0.1) проходил.

## Основной сценарий

1. Клиент и сервер выполняют handshake, клиент получает IP 10.10.0.2.
2. Клиент запускает forwarding loop: TUN Read → WebSocket Write.
3. Сервер запускает forwarding loop: WebSocket Read → TUN Write.
4. Пакет (например ICMP echo) поступает в TUN клиента → инкапсулируется → отправляется на сервер → сервер пишет в TUN.
5. Ответный пакет проходит обратный путь.
6. Клиент видит reply на ping 10.10.0.1.

## Контекст

- `tunDevice.Write()` использует `t.device.Write([][]byte{buf}, 0)` — offset=0 может быть некорректен для Linux TUN.
- `tunDevice.Read()` распределяет один buf на все batch-слоты — возможна коллизия, если TUN возвращает >1 пакета за read.
- WireGuard TUN library требует alignment для virtio — Linux TUN работает с 4-byte header (AF_INET family).
- Server и client forwarding loop идентичны по структуре, ошибка проявляется на обоих.

## Требования

- RQ-001 `tunDevice.Read()` ДОЛЖЕН возвращать одиночный валидный IP-пакет без `invalid offset` / `invalid argument`.
- RQ-002 `tunDevice.Write()` ДОЛЖЕН принимать сырой IP-пакет и успешно передавать его TUN-устройству.
- RQ-003 Forwarding loop (Read→Write) ДОЛЖЕН работать без потери соединения при передаче ≥1 пакета.
- RQ-004 При закрытии WebSocket forwarding loop ДОЛЖЕН завершаться без panic/deadlock.

## Критерии приемки

### AC-001 TUN write error fixed

- **Given** клиент получил IP через handshake
- **When** forwarding loop запускается и первый пакет идёт в `tunDevice.Write()`
- **Then** ошибка `invalid offset` не возникает, пакет записан
- Evidence: client log показывает `forwarding started`, а не `forwarding stopped with error`

### AC-002 TUN read returns valid packets

- **Given** TUN интерфейс активен
- **When** `tunDevice.Read()` вызывается
- **Then** возвращается валидный IP-пакет (может быть ARP/DHCP/ICMP) без ошибки
- Evidence: `go test` для `tun.Read` с mock-пакетом проходит

### AC-003 Ping через туннель

- **Given** сервер и клиент запущены в Docker, handshake завершён
- **When** `docker exec client ping -c 1 10.10.0.1`
- **Then** пинг успешен (0% packet loss)
- Evidence: `ping` output показывает `1 received, 0% packet loss`

### AC-004 Session cleanup

- **Given** активная forwarding сессия
- **When** WebSocket закрывается (graceful или по ошибке)
- **Then** обе goroutine в forwarding loop завершаются, TUN закрывается
- Evidence: `go test -race` не показывает data race, pprof не показывает goroutine leak

### AC-005 nftables установлен в Docker

- **Given** образ собран через `docker compose build`
- **When** `docker run --rm go_kvn-server which nft`
- **Then** возвращает `/usr/sbin/nft`
- Evidence: `which nft` успешен

### AC-006 NAT MASQUERADE работает

- **Given** сервер и клиент запущены, handshake завершён
- **When** `docker exec client ping -c 1 10.10.0.1`
- **Then** пинг проходит (0% loss)
- Evidence: ping output показывает `1 received, 0% packet loss`

### AC-007 Dockerfile production-ready

- **Given** `docker images go_kvn-server`
- **Then** размер < 50 MB
- **And** `ip` и `nft` доступны внутри контейнера
- Evidence: `docker images` + `docker exec`

### AC-008 Smoke test проходит

- **Given** свежий `git clone`
- **When** `cp -r examples/* . && bash examples/run.sh`
- **Then** `SUCCESS: Client connected!`
- Evidence: stdout вывода run.sh

## Вне scope

- IPv6 data path
- CI/CD pipeline
- Performance tuning, bench, MTU discovery
- Production metrics/alerting

## Допущения

- TUN-устройство создаётся успешно (с фиксом MTU=1400 в `CreateTUN`).
- `ip` команда доступна в контейнере (alpine + iproute2).
- `/dev/net/tun` проброшен в контейнер через `--privileged`.
- Root-причина `invalid offset` — неправильный buffer offset/alignment в Write, а не в TUN device.

## Открытые вопросы

1. Какой минимальный offset нужен для `tun.Device.Write()` на Linux? Возможно 4 байта для AF_INET header.
2. Нужно ли менять batch-oriented Read на single-buf для устранения коллизии?
3. Нужен ли mock TUN device для unit-тестов без /dev/net/tun?
