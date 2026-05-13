// @sk-test security-acl#T2: CIDR matcher unit tests
package acl

import (
	"net"
	"testing"
)

func TestCIDRMatcherDeny(t *testing.T) {
	m, err := NewCIDRMatcher(nil, []string{"10.0.0.0/8"})
	if err != nil {
		t.Fatalf("NewCIDRMatcher: %v", err)
	}
	if m.Allowed(net.ParseIP("10.0.0.1")) {
		t.Error("expected 10.0.0.1 to be denied")
	}
	if !m.Allowed(net.ParseIP("192.168.1.1")) {
		t.Error("expected 192.168.1.1 to be allowed")
	}
}

func TestCIDRMatcherAllow(t *testing.T) {
	m, err := NewCIDRMatcher([]string{"192.168.0.0/16"}, nil)
	if err != nil {
		t.Fatalf("NewCIDRMatcher: %v", err)
	}
	if !m.Allowed(net.ParseIP("192.168.1.100")) {
		t.Error("expected 192.168.1.100 to be allowed")
	}
	if m.Allowed(net.ParseIP("10.0.0.1")) {
		t.Error("expected 10.0.0.1 to be denied")
	}
}

func TestCIDRMatcherDenyOverridesAllow(t *testing.T) {
	m, err := NewCIDRMatcher([]string{"10.0.0.0/8"}, []string{"10.0.0.0/16"})
	if err != nil {
		t.Fatalf("NewCIDRMatcher: %v", err)
	}
	if m.Allowed(net.ParseIP("10.0.0.1")) {
		t.Error("expected deny to override allow")
	}
}

func TestCIDRMatcherEmptyLists(t *testing.T) {
	m, err := NewCIDRMatcher(nil, nil)
	if err != nil {
		t.Fatalf("NewCIDRMatcher: %v", err)
	}
	if !m.Allowed(net.ParseIP("1.2.3.4")) {
		t.Error("expected all IPs to be allowed when lists are empty")
	}
}

func TestCIDRMatcherInvalidCIDR(t *testing.T) {
	_, err := NewCIDRMatcher(nil, []string{"not-a-cidr"})
	if err == nil {
		t.Error("expected error for invalid CIDR")
	}
}

func TestCIDRMatcherNilIP(t *testing.T) {
	m, err := NewCIDRMatcher(nil, nil)
	if err != nil {
		t.Fatalf("NewCIDRMatcher: %v", err)
	}
	if m.Allowed(nil) {
		t.Error("expected nil IP to be denied")
	}
}

// @sk-test security-acl#SC-001: Benchmark CIDR matcher (<1ms per check)
func BenchmarkCIDRMatcher(b *testing.B) {
	m, err := NewCIDRMatcher(
		[]string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"},
		[]string{"10.0.0.0/16"},
	)
	if err != nil {
		b.Fatal(err)
	}
	ips := []net.IP{
		net.ParseIP("10.0.0.1"),      // denied (10.0.0.0/16 deny)
		net.ParseIP("10.10.10.10"),    // denied (10.0.0.0/8 allow -> 10.0.0.0/16 deny wins)
		net.ParseIP("172.16.0.1"),     // allowed
		net.ParseIP("192.168.1.1"),    // allowed
		net.ParseIP("8.8.8.8"),        // denied (not in allow list)
		net.ParseIP("1.1.1.1"),        // denied
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.Allowed(ips[i%len(ips)])
	}
}
