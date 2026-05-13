// @sk-task foundation#T1.3: internal stubs (AC-002)

package session

import (
	"fmt"
	"math"
	"net"
	"sync"
	"time"
)

type PoolCfg struct {
	Subnet     string
	Gateway    string
	RangeStart string
	RangeEnd   string
}

// @sk-task core-tunnel-mvp#T1.3: session store + IP pool (AC-009)
type IPPool struct {
	mu        sync.Mutex
	subnet    *net.IPNet
	gateway   net.IP
	start     net.IP
	end       net.IP
	allocated map[string]net.IP
	usedIPs   map[string]bool
}

func NewIPPool(cfg PoolCfg) (*IPPool, error) {
	_, subnet, err := net.ParseCIDR(cfg.Subnet)
	if err != nil {
		return nil, fmt.Errorf("parse subnet %s: %w", cfg.Subnet, err)
	}
	gateway := net.ParseIP(cfg.Gateway)
	if gateway == nil {
		return nil, fmt.Errorf("parse gateway %s", cfg.Gateway)
	}
	var startIP, endIP net.IP
	if cfg.RangeStart != "" {
		startIP = net.ParseIP(cfg.RangeStart)
		if startIP == nil {
			return nil, fmt.Errorf("parse range_start %s", cfg.RangeStart)
		}
	} else {
		startIP = nextIP(subnet.IP)
	}
	if cfg.RangeEnd != "" {
		endIP = net.ParseIP(cfg.RangeEnd)
		if endIP == nil {
			return nil, fmt.Errorf("parse range_end %s", cfg.RangeEnd)
		}
	} else {
		endIP = broadcastAddr(subnet)
	}
	return &IPPool{
		subnet:    subnet,
		gateway:   gateway,
		start:     startIP,
		end:       endIP,
		allocated: make(map[string]net.IP),
		usedIPs:   make(map[string]bool),
	}, nil
}

func (p *IPPool) Allocate(sessionID string) (net.IP, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if ip, ok := p.allocated[sessionID]; ok {
		return ip, nil
	}
	done := false
	for ip := p.start; !done; ip = nextIP(ip) {
		if ip.Equal(p.end) {
			done = true
		}
		ipStr := ip.String()
		if p.usedIPs[ipStr] {
			if done {
				break
			}
			continue
		}
		if ip.Equal(p.gateway) {
			if done {
				break
			}
			continue
		}
		assigned := make(net.IP, len(ip))
		copy(assigned, ip)
		p.allocated[sessionID] = assigned
		p.usedIPs[ipStr] = true
		return assigned, nil
	}
	return nil, fmt.Errorf("ip pool exhausted for subnet %s", p.subnet)
}

func (p *IPPool) Release(sessionID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if ip, ok := p.allocated[sessionID]; ok {
		delete(p.usedIPs, ip.String())
		delete(p.allocated, sessionID)
	}
}

func (p *IPPool) Resolve(sessionID string) (net.IP, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	ip, ok := p.allocated[sessionID]
	if !ok {
		return nil, false
	}
	return ip, true
}

type Session struct {
	ID          string
	AssignedIP  net.IP
	ConnectedAt time.Time
}

type SessionManager struct {
	mu       sync.Mutex
	pool     *IPPool
	sessions map[string]*Session
}

func NewSessionManager(pool *IPPool) *SessionManager {
	return &SessionManager{
		pool:     pool,
		sessions: make(map[string]*Session),
	}
}

func (sm *SessionManager) Create(sessionID string) (*Session, net.IP, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if s, ok := sm.sessions[sessionID]; ok {
		return s, s.AssignedIP, nil
	}
	ip, err := sm.pool.Allocate(sessionID)
	if err != nil {
		return nil, nil, err
	}
	s := &Session{
		ID:          sessionID,
		AssignedIP:  ip,
		ConnectedAt: time.Now(),
	}
	sm.sessions[sessionID] = s
	return s, ip, nil
}

func (sm *SessionManager) Remove(sessionID string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	delete(sm.sessions, sessionID)
	sm.pool.Release(sessionID)
}

func (sm *SessionManager) Get(sessionID string) (*Session, bool) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	s, ok := sm.sessions[sessionID]
	return s, ok
}

func nextIP(ip net.IP) net.IP {
	n := make(net.IP, len(ip))
	copy(n, ip)
	for i := len(n) - 1; i >= 0; i-- {
		n[i]++
		if n[i] > 0 {
			break
		}
	}
	return n
}

func broadcastAddr(subnet *net.IPNet) net.IP {
	mask := subnet.Mask
	n := subnet.IP.To4()
	if n == nil {
		n = subnet.IP
	}
	bcast := make(net.IP, len(n))
	for i := range n {
		bcast[i] = n[i] | ^mask[i]
	}
	// return last usable (broadcast - 1)
	for i := len(bcast) - 1; i >= 0; i-- {
		bcast[i]--
		if bcast[i] < 255 {
			break
		}
	}
	return bcast
}

func MaxSessions(subnet *net.IPNet) int {
	ones, bits := subnet.Mask.Size()
	usable := int(math.Pow(2, float64(bits-ones))) - 2
	if usable < 0 {
		return 0
	}
	return usable
}
