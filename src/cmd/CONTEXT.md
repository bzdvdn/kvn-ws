# src/cmd

## Entry Points
- `client` — `kvn-client`, v2 standalone TUN/proxy client.
- `server` — `kvn-server`, v2 HTTP/2/QUIC server + TUN lease pool + ICMP proxy.
- `web` — `kvn-web`, v2-only UI.
- `desktop` — native WebView wrapper for `kvn-web`.

## Notes
- `kvn-web` uses unified `~/.config/kvn/config.yaml` (old v1 → `.v1.bak`).
- No v1 entrypoints exist; v1 доступен только в старых GitHub Releases.
- Windows installer defaults to `kvn-desktop` + `kvn-web` autostart and cleans v1.
