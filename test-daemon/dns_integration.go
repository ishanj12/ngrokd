package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/ishanjain/ngrok-forward-proxy/pkg/dns"
	"github.com/ishanjain/ngrok-forward-proxy/pkg/routing"

	"github.com/go-logr/logr/funcr"
	mdns "github.com/miekg/dns"
)

func main() {
	logger := funcr.New(func(prefix, args string) {
		fmt.Fprintf(os.Stderr, "[%s] %s\n", prefix, args)
	}, funcr.Options{Verbosity: 1})

	// Find a free port (avoid needing sudo for port 53)
	port := findFreePort()
	addr := fmt.Sprintf("127.0.0.1:%d", port)

	// Create routing table and add test records
	table := routing.NewTable()

	// Simulate: *.example.com → 10.0.1.1 (wildcard)
	table.AddWildcard("example.com", net.ParseIP("10.0.1.1"))

	// Simulate: service.example.com → 10.0.0.99 (exact, under the wildcard)
	table.AddExact("service.example.com", net.ParseIP("10.0.0.99"))

	// Simulate: other.test.com → 10.0.0.50 (exact, no wildcard domain)
	table.AddExact("other.test.com", net.ParseIP("10.0.0.50"))

	// Start DNS resolver
	resolver := dns.NewResolver(addr, []string{"8.8.8.8:53"}, table, logger)
	if err := resolver.Start(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start resolver: %v\n", err)
		os.Exit(1)
	}
	defer resolver.Stop()

	fmt.Printf("DNS resolver running on %s\n\n", addr)

	passed := 0
	failed := 0

	// Test 1: Exact match takes priority over wildcard
	fmt.Println("=== Test 1: Exact match over wildcard ===")
	ip := queryA(addr, "service.example.com")
	if ip == "10.0.0.99" {
		fmt.Printf("  ✅ service.example.com → %s (exact match wins)\n", ip)
		passed++
	} else {
		fmt.Printf("  ❌ service.example.com → %s (expected 10.0.0.99)\n", ip)
		failed++
	}

	// Test 2: Wildcard catches unknown subdomains
	fmt.Println("\n=== Test 2: Wildcard match ===")
	ip = queryA(addr, "random.example.com")
	if ip == "10.0.1.1" {
		fmt.Printf("  ✅ random.example.com → %s (wildcard match)\n", ip)
		passed++
	} else {
		fmt.Printf("  ❌ random.example.com → %s (expected 10.0.1.1)\n", ip)
		failed++
	}

	// Test 3: Another wildcard subdomain
	fmt.Println("\n=== Test 3: Another wildcard subdomain ===")
	ip = queryA(addr, "anything.example.com")
	if ip == "10.0.1.1" {
		fmt.Printf("  ✅ anything.example.com → %s (wildcard match)\n", ip)
		passed++
	} else {
		fmt.Printf("  ❌ anything.example.com → %s (expected 10.0.1.1)\n", ip)
		failed++
	}

	// Test 4: Exact match on non-wildcard domain
	fmt.Println("\n=== Test 4: Exact match (no wildcard) ===")
	ip = queryA(addr, "other.test.com")
	if ip == "10.0.0.50" {
		fmt.Printf("  ✅ other.test.com → %s (exact match)\n", ip)
		passed++
	} else {
		fmt.Printf("  ❌ other.test.com → %s (expected 10.0.0.50)\n", ip)
		failed++
	}

	// Test 5: Wildcard does NOT match base domain (forwards upstream instead)
	fmt.Println("\n=== Test 5: Wildcard does NOT match base domain ===")
	ip = queryA(addr, "example.com")
	if ip != "10.0.1.1" {
		fmt.Printf("  ✅ example.com → %s (not the wildcard IP 10.0.1.1, forwarded upstream)\n", ip)
		passed++
	} else {
		fmt.Printf("  ❌ example.com → %s (should NOT match wildcard)\n", ip)
		failed++
	}

	// Test 6: AAAA returns NODATA for managed hosts (prevents IPv6 bypass)
	fmt.Println("\n=== Test 6: AAAA returns NODATA ===")
	hasAAAA := queryAAAA(addr, "service.example.com")
	if !hasAAAA {
		fmt.Printf("  ✅ service.example.com AAAA → NODATA\n")
		passed++
	} else {
		fmt.Printf("  ❌ service.example.com AAAA → got answer (expected NODATA)\n")
		failed++
	}

	// Test 7: Unmanaged domain forwards upstream
	fmt.Println("\n=== Test 7: Unmanaged domain forwards upstream ===")
	ip = queryA(addr, "google.com")
	if ip != "" {
		fmt.Printf("  ✅ google.com → %s (forwarded to upstream)\n", ip)
		passed++
	} else {
		fmt.Printf("  ❌ google.com → no answer (expected upstream response)\n")
		failed++
	}

	fmt.Printf("\n=== Results: %d passed, %d failed ===\n", passed, failed)
	if failed > 0 {
		os.Exit(1)
	}
}

func findFreePort() int {
	l, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	port := l.LocalAddr().(*net.UDPAddr).Port
	l.Close()
	return port
}

func queryA(server, name string) string {
	c := new(mdns.Client)
	c.Timeout = 3 * time.Second
	m := new(mdns.Msg)
	m.SetQuestion(mdns.Fqdn(name), mdns.TypeA)

	resp, _, err := c.Exchange(m, server)
	if err != nil {
		return fmt.Sprintf("ERROR: %v", err)
	}

	for _, ans := range resp.Answer {
		if a, ok := ans.(*mdns.A); ok {
			return a.A.String()
		}
	}
	return ""
}

func queryAAAA(server, name string) bool {
	c := new(mdns.Client)
	c.Timeout = 3 * time.Second
	m := new(mdns.Msg)
	m.SetQuestion(mdns.Fqdn(name), mdns.TypeAAAA)

	resp, _, err := c.Exchange(m, server)
	if err != nil {
		return false
	}
	return len(resp.Answer) > 0
}


