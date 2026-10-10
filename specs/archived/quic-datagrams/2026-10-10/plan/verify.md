---
report_type: verify
slug: quic-datagrams
status: pass
docs_language: ru
generated_at: 2026-10-10
---

# Verify Report: quic-datagrams

## Scope

- snapshot: проверка фичи quic-datagrams — real-time UDP как unreliable QUIC datagrams, аддитивная capability-негоциация, fallback на stream, настраиваемость через kvn-web, без изменений Android/WS.
- verification_mode: default
- artifacts:
  - CONSTITUTION.md (summary)
  - specs/active/quic-datagrams/spec.md
  - specs/active/quic-datagrams/plan.md
  - specs/active/quic-datagrams/tasks.md
- inspected_surfaces:
  - src/internal/transport/quic/{conn,listen,dial,obfuscated}.go
  - src/internal/transport/transport.go
  - src/internal/tunnel/session.go
  - src/internal/bootstrap/client/tun.go, src/internal/bootstrap/server/handler.go
  - src/internal/config/{client,server}.go
  - src/internal/webui/frontend/src/{types.ts,TabbedForm.tsx}, src/internal/webui/handler_connect.go
  - src/integration/tunnel_integration_test.go
  - docs/ru/config.md, docs/en/config.md

## Verdict

- status: pass
- archive_readiness: safe
- summary: 14/14 задач `[x]` с валидными Proof (14 записей, 0 пропусков); 4 AC подтверждены фокусными тестами (все PASS), UI/merge kvn-web и docs на месте; обратная совместимость покрыта тестом legacy-peer.

## Checks

- task_state: completed=14, open=0; still-open: none
- proof_state: PROOFS_TOTAL=14, PROOFS_MISSING=0 (verify-task-state.sh); trace — все файлы/якоря резолвятся
- acceptance_evidence:
  - AC-001 -> T1.1, T3.1, T3.2, T4.1, T4.3
  - AC-002 -> T3.1, T3.2, T4.1
  - AC-003 -> T2.1, T2.2, T2.3, T4.1
  - AC-004 -> T2.3, T4.2, T4.3
- implementation_alignment:
  - quic/conn.go:SendDatagram/ReceiveDatagram + withDatagrams(EnableDatagrams) в Listen/Dial — датаграммы включены на обеих сторонах; TestQUICDatagramRoundTrip/TooLarge PASS.
  - tunnel/session.go:sendDatagram — UDP уходит датаграммой (сырой IP-пакет, cipher при наличии), иначе stream; DatagramTooLarge → фоллбэк; TestSendDatagramSelection PASS.
  - tunnel/session.go:startDatagramReader — входящие датаграммы в TUN, не роняет сессию на потерях.
  - quic/obfuscated.go:SendDatagram/ReceiveDatagram — XOR тем же nonce, что stream; TestObfuscatedDatagramRoundTrip PASS.
  - bootstrap: client шлёт `Transport="quic-dgram"` при `transport=quic`+флаг; server эхом подтверждает и вызывает SetDatagrams; TestDatagramCapableMarker/TestDatagramTransportHandshakeRoundTrip PASS.
  - kvn-web: чекбокс UDP Datagrams (Advanced/Global) + mergeConfig per-server override; `tsc -b && vite build` OK.

## Verification Matrix

| AC-ID | Task IDs | Evidence | Verdict |
|-------|----------|----------|---------|
| AC-001 | T1.1, T3.1, T3.2, T4.1, T4.3 | TestQUICDatagramRoundTrip: PASS; TestSendDatagramSelection: PASS; TestQUICDatagramAndStreamIsolation: PASS; TabbedForm.tsx/mergeConfig udp_datagrams | pass |
| AC-002 | T3.1, T3.2, T4.1 | TestQUICDatagramLossDoesNotBlockStream: PASS (2000 unread datagrams, stream echo < 1s); TestQUICDatagramAndStreamIsolation: PASS | pass |
| AC-003 | T2.1, T2.2, T2.3, T4.1 | TestDatagramCapableMarker: PASS; TestDatagramTransportHandshakeRoundTrip: PASS (legacy peer → non-capable) | pass |
| AC-004 | T2.3, T4.2, T4.3 | TestDatagramTransportHandshakeRoundTrip (legacy): PASS; docs ru/en `udp_datagrams`; WS/websocket suite green; mergeConfig kvn-web | pass |

## Errors

- none

## Warnings

- Латентная асимметрия ServerHello encode/decode: `EncodeServerHello` пишет MTU только при `hasMTU`, а `DecodeServerHello` читает 2 байта MTU всегда. В реальности сервер всегда шлёт Mtu>0 (дефолт 1400), поэтому не проявляется; вне scope этой фичи — кандидат на отдельный fix.

## Questions

- none

## Not Verified

- Количественный порог p99 (≤ RTT+50 мс) под реальной сетевой потерей датаграмм: локальный loopback-тест аппроксимирует потерю (нечитаемые датаграммы) и подтверждает независимость stream, но не измеряет p99 в реальной сети.
- Живой прогон на удалённом сервере (`transport: quic`): проверено на loopback через транспортный слой.
- SC-001/SC-002/SC-003 (p99, throughput, soak/speedtest) в этом окружении не прогонялись.

## Next Step

- safe to archive
