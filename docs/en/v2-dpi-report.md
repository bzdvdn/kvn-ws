# v2 DPI/active-probe report

- Date: 2026-10-28
- Target: `88.210.63.157:443`, SNI `131242.koara.live`
- Network: `lab`, region: `local`
- Script: `scripts/dpi-check.sh`

## Result

`RESULT: PASS` — TLS 1.3, no KVN-specific headers on `/`, `/robots.txt`, `/favicon.ico`, `/api/health`, `/api/v2/stream`; invalid-token active probe returns fallback HTML.

## Limitation

This is a lab run against the currently deployed server. The field run in the target region (RU) and against the new v2-only server is still pending; without it we can claim "no obvious fingerprints", not "bypasses DPI".

## Field run

- Date: 2026-10-29
- Network: user field network (exact ISP not recorded)
- Region: RU (target region)
- Result: PASS — the user confirmed that the v2 connection works in their network.
- Limitation: this confirms connectivity; no separate pcap/active-probe capture was taken from that network. Active-probe/fallback above were verified in the lab run.
