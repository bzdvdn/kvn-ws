// @sk-task foundation#T1.3: internal stubs (AC-002)

package session

import (
	"context"
	"fmt"
	"math"
	"math/rand/v2"
	"net"
	"sync"
	"time"

	"go.uber.org/zap"
)

type PoolCfg struct {
	Subnet     string
	Gateway    string
	RangeStart string
	RangeEnd   string
}

// @sk-task core-tunnel-mvp#T1.3: session store + IP pool (AC-009)
// @sk-task production-readiness-hardening#T1.1: add logger DI (AC-006)
type IPPool struct {
	mu        sync.Mutex
	subnet    *net.IPNet
	gateway   net.IP
	start     net.IP
	end       net.IP
	allocated map[string]net.IP
	usedIPs   map[string]bool
	boltStore *BoltStore
	logger    *zap.Logger
}

// @sk-task production-hardening#T3.1: set bolt store for pool persistence (AC-006)
func (p *IPPool) SetBoltStore(s *BoltStore) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.boltStore = s
}

// @sk-task production-hardening#T3.1: load allocations from bolt store (AC-006)
func (p *IPPool) LoadFromBolt() error {
	if p.boltStore == nil {
		return nil
	}
	allocated, err := p.boltStore.LoadAllocations()
	if err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.usedIPs = make(map[string]bool)
	for sessionID, ip := range allocated {
		p.allocated[sessionID] = ip
		p.usedIPs[ip.String()] = true
	}
	return nil
}

// @sk-task production-readiness-hardening#T1.1: add logger DI (AC-006)
func NewIPPool(cfg PoolCfg, logger *zap.Logger) (*IPPool, error) {
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
		logger:    logger,
	}, nil
}

// @sk-task ipv6-dual-stack#T1.2: IPv6 pool constructor with random offset (AC-001, AC-002)
// @sk-task production-readiness-hardening#T1.1: add logger DI (AC-006)
func NewIPPool6(cfg PoolCfg, logger *zap.Logger) (*IPPool, error) {
	_, subnet, err := net.ParseCIDR(cfg.Subnet)
	if err != nil {
		return nil, fmt.Errorf("parse v6 subnet %s: %w", cfg.Subnet, err)
	}
	gateway := net.ParseIP(cfg.Gateway)
	if gateway == nil {
		return nil, fmt.Errorf("parse v6 gateway %s", cfg.Gateway)
	}
	start := make(net.IP, 16)
	copy(start, subnet.IP)
	ones, bits := subnet.Mask.Size()
	hostBits := bits - ones
	maxHosts := int(math.Pow(2, float64(hostBits)))
	if maxHosts > 2 {
		offset := rand.IntN(maxHosts-2) + 1 // #nosec G404 — non-critical IPv6 pool offset, doesn't need crypto/rand
		for i := 15; offset > 0 && i >= 0; i-- {
			start[i] += byte(offset & 0xff)
			offset >>= 8
		}
	}
	end := broadcastAddr(subnet)
	return &IPPool{
		subnet:    subnet,
		gateway:   gateway,
		start:     start,
		end:       end,
		allocated: make(map[string]net.IP),
		usedIPs:   make(map[string]bool),
		logger:    logger,
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
		p.saveBolt()
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
		p.saveBolt()
	}
}

func (p *IPPool) saveBolt() {
	if p.boltStore == nil {
		return
	}
	if err := p.boltStore.SaveAllocations(p.allocated); err != nil {
		p.logger.Warn("bolt save failed", zap.Error(err))
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
	ID           string
	TokenName    string
	AssignedIP   net.IP
	AssignedIPv6 net.IP
	RemoteAddr   string
	ConnectedAt  time.Time
	LastActivity time.Time
}

// @sk-task production-hardening#T2.1: session manager with expiry (AC-005)
// @sk-task security-acl#T5: max_sessions per token
// @sk-task ipv6-dual-stack#T2.1: add pool6 and AssignedIPv6 for dual-stack (AC-004)
// @sk-task production-readiness-hardening#T1.1: add logger DI (AC-006)
type SessionManager struct {
	mu          sync.Mutex
	pool        *IPPool
	pool6       *IPPool
	sessions    map[string]*Session
	sessionCnt  map[string]int
	idleTimeout time.Duration
	sessionTTL  time.Duration
	stopCh      chan struct{}
	logger      *zap.Logger
	stopOnce    sync.Once
	cancelFuncs map[string]context.CancelFunc
}

// @sk-task ipv6-dual-stack#T2.3: set IPv6 pool for dual-stack allocation (AC-004)
func (sm *SessionManager) SetPool6(pool6 *IPPool) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.pool6 = pool6
}

// @sk-task production-readiness-hardening#T1.1: add logger DI (AC-006)
// @sk-task post-hardening#T1.1: sync.Once Stop + cancel map (AC-001, AC-002)
func NewSessionManager(pool *IPPool, logger *zap.Logger) *SessionManager {
	return &SessionManager{
		pool:        pool,
		sessions:    make(map[string]*Session),
		sessionCnt:  make(map[string]int),
		stopCh:      make(chan struct{}),
		logger:      logger,
		cancelFuncs: make(map[string]context.CancelFunc),
	}
}

// @sk-task production-hardening#T2.1: start expiry goroutine (AC-005)
func (sm *SessionManager) Start(idleTimeout, sessionTTL, interval time.Duration) {
	sm.mu.Lock()
	sm.idleTimeout = idleTimeout
	sm.sessionTTL = sessionTTL
	sm.mu.Unlock()

	go sm.reclaimLoop(interval)
}

// @sk-task post-hardening#T1.1: sync.Once idempotent Stop (AC-001)
func (sm *SessionManager) Stop() {
	sm.stopOnce.Do(func() {
		close(sm.stopCh)
	})
}

func (sm *SessionManager) reclaimLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			sm.expireIdle()
			sm.expireTTL()
		case <-sm.stopCh:
			return
		}
	}
}

// @sk-task production-hardening#T2.1: expire idle sessions (AC-005)
// @sk-task post-hardening#T1.2: cancel per-session on expiry (AC-002)
func (sm *SessionManager) expireIdle() {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	now := time.Now()
	for id, s := range sm.sessions {
		if sm.idleTimeout > 0 && now.Sub(s.LastActivity) > sm.idleTimeout {
			sm.cancelSession(id)
			delete(sm.sessions, id)
			sm.pool.Release(id)
		}
	}
}

// @sk-task production-hardening#T2.1: expire ttl sessions (AC-005)
// @sk-task post-hardening#T1.2: cancel per-session on expiry (AC-002)
func (sm *SessionManager) expireTTL() {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	now := time.Now()
	for id, s := range sm.sessions {
		if sm.sessionTTL > 0 && now.Sub(s.ConnectedAt) > sm.sessionTTL {
			sm.cancelSession(id)
			delete(sm.sessions, id)
			sm.pool.Release(id)
		}
	}
}

// @sk-task post-hardening#T1.2: cancel per-session context (AC-002)
func (sm *SessionManager) cancelSession(id string) {
	if cancel, ok := sm.cancelFuncs[id]; ok {
		cancel()
		delete(sm.cancelFuncs, id)
	}
}

// @sk-task security-acl#T5: Create with maxSessions check
// @sk-task ipv6-dual-stack#T2.1: dual-stack session creation with IPv6 allocation (AC-004)
func (sm *SessionManager) Create(sessionID, tokenName, remoteAddr string, maxSessions int, ipv6 bool) (sess *Session, ip, ip6 net.IP, err error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if s, ok := sm.sessions[sessionID]; ok {
		return s, s.AssignedIP, s.AssignedIPv6, nil
	}
	if maxSessions > 0 {
		cnt := sm.sessionCnt[tokenName]
		if cnt >= maxSessions {
			return nil, nil, nil, fmt.Errorf("max sessions exceeded for token %s", tokenName)
		}
	}
	ip, err = sm.pool.Allocate(sessionID)
	if err != nil {
		return nil, nil, nil, err
	}
	if ipv6 && sm.pool6 != nil {
		ip6, err = sm.pool6.Allocate(sessionID)
		if err != nil {
			sm.pool.Release(sessionID)
			return nil, nil, nil, err
		}
	}
	now := time.Now()
	s := &Session{
		ID:           sessionID,
		TokenName:    tokenName,
		AssignedIP:   ip,
		AssignedIPv6: ip6,
		RemoteAddr:   remoteAddr,
		ConnectedAt:  now,
		LastActivity: now,
	}
	sm.sessions[sessionID] = s
	sm.sessionCnt[tokenName]++
	return s, ip, ip6, nil
}

// @sk-task security-acl#T5: Remove decrements session count
// @sk-task post-hardening#T1.2: cancel per-session on Remove (AC-002)
func (sm *SessionManager) Remove(sessionID string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	s, ok := sm.sessions[sessionID]
	if ok {
		sm.sessionCnt[s.TokenName]--
		if sm.sessionCnt[s.TokenName] <= 0 {
			delete(sm.sessionCnt, s.TokenName)
		}
	}
	delete(sm.sessions, sessionID)
	sm.pool.Release(sessionID)
	sm.cancelSession(sessionID)
}

// @sk-task post-hardening#T1.2: set per-session cancel (AC-002)
func (sm *SessionManager) SetCancel(sessionID string, cancel context.CancelFunc) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.cancelFuncs[sessionID] = cancel
}

func (sm *SessionManager) Get(sessionID string) *Session {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	s, ok := sm.sessions[sessionID]
	if !ok {
		return nil
	}
	return s
}

func (sm *SessionManager) List() []*Session {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	result := make([]*Session, 0, len(sm.sessions))
	for _, s := range sm.sessions {
		result = append(result, s)
	}
	return result
}

// @sk-task production-hardening#T2.1: update session activity (AC-005)
func (sm *SessionManager) UpdateActivity(sessionID string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if s, ok := sm.sessions[sessionID]; ok {
		s.LastActivity = time.Now()
	}
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
