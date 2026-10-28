package transport

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/bzdvdn/kvn-ws/src/internal/tunpool"
)

// TUNSession is the authenticated per-client TUN assignment.
type TUNSession struct {
	TUNAddress  string `json:"tun_address"`
	TUNGateway  string `json:"tun_gateway"`
	LeaseTTLSec int    `json:"lease_ttl_sec"`
}

// Validate checks the server-assigned address/gateway pair.
func (s TUNSession) Validate() error {
	ip, network, err := net.ParseCIDR(s.TUNAddress)
	if err != nil || ip.To4() == nil {
		return fmt.Errorf("invalid tun_address %q", s.TUNAddress)
	}
	gw := net.ParseIP(s.TUNGateway).To4()
	if gw == nil || !network.Contains(gw) {
		return fmt.Errorf("invalid tun_gateway %q", s.TUNGateway)
	}
	if s.LeaseTTLSec <= 0 {
		return fmt.Errorf("invalid lease_ttl_sec %d", s.LeaseTTLSec)
	}
	return nil
}

func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	if s.pool == nil {
		http.Error(w, "tun assignment unavailable", http.StatusServiceUnavailable)
		return
	}
	clientID := strings.TrimSpace(r.Header.Get("X-Request-ID"))
	if clientID == "" {
		http.Error(w, "missing client_id", http.StatusBadRequest)
		return
	}
	lease, err := s.pool.Lease(clientID)
	if err != nil {
		if errors.Is(err, tunpool.ErrExhausted) {
			http.Error(w, "tun assignment exhausted", http.StatusServiceUnavailable)
			return
		}
		http.Error(w, "tun assignment unavailable", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(TUNSession{
		TUNAddress:  lease.Address,
		TUNGateway:  lease.Gateway,
		LeaseTTLSec: int(lease.TTL.Seconds()),
	})
}
