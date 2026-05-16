// @sk-task routing-split-tunnel#T4.1: gate test program (AC-010)
// @sk-task performance-and-polish#T2.4: load testing mode (AC-008)
package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net/netip"
	"os"
	"strings"
	"time"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"go.uber.org/zap"

	"github.com/bzdvdn/kvn-ws/src/internal/config"
	"github.com/bzdvdn/kvn-ws/src/internal/routing"
	"github.com/bzdvdn/kvn-ws/src/internal/transport/websocket"
)

type mockResolver struct{}

func (m *mockResolver) Lookup(ctx context.Context, domain string) ([]netip.Addr, error) {
	if domain == "corp.example.com" {
		return []netip.Addr{netip.MustParseAddr("10.10.10.10")}, nil
	}
	return nil, nil
}

func main() {
	mode := pflag.String("mode", "routing", "test mode: routing | loadtest")
	cfgPath := pflag.String("config", "configs/loadtest.yaml", "config path (loadtest mode)")
	pflag.Parse()

	switch *mode {
	case "routing":
		runRoutingTest()
	case "loadtest":
		runLoadTest(*cfgPath)
	default:
		log.Fatalf("unknown mode: %s", *mode)
	}
}

func runRoutingTest() {
	fmt.Println("=== Routing Gate Test ===")

	cfg := &config.RoutingCfg{
		DefaultRoute:   "direct",
		IncludeRanges:  []string{"10.0.0.0/8", "172.16.0.0/12"},
		IncludeDomains: []string{"corp.example.com"},
	}

	rs, err := routing.NewRuleSetWithResolver(cfg, &mockResolver{}, zap.NewNop())
	if err != nil {
		fmt.Printf("FAIL: NewRuleSet: %v\n", err)
		return
	}

	tests := []struct {
		ip       string
		expected routing.RouteAction
		desc     string
	}{
		{"10.10.10.10", routing.RouteServer, "corp IP via include_range"},
		{"172.16.0.50", routing.RouteServer, "corp IP via include_range 2"},
		{"8.8.8.8", routing.RouteDirect, "public IP — default direct"},
		{"1.1.1.1", routing.RouteDirect, "public DNS — default direct"},
		{"10.0.0.1", routing.RouteServer, "edge of include_range"},
	}

	allPass := true
	for _, tt := range tests {
		ip := netip.MustParseAddr(tt.ip)
		action := rs.Route(ip)
		status := "PASS"
		if action != tt.expected {
			status = "FAIL"
			allPass = false
		}
		fmt.Printf("  [%s] %-15s -> %d (expected %d) — %s\n", status, tt.ip, action, tt.expected, tt.desc)
	}

	if !allPass {
		fmt.Println("\nFAIL: routing decisions did not match expectations")
	} else {
		fmt.Println("\nPASS: all routing decisions match expectations")
	}
}

func runLoadTest(cfgPath string) {
	v := viper.New()
	v.SetConfigFile(cfgPath)
	v.SetEnvPrefix("KVN_LOADTEST")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {
		log.Fatalf("read config %s: %v", cfgPath, err)
	}

	targetHost := v.GetString("target_host")
	sessionCount := v.GetInt("session_count")
	durationSec := v.GetInt("duration_sec")
	throughputThreshold := v.GetInt64("throughput_threshold_bps")
	latencyThreshold := v.GetInt("latency_threshold_ms")
	token := v.GetString("auth.token")
	mtu := v.GetInt("mtu")

	if targetHost == "" {
		targetHost = "wss://localhost:443/tunnel"
	}
	if sessionCount <= 0 {
		sessionCount = 10
	}
	if durationSec <= 0 {
		durationSec = 5
	}

	fmt.Println("=== Load Test ===")
	fmt.Printf("Target: %s\n", targetHost)
	fmt.Printf("Sessions: %d\n", sessionCount)
	fmt.Printf("Duration: %ds\n", durationSec)
	if token != "" {
		fmt.Printf("Auth token: %s\n", token[:min(len(token), 8)]+"...")
	}

	tlsCfg := &tls.Config{InsecureSkipVerify: true} // #nosec G402 — test tool, not production
	wsCfg := websocket.WSConfig{MTU: mtu}

	connStart := time.Now()
	conns := make([]*websocket.WSConn, 0, sessionCount)

	for i := 0; i < sessionCount; i++ {
		conn, err := websocket.Dial(targetHost, tlsCfg, zap.NewNop(), wsCfg)
		if err != nil {
			fmt.Printf("session %d: dial failed: %v\n", i, err)
			continue
		}
		conns = append(conns, conn)
		if (i+1)%100 == 0 {
			fmt.Printf("  connected %d/%d...\n", i+1, sessionCount)
		}
	}

	connElapsed := time.Since(connStart)
	fmt.Printf("\nConnections: %d/%d in %v\n", len(conns), sessionCount, connElapsed)

	if len(conns) == 0 {
		fmt.Println("\nSKIP: no connections established (server may not be running)")
		return
	}

	payload := make([]byte, 1400)
	for i := range payload {
		payload[i] = byte(i)
	}

	testStart := time.Now()
	testEnd := testStart.Add(time.Duration(durationSec) * time.Second)
	totalSent := int64(0)
	done := make(chan struct{})

	go func() {
		for _, conn := range conns {
			go func(c *websocket.WSConn) {
				for time.Now().Before(testEnd) {
					if err := c.WriteMessage(payload); err != nil {
						return
					}
					totalSent += int64(len(payload))
				}
			}(conn)
		}
		close(done)
	}()

	time.Sleep(time.Duration(durationSec) * time.Second)
	actualDuration := time.Since(testStart)

	for _, conn := range conns {
		_ = conn.Close()
	}

	throughputBps := int64(float64(totalSent*8) / actualDuration.Seconds())

	fmt.Printf("\nResults:\n")
	fmt.Printf("  Data sent: %d bytes\n", totalSent)
	fmt.Printf("  Duration: %v\n", actualDuration)
	fmt.Printf("  Throughput: %d bps\n", throughputBps)

	pass := true
	switch {
	case throughputThreshold > 0 && throughputBps >= throughputThreshold:
		fmt.Printf("  [PASS] Throughput %d >= %d bps\n", throughputBps, throughputThreshold)
	case throughputThreshold > 0:
		fmt.Printf("  [FAIL] Throughput %d < %d bps\n", throughputBps, throughputThreshold)
		pass = false
	default:
		fmt.Printf("  [INFO] No throughput threshold configured\n")
	}

	switch {
	case latencyThreshold > 0 && int(connElapsed.Milliseconds()) <= latencyThreshold:
		fmt.Printf("  [PASS] Connection latency %d ms <= %d ms\n", connElapsed.Milliseconds(), latencyThreshold)
	case latencyThreshold > 0:
		fmt.Printf("  [FAIL] Connection latency %d ms > %d ms\n", connElapsed.Milliseconds(), latencyThreshold)
		pass = false
	default:
		fmt.Printf("  [INFO] No latency threshold configured\n")
	}

	if pass {
		fmt.Println("\nPASS: load test passed")
	} else {
		fmt.Println("\nFAIL: load test did not meet thresholds")
		os.Exit(1)
	}
}
