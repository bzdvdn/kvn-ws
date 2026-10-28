# src/internal/tunpool

## Purpose
Unique IPv4 TUN lease allocator for v2 clients.

## Contract
- `New(Config)` validates subnet/range/gateway/TTL and loads `LeaseFile` if set.
- `Lease(clientID)` returns the same active IP for a client, otherwise a free IP.
- `Touch(clientID)` renews on session/streams.
- Expired leases are pruned greedily; store is atomic JSON.
