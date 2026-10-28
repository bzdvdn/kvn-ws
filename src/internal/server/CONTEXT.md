# src/internal/server

## Purpose
v2 fallback site for unauthenticated/unknown HTTP/2 requests.

## Notes
- Used by `cmd/server` through `transport.NewServer`.
- Must not expose KVN-specific headers.
