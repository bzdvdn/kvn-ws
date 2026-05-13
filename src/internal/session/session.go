// @sk-task foundation#T1.3: internal stubs (AC-002)

package session

import (
	"fmt"
	"log"
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
	boltStore *BoltStore
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
		log.Printf("[pool] bolt save: %v", err)
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
	RemoteAddr   string
	ConnectedAt  time.Time
	LastActivity time.Time
}

// @sk-task production-hardening#T2.1: session manager with expiry (AC-005)
// @sk-task security-acl#T5: max_sessions per token
type SessionManager struct {
	mu          sync.Mutex
	pool        *IPPool
	sessions    map[string]*Session
	sessionCnt  map[string]int
	idleTimeout time.Duration
	sessionTTL  time.Duration
	stopCh      chan struct{}
}

func NewSessionManager(pool *IPPool) *SessionManager {
	return &SessionManager{
		pool:       pool,
		sessions:   make(map[string]*Session),
		sessionCnt: make(map[string]int),
		stopCh:     make(chan struct{}),
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

func (sm *SessionManager) Stop() {
	close(sm.stopCh)
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
func (sm *SessionManager) expireIdle() {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	now := time.Now()
	for id, s := range sm.sessions {
		if sm.idleTimeout > 0 && now.Sub(s.LastActivity) > sm.idleTimeout {
			delete(sm.sessions, id)
			sm.pool.Release(id)
		}
	}
}

// @sk-task production-hardening#T2.1: expire ttl sessions (AC-005)
func (sm *SessionManager) expireTTL() {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	now := time.Now()
	for id, s := range sm.sessions {
		if sm.sessionTTL > 0 && now.Sub(s.ConnectedAt) > sm.sessionTTL {
			delete(sm.sessions, id)
			sm.pool.Release(id)
		}
	}
}

// @sk-task security-acl#T5: Create with maxSessions check
func (sm *SessionManager) Create(sessionID, tokenName, remoteAddr string, maxSessions int) (*Session, net.IP, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if s, ok := sm.sessions[sessionID]; ok {
		return s, s.AssignedIP, nil
	}
	if maxSessions > 0 {
		cnt := sm.sessionCnt[tokenName]
		if cnt >= maxSessions {
			return nil, nil, fmt.Errorf("max sessions exceeded for token %s", tokenName)
		}
	}
	ip, err := sm.pool.Allocate(sessionID)
	if err != nil {
		return nil, nil, err
	}
	now := time.Now()
	s := &Session{
		ID:           sessionID,
		TokenName:    tokenName,
		AssignedIP:   ip,
		RemoteAddr:   remoteAddr,
		ConnectedAt:  now,
		LastActivity: now,
	}
	sm.sessions[sessionID] = s
	sm.sessionCnt[tokenName]++
	return s, ip, nil
}

// @sk-task security-acl#T5: Remove decrements session count
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
