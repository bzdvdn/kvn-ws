# src/internal/server

## Purpose
Public cover site and fallback for unauthenticated/unknown HTTP requests.

## Notes
- `fallback.dir` serves a customizable static site; without it, the embedded
  "NOVA" streaming site is served.
- `kvn-server --export-cover-site <dir>` extracts the embedded assets.
- `transport.NewServer` routes HTTP/1.1, unknown paths and invalid tokens here.
- Must not expose KVN-specific headers, `pad=` or `application/octet-stream`.
- Cover assets live in `cover_assets/` (embedded via `embed.FS`).
