// Package tunpool allocates unique TUN IPv4 addresses with short leases.
package tunpool

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var (
	ErrExhausted = errors.New("tun assignment exhausted")
	ErrInvalid   = errors.New("invalid tun pool")
)

// Config is the allocator range.
type Config struct {
	Subnet      string
	RangeStart  string
	RangeEnd    string
	Gateway     string
	LeaseTTLSec int
	LeaseFile   string
}

// Lease is one active client assignment.
type Lease struct {
	ClientID string
	Address  string
	Gateway  string
	TTL      time.Duration
}

// Pool is an in-memory IPv4 lease pool.
type Pool struct {
	mu        sync.Mutex
	subnet    *net.IPNet
	start     uint32
	end       uint32
	gateway   string
	ttl       time.Duration
	now       func() time.Time
	storePath string
	byClient  map[string]*entry
	byIP      map[uint32]string
}

type entry struct {
	clientID string
	ip       uint32
	expires  time.Time
}

// New validates the range and returns an empty pool.
func New(cfg *Config) (*Pool, error) {
	_, subnet, err := net.ParseCIDR(cfg.Subnet)
	if err != nil || subnet.IP.To4() == nil {
		return nil, fmt.Errorf("%w: subnet must be IPv4 CIDR", ErrInvalid)
	}
	start, err := ip4ToUint(cfg.RangeStart)
	if err != nil || !subnet.Contains(uintToIP(start)) {
		return nil, fmt.Errorf("%w: range_start outside subnet", ErrInvalid)
	}
	end, err := ip4ToUint(cfg.RangeEnd)
	if err != nil || !subnet.Contains(uintToIP(end)) {
		return nil, fmt.Errorf("%w: range_end outside subnet", ErrInvalid)
	}
	if start > end {
		return nil, fmt.Errorf("%w: range_start must be <= range_end", ErrInvalid)
	}
	gw := net.ParseIP(cfg.Gateway).To4()
	if gw == nil || !subnet.Contains(gw) {
		return nil, fmt.Errorf("%w: gateway outside subnet", ErrInvalid)
	}
	if cfg.LeaseTTLSec <= 0 {
		return nil, fmt.Errorf("%w: lease_ttl_sec must be > 0", ErrInvalid)
	}
	p := &Pool{
		subnet: subnet, start: start, end: end, gateway: cfg.Gateway,
		ttl: time.Duration(cfg.LeaseTTLSec) * time.Second, now: time.Now,
		storePath: cfg.LeaseFile,
		byClient:  make(map[string]*entry), byIP: make(map[uint32]string),
	}
	if err := p.load(); err != nil {
		return nil, err
	}
	return p, nil
}

// Lease returns the same active address for a client or allocates a free one.
func (p *Pool) Lease(clientID string) (Lease, error) {
	if clientID == "" {
		return Lease{}, fmt.Errorf("%w: client_id is required", ErrInvalid)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	now := p.now()
	p.expireLocked(now)
	if e := p.byClient[clientID]; e != nil {
		e.expires = now.Add(p.ttl)
		p.saveLocked()
		return p.leaseLocked(e, now), nil
	}
	for ip := p.start; ip <= p.end; ip++ {
		if _, used := p.byIP[ip]; used {
			continue
		}
		e := &entry{clientID: clientID, ip: ip, expires: now.Add(p.ttl)}
		p.byClient[clientID] = e
		p.byIP[ip] = clientID
		p.saveLocked()
		return p.leaseLocked(e, now), nil
	}
	return Lease{}, ErrExhausted
}

// Expire removes expired leases.
func (p *Pool) Expire() {
	p.mu.Lock()
	p.expireLocked(p.now())
	p.saveLocked()
	p.mu.Unlock()
}

// Touch renews an existing lease after a stream/session activity.
func (p *Pool) Touch(clientID string) {
	if clientID == "" {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if e := p.byClient[clientID]; e != nil {
		e.expires = p.now().Add(p.ttl)
		p.saveLocked()
	}
}

func (p *Pool) expireLocked(now time.Time) {
	for id, e := range p.byClient {
		if !e.expires.After(now) {
			delete(p.byClient, id)
			delete(p.byIP, e.ip)
		}
	}
}

func (p *Pool) leaseLocked(e *entry, now time.Time) Lease {
	ones, _ := p.subnet.Mask.Size()
	return Lease{
		ClientID: e.clientID,
		Address:  fmt.Sprintf("%s/%d", uintToIP(e.ip).String(), ones),
		Gateway:  p.gateway,
		TTL:      e.expires.Sub(now),
	}
}

type persistedLease struct {
	ClientID string `json:"client_id"`
	IP       string `json:"ip"`
	Expires  int64  `json:"expires_unix"`
}

func (p *Pool) load() error {
	if p.storePath == "" {
		return nil
	}
	data, err := os.ReadFile(p.storePath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read tun leases: %w", err)
	}
	var leases []persistedLease
	if err := json.Unmarshal(data, &leases); err != nil {
		// A corrupt optional lease file must not prevent the server from starting.
		return nil
	}
	now := p.now()
	for _, l := range leases {
		ip, err := ip4ToUint(l.IP)
		if err != nil || ip < p.start || ip > p.end || l.ClientID == "" {
			continue
		}
		if l.Expires <= now.Unix() {
			continue
		}
		p.byClient[l.ClientID] = &entry{clientID: l.ClientID, ip: ip, expires: time.Unix(l.Expires, 0)}
		p.byIP[ip] = l.ClientID
	}
	return nil
}

func (p *Pool) saveLocked() {
	if p.storePath == "" {
		return
	}
	leases := make([]persistedLease, 0, len(p.byClient))
	for _, e := range p.byClient {
		leases = append(leases, persistedLease{ClientID: e.clientID, IP: uintToIP(e.ip).String(), Expires: e.expires.Unix()})
	}
	data, err := json.Marshal(leases)
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(p.storePath), 0o700); err != nil {
		return
	}
	tmp := p.storePath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return
	}
	_ = os.Rename(tmp, p.storePath)
}

func ip4ToUint(s string) (uint32, error) {
	ip := net.ParseIP(s).To4()
	if ip == nil {
		return 0, errors.New("not IPv4")
	}
	return uint32(ip[0])<<24 | uint32(ip[1])<<16 | uint32(ip[2])<<8 | uint32(ip[3]), nil
}

func uintToIP(v uint32) net.IP {
	return net.IPv4(byte(v>>24), byte(v>>16), byte(v>>8), byte(v)).To4() // #nosec G115 -- uint32 is exactly 4 bytes
}
