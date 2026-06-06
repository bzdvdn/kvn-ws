// @sk-task quic-transport#T1.1: StreamConn interface for transport abstraction (AC-001, AC-004)
// @sk-task arch-refactoring#T2.2: type alias to transport.StreamConn (AC-003)
package tunnel

import "github.com/bzdvdn/kvn-ws/src/internal/transport"

type StreamConn = transport.StreamConn
