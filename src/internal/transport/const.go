package transport

// Path is the legacy HTTP/2 TCP stream endpoint. 2.0 clients use DefaultPath
// and carry the target in :authority; the server dual-accepts this path.
const Path = "/api/v2/stream"

// UDPPath is the legacy HTTP/2 best-effort UDP flow endpoint.
const UDPPath = "/api/v2/udp"

// TargetHeader carries the requested target for legacy clients. New clients
// use the HTTP/2 :authority (Host) instead.
const TargetHeader = "X-KVN-Target"

// MaxDatagramSize bounds one UDP payload.
const MaxDatagramSize = 65507

// DefaultMaxStreams bounds concurrent streams per server process.
const DefaultMaxStreams = 4096

// TokenLookup validates a bearer token and returns its name.
type TokenLookup func(token string) (string, bool)
