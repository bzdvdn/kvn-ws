package client

import (
	"context"
	"encoding/binary"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"

	"github.com/bzdvdn/kvn-ws/src/internal/tun"
)

const (
	udpFlowIdle     = 30 * time.Second
	udpFlowQueue    = 256
	udpCleanupEvery = 10 * time.Second
)

// packetFlow is implemented by HTTP/2 and QUIC datagram flows.
type packetFlow interface {
	WritePacket([]byte) error
	ReadPacket() ([]byte, error)
	Close() error
}

type packetDialer func(context.Context, string) (packetFlow, error)

type udpManager struct {
	ctx    context.Context
	dev    tun.TunDevice
	dial   packetDialer
	logger *zap.Logger

	mu    sync.Mutex
	flows map[string]*udpFlow
}

func newUDPManager(ctx context.Context, dev tun.TunDevice, dial packetDialer, logger *zap.Logger) *udpManager {
	m := &udpManager{ctx: ctx, dev: dev, dial: dial, logger: logger, flows: make(map[string]*udpFlow)}
	go m.cleanup()
	return m
}

func (m *udpManager) cleanup() {
	ticker := time.NewTicker(udpCleanupEvery)
	defer ticker.Stop()
	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			now := time.Now().UnixNano()
			m.mu.Lock()
			for key, f := range m.flows {
				if now-f.last.Load() > int64(udpFlowIdle) {
					delete(m.flows, key)
					f.stop()
				}
			}
			m.mu.Unlock()
		}
	}
}

// @sk-task kvn-v2-transport#T4.1: parse IPv4 UDP and route it to a v2 datagram flow (AC-008)
func (m *udpManager) Handle(packet []byte) {
	if len(packet) < 28 || packet[0]>>4 != 4 || packet[9] != 17 {
		return
	}
	ihl := int(packet[0]&0x0f) * 4
	if ihl < 20 || len(packet) < ihl+8 {
		return
	}
	srcIP := net.IP(packet[12:16])
	dstIP := net.IP(packet[16:20])
	srcPort := binary.BigEndian.Uint16(packet[ihl : ihl+2])
	dstPort := binary.BigEndian.Uint16(packet[ihl+2 : ihl+4])
	payload := packet[ihl+8:]
	key := string(srcIP) + ":" + itoa(int(srcPort)) + "-" + string(dstIP) + ":" + itoa(int(dstPort))

	m.mu.Lock()
	f := m.flows[key]
	if f == nil {
		f = &udpFlow{
			manager: m,
			key:     key,
			srcIP:   append(net.IP(nil), srcIP...),
			dstIP:   append(net.IP(nil), dstIP...),
			srcPort: srcPort,
			dstPort: dstPort,
			out:     make(chan []byte, udpFlowQueue),
		}
		f.last.Store(time.Now().UnixNano())
		m.flows[key] = f
		go f.run()
	}
	m.mu.Unlock()

	cp := make([]byte, len(payload))
	copy(cp, payload)
	select {
	case f.out <- cp:
		f.last.Store(time.Now().UnixNano())
	default:
		m.logger.Debug("v2 udp flow queue full", zap.String("dst", net.JoinHostPort(dstIP.String(), itoa(int(dstPort)))))
	}
}

func (m *udpManager) remove(key string, f *udpFlow) {
	m.mu.Lock()
	if m.flows[key] == f {
		delete(m.flows, key)
	}
	m.mu.Unlock()
}

type udpFlow struct {
	manager *udpManager
	key     string
	srcIP   net.IP
	dstIP   net.IP
	srcPort uint16
	dstPort uint16
	out     chan []byte
	last    atomic.Int64

	mu     sync.Mutex
	stream packetFlow
	once   sync.Once
}

func (f *udpFlow) run() {
	target := net.JoinHostPort(f.dstIP.String(), itoa(int(f.dstPort)))
	stream, err := f.manager.dial(f.manager.ctx, target)
	if err != nil {
		f.manager.logger.Debug("v2 udp flow dial failed", zap.String("target", target), zap.Error(err))
		f.manager.remove(f.key, f)
		return
	}
	f.mu.Lock()
	f.stream = stream
	f.mu.Unlock()
	defer func() {
		f.stop()
		f.manager.remove(f.key, f)
	}()

	go func() {
		for {
			select {
			case <-f.manager.ctx.Done():
				_ = stream.Close()
				return
			case payload, ok := <-f.out:
				if !ok {
					return
				}
				if err := stream.WritePacket(payload); err != nil {
					_ = stream.Close()
					return
				}
			}
		}
	}()

	for {
		payload, err := stream.ReadPacket()
		if err != nil {
			return
		}
		f.last.Store(time.Now().UnixNano())
		packet := buildUDPReply(f.dstIP, f.srcIP, f.dstPort, f.srcPort, payload)
		if _, werr := f.manager.dev.Write(packet); werr != nil {
			return
		}
	}
}

func (f *udpFlow) stop() {
	f.once.Do(func() {
		close(f.out)
		f.mu.Lock()
		if f.stream != nil {
			_ = f.stream.Close()
		}
		f.mu.Unlock()
	})
}

func buildUDPReply(srcIP, dstIP net.IP, srcPort, dstPort uint16, payload []byte) []byte {
	total := 20 + 8 + len(payload)
	pkt := make([]byte, total)
	pkt[0] = 0x45
	binary.BigEndian.PutUint16(pkt[2:4], uint16(total))
	pkt[8] = 64
	pkt[9] = 17
	copy(pkt[12:16], srcIP.To4())
	copy(pkt[16:20], dstIP.To4())
	binary.BigEndian.PutUint16(pkt[20:22], srcPort)
	binary.BigEndian.PutUint16(pkt[22:24], dstPort)
	binary.BigEndian.PutUint16(pkt[24:26], uint16(8+len(payload)))
	copy(pkt[28:], payload)
	binary.BigEndian.PutUint16(pkt[10:12], ipChecksum(pkt[:20]))
	return pkt
}

func ipChecksum(hdr []byte) uint16 {
	var sum uint32
	for i := 0; i+1 < len(hdr); i += 2 {
		sum += uint32(binary.BigEndian.Uint16(hdr[i : i+2]))
	}
	if len(hdr)%2 == 1 {
		sum += uint32(hdr[len(hdr)-1]) << 8
	}
	for sum>>16 != 0 {
		sum = (sum & 0xffff) + (sum >> 16)
	}
	return ^uint16(sum)
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	var buf [8]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}
