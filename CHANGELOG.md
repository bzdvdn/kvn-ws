<!-- @sk-task docs-and-release#T1.2: CHANGELOG v1.0.0 (AC-007) -->

# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [1.0.0] — 2026-05-14

### Added

- VPN-туннель через WebSocket Binary Frames поверх TLS 1.3
- TUN-интерфейс на стороне клиента
- IP-пул с динамическим выделением (IPv4 + IPv6)
- Сессионный менеджмент с BoltDB-персистентностью
- Гибкая маршрутизация: server/direct, CIDR, DNS-имена, отдельные IP
- Ordered rules для конфликтующих маршрутов
- DNS-резолвер с in-memory TTL-кэшем
- Аутентификация: token-based, JWT, basic
- Keepalive (PING/PONG) и контроль сессий
- nftables MASQUERADE для server-side NAT
- Prometheus-метрики (active_sessions, throughput, errors)
- App-layer encryption (AES-256-GCM) для Data-фреймов, per-session key derivation через HMAC-SHA256
- SOCKS5 + HTTP CONNECT proxy listener
- Docker multi-stage build
- docker-compose оркестрация
- Документация на английском и русском
