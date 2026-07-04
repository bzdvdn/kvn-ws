---
report_type: verify
slug: dns-upstreams-list
status: pass
docs_language: ru
generated_at: 2026-07-04
---

# Verify Report: dns-upstreams-list

## Scope

- snapshot: замена `upstream string` → `upstreams []string` во всех компонентах (config, dnsproxy, tunnel session, bootstrap, webui) с backward compat и fallback
- verification_mode: default
- artifacts:
  - CONSTITUTION.md
  - specs/active/dns-upstreams-list/tasks.md
  - specs/active/dns-upstreams-list/spec.md
- inspected_surfaces:
  - `src/internal/config/client.go` — DNSProxyCfg, RelayDNSCfg, DefaultDNSUpstreams, custom marshal/unmarshal, LoadClientConfig/LoadRelayConfig
  - `src/internal/config/server.go` — ServerConfig.DNSUpstreams, LoadServerConfig
  - `src/internal/config/webui.go` — defaultWebUIConfig
  - `src/internal/dnsproxy/dnsproxy.go` — variadic New, fallback loop
  - `src/internal/tunnel/session.go` — dnsUpstreams field, forwardDNS fallback
  - `src/internal/bootstrap/client/tun.go` — upstreams → dnsproxy.New
  - `src/internal/bootstrap/server/handler.go` — DNSUpstreams → NewSession
  - `src/internal/webui/handler_connect.go` — mergeConfig
  - `src/internal/webui/frontend/src/App.tsx` — upstreams UI
  - `docs/ru/config.md`, `docs/en/config.md` — поле upstreams

## Verdict

- status: pass
- archive_readiness: safe
- summary: все 10 AC подтверждены тестами или code inspection; 11 задач из 11 отмечены [x]; traceability полная (35 code + 15 test markers)

## Checks

- task_state: completed=11, open=0
- acceptance_evidence:
  - AC-001 (новый upstreams работает) → `TestDNSProxyCfgUpstreams` PASS
  - AC-002 (старый upstream работает) → `TestDNSProxyCfgBackwardCompat` PASS
  - AC-003 (дефолты при пустом конфиге) → `TestDNSProxyCfgDefaults` + `TestDefaultDNSUpstreams` PASS
  - AC-004 (server-side DNS upstreams) → `TestServerDNSUpstreams` + `TestServerDNSUpstreamsDefaults` PASS
  - AC-005 (fallback при недоступности первого) → `TestDNSProxyFallback` PASS (mock upstream получает запрос после отказа первого)
  - AC-006 (server-side forward использует конфиг) → `TestServerDNSForwardUsesConfig` PASS (mock UDP upstream подтверждает приём)
  - AC-007 (relay backward compat) → `TestRelayDNSUpstreamBackwardCompat` PASS
  - AC-008 (WebUI roundtrip JSON) → `TestDNSProxyCfgJSONRoundTrip` PASS
  - AC-009 (mergeConfig корректный) → `TestMergeConfigDNSUpstreams` + `TestMergeConfigDNSUpstreamsNotOverridden` PASS
  - AC-010 (WebUI backward compat JSON) → `TestDNSProxyCfgJSONBackwardCompat` PASS
- implementation_alignment:
  - DNSProxyCfg: `Upstreams []string` + custom UnmarshalYAML/JSON (backward compat) + DefaultDNSUpstreams
  - RelayDNSCfg: `Upstreams []string` + поле Upstream для backward compat
  - ServerConfig: `DNSUpstreams []string` с дефолтом DefaultDNSUpstreams
  - dnsproxy.New: variadic (upstreams ...string), fallback-цикл в forward
  - Session: `dnsUpstreams []string`, forwardDNS с fallback по порядку
  - WebUI App.tsx: upstreams []string в типе + add/remove в UI
  - Документация обновлена: оба языка

## Errors

- none

## Warnings

- none

## Questions

- none

## Not Verified

- AC-008 (WebUI upstreams roundtrip через UI) — проверен только JSON roundtrip тестом, полный e2e через браузер не проводился (out of scope для unit verify)
- AC-010 (WebUI backward compat через UI) — проверен JSON backward compat тестом

## Next Step

- safe to archive
