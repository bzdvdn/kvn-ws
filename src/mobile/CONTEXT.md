# src/mobile

## Purpose
gomobile AAR boundary для Android v2.

## API
- `Resolve(configJSON)` — authenticated TUN assignment.
- `Start(configJSON, fd)` — v2 data plane на VpnService fd.
- `Client.Stop`, `Client.Error`, `Client.RxBytes`, `Client.TxBytes`.

## Contract
- Kotlin владеет VpnService address/routes/DNS и fd; Go владеет HTTP/2/QUIC packet pump.
- `quic_enabled` и `quic_endpoint` приходят из Android `ConnectionConfig`.
- Exclude-domain DNS/direct routing — `internal/client/direct.go`.
- `Stop` идемпотентен; Go не закрывает Kotlin-owned fd.
