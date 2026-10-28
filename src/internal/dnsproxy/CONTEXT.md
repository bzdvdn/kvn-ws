# Context — src/internal/dnsproxy

## Purpose

Локальный UDP DNS-прокси для v2 desktop TUN: принимает запросы на loopback, отправляет exclude-домены в direct-резолверы, остальные — по TCP к configured upstreams, и трекает IP↔домен.

## Responsibilities

- Владеет: UDP server (`Server`), route-by-domain, direct UDP resolvers, TCP fallback к upstreams, `dns.Tracker`, Linux resolv.conf override/restore.
- Не владеет: v2 transport/stream, TUN-маршрутами и правилами split-tunnel.

## How it works

- `New(listenAddr, upstreams...)`: default `1.1.1.1:53`, `8.8.8.8:53`.
- `Run(ctx)` слушает UDP; каждый запрос обрабатывается в `forward` под snapshot конфигурации.
- `routeDirect(qname)` → `resolveDirect` через orig resolvers (UDP, 3с на попытку, перебор).
- Иначе — DNS-over-TCP к upstreams (2-byte length prefix, ответ ≤1500); ответ отправляется клиенту и трекается.
- Linux: `BackupResolvConf` → `OverrideResolvConf` → `Restore`; `CleanupStaleDNS` убирает stale proxy nameserver.

## Public contracts

- `New`, `Run`, `Shutdown`; `SetRouteFunc`, `SetOrigResolvers`, `SetTracker`, `SetDirectRouteFunc`.
- Linux: `BackupResolvConf`, `OverrideResolvConf`, `Restore`, `CleanupStaleDNS`.

## Gotchas

- Нет ответа при пустом upstreams или ошибке всех резолверов.
- TCP-fallback ограничен 1500 байтами.
- `extractDNSDomain` не поддерживает compression pointers.
- `OverrideResolvConf` не делает backup сам; порядок Backup → Override → Restore обязателен.
