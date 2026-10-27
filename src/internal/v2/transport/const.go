package transport

// Path is the standard HTTP/2 TCP stream endpoint.
const Path = "/api/v2/stream"

// UDPPath is the HTTP/2 best-effort UDP flow endpoint.
const UDPPath = "/api/v2/udp"

// TargetHeader carries the requested TCP/UDP target on an authenticated stream.
const TargetHeader = "X-KVN-Target"

// MaxDatagramSize bounds one UDP payload.
const MaxDatagramSize = 65507

// DefaultMaxStreams bounds concurrent streams per server process.
const DefaultMaxStreams = 4096

// TokenLookup validates a bearer token and returns its name.
type TokenLookup func(token string) (string, bool)
