package client

import (
	"context"
	"encoding/binary"
	"io"
	"net"
	"time"

	"go.uber.org/zap"

	"github.com/bzdvdn/kvn-ws/src/internal/transport"
	"github.com/bzdvdn/kvn-ws/src/internal/tun"
)

func isICMPv4(packet []byte) bool {
	return len(packet) >= 20 && packet[0]>>4 == 4 && packet[9] == 1
}

// handlePing forwards one IPv4 ICMP echo request over a v2 HTTP/2 stream and
// writes the echo reply back to TUN.
func (c *Client) handlePing(parent context.Context, httpClient *transport.Client, packet []byte, dev tun.TunDevice) {
	ihl := int(packet[0]&0x0f) * 4
	if ihl < 20 || len(packet) < ihl+8 || packet[ihl] != 8 {
		return
	}
	target := net.IP(packet[16:20]).To4()
	if target == nil {
		return
	}
	c.logger.Debug("v2 ping", zap.String("target", target.String()))
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	stream, err := httpClient.DialICMP(ctx, target.String())
	if err != nil {
		return
	}
	defer stream.Close()
	if _, err := stream.Write(packet[ihl:]); err != nil {
		return
	}
	_ = stream.CloseWrite()
	reply, err := io.ReadAll(io.LimitReader(stream, 64<<10))
	if err != nil || len(reply) < 8 || reply[0] != 0 {
		return
	}
	_, _ = dev.Write(buildPingReply(packet, reply, ihl))
}

func buildPingReply(req, reply []byte, ihl int) []byte {
	total := 20 + len(reply)
	out := make([]byte, total)
	copy(out[:20], req[:20])
	out[0] = 0x45
	binary.BigEndian.PutUint16(out[2:4], uint16(total)) // #nosec G115 -- ICMP reply is bounded to 64 KiB
	out[8] = 64
	out[9] = 1
	copy(out[12:16], req[16:20])
	copy(out[16:20], req[12:16])
	binary.BigEndian.PutUint16(out[10:12], 0)
	binary.BigEndian.PutUint16(out[10:12], ipChecksum(out[:20]))
	copy(out[20:], reply)
	return out
}
