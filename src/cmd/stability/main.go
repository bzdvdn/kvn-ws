// @sk-task production-hardening#T5.2: stability gate program (AC-012)
package main

import (
	"fmt"
	"log"
	"net/netip"
	"os"
	"runtime"
	"strconv"
	"time"

	"github.com/bzdvdn/kvn-ws/src/internal/config"
	"github.com/bzdvdn/kvn-ws/src/internal/routing"
)

func main() {
	duration := 30
	if len(os.Args) > 1 {
		if v, err := strconv.Atoi(os.Args[1]); err == nil {
			duration = v
		}
	}

	cfg := &config.RoutingCfg{
		DefaultRoute:  "direct",
		IncludeRanges: []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"},
		ExcludeRanges: []string{"10.0.0.0/16"},
		IncludeIPs:    []string{"10.10.10.10"},
	}

	rs, err := routing.NewRuleSet(cfg)
	if err != nil {
		log.Fatalf("NewRuleSet: %v", err)
	}

	ips := []netip.Addr{
		netip.MustParseAddr("10.0.0.1"),
		netip.MustParseAddr("10.10.10.10"),
		netip.MustParseAddr("172.16.0.1"),
		netip.MustParseAddr("8.8.8.8"),
		netip.MustParseAddr("1.1.1.1"),
		netip.MustParseAddr("192.168.1.1"),
	}

	start := time.Now()
	iterations := 0
	var memStats runtime.MemStats
	var maxHeap uint64

	for time.Since(start) < time.Duration(duration)*time.Second {
		for _, ip := range ips {
			rs.Route(ip)
			iterations++
		}
		if iterations%100000 == 0 {
			runtime.ReadMemStats(&memStats)
			if memStats.HeapInuse > maxHeap {
				maxHeap = memStats.HeapInuse
			}
		}
	}

	elapsed := time.Since(start)
	runtime.ReadMemStats(&memStats)
	rps := float64(iterations) / elapsed.Seconds()

	fmt.Printf("iterations: %d\n", iterations)
	fmt.Printf("elapsed: %.2fs\n", elapsed.Seconds())
	fmt.Printf("rps: %.0f\n", rps)
	fmt.Printf("max_heap: %d bytes (%.2f MB)\n", maxHeap, float64(maxHeap)/1024/1024)
	fmt.Printf("final_heap: %d bytes (%.2f MB)\n", memStats.HeapInuse, float64(memStats.HeapInuse)/1024/1024)
	fmt.Printf("total_alloc: %d bytes (%.2f MB)\n", memStats.TotalAlloc, float64(memStats.TotalAlloc)/1024/1024)
	fmt.Printf("mallocs: %d\n", memStats.Mallocs)
	fmt.Printf("frees: %d\n", memStats.Frees)
}
