package tunnel

import (
	"errors"
	"net"
	"sync"

	"go.uber.org/zap"

	"github.com/bzdvdn/kvn-ws/src/internal/tun"
)

// TunDemux provides a single-goroutine reader for a shared TUN device,
// dispatching each packet to the session registered for its destination IP.
type TunDemux struct {
	tunDev   tun.TunDevice
	mu       sync.RWMutex
	sessions map[string]chan tunReadResult
	logger   *zap.Logger
}

func NewTunDemux(tunDev tun.TunDevice, logger *zap.Logger) *TunDemux {
	d := &TunDemux{
		tunDev:   tunDev,
		sessions: make(map[string]chan tunReadResult),
		logger:   logger,
	}
	go d.run()
	return d
}

func (d *TunDemux) Register(ip4, ip6 net.IP, ch chan tunReadResult) {
	d.mu.Lock()
	if ip4 != nil {
		d.sessions[ip4.String()] = ch
	}
	if ip6 != nil {
		d.sessions[ip6.String()] = ch
	}
	d.mu.Unlock()
}

func (d *TunDemux) Unregister(ip4, ip6 net.IP) {
	d.mu.Lock()
	if ip4 != nil {
		delete(d.sessions, ip4.String())
	}
	if ip6 != nil {
		delete(d.sessions, ip6.String())
	}
	d.mu.Unlock()
}

func (d *TunDemux) run() {
	defer d.signalAll(errors.New("tun demux stopped"))

	for {
		buf := make([]byte, 1500)
		n, err := d.tunDev.Read(buf)
		if err != nil {
			d.logger.Error("tun read error in demux", zap.Error(err))
			return
		}

		destIP := parseDestIP(buf[:n])
		if destIP == nil {
			continue
		}

		d.mu.RLock()
		ch, ok := d.sessions[destIP.String()]
		d.mu.RUnlock()

		if !ok {
			continue
		}

		select {
		case ch <- tunReadResult{n: n, buf: buf}:
		default:
		}
	}
}

func (d *TunDemux) signalAll(err error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	for _, ch := range d.sessions {
		select {
		case ch <- tunReadResult{err: err}:
		default:
		}
	}
}

func parseDestIP(packet []byte) net.IP {
	if len(packet) < 1 {
		return nil
	}
	switch packet[0] >> 4 {
	case 4:
		if len(packet) < 20 {
			return nil
		}
		return net.IP(packet[16:20])
	case 6:
		if len(packet) < 40 {
			return nil
		}
		return net.IP(packet[24:40])
	default:
		return nil
	}
}
