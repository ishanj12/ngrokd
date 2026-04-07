//go:build ignore

package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/go-logr/logr/funcr"
	dns "github.com/ishanjain/ngrok-forward-proxy/pkg/dns"
	"github.com/ishanjain/ngrok-forward-proxy/pkg/routing"
	mdns "github.com/miekg/dns"
)

func main() {
	logger := funcr.New(func(p, a string) { fmt.Printf("  %s: %s\n", p, a) }, funcr.Options{})

	// ── Part 1: resolv.conf management ──

	before := "nameserver 192.168.65.7\n"
	os.WriteFile("/tmp/test-resolv.conf", []byte(before), 0644)

	fmt.Printf("1. resolv.conf BEFORE: %q\n", strings.TrimSpace(before))

	m := dns.NewResolvManagerWithPaths("/tmp/test-resolv.conf", "/tmp/test-backup", logger)

	fmt.Println("2. Calling AddDomain(\"example.com\", \"127.0.0.1:15353\")...")
	if err := m.AddDomain("example.com", "127.0.0.1:15353"); err != nil {
		fmt.Printf("FAIL: %v\n", err)
		os.Exit(1)
	}

	after, _ := os.ReadFile("/tmp/test-resolv.conf")
	if !strings.Contains(string(after), "127.0.0.1") {
		fmt.Println("FAIL: missing our nameserver")
		os.Exit(1)
	}
	fmt.Println("   ✓ resolv.conf managed")

	// Restore for cleanup
	m.RestoreResolvConf()
	fmt.Println("   ✓ resolv.conf restored")

	// ── Part 2: DNS resolution end-to-end ──

	fmt.Println("\n3. Starting DNS server on 127.0.0.1:15353...")
	table := routing.NewTable()
	resolver := dns.NewResolver("127.0.0.1:15353", []string{}, table, logger)
	if err := resolver.Start(context.Background()); err != nil {
		fmt.Printf("FAIL: could not start DNS server: %v\n", err)
		os.Exit(1)
	}
	defer resolver.Stop()
	fmt.Println("   ✓ DNS server running")

	// Add wildcard record: *.example.com → 10.107.0.5
	wildcardIP := net.ParseIP("10.107.0.5")
	table.AddWildcard("example.com", wildcardIP)
	fmt.Println("4. Added wildcard record: *.example.com → 10.107.0.5")

	// Add exact record: specific.example.com → 10.107.0.10
	exactIP := net.ParseIP("10.107.0.10")
	table.AddExact("specific.example.com", exactIP)
	fmt.Println("   Added exact record: specific.example.com → 10.107.0.10")

	// Test wildcard resolution
	fmt.Println("\n5. Testing DNS resolution...")

	tests := []struct {
		name     string
		expected string
	}{
		{"foo.example.com", "10.107.0.5"},
		{"bar.example.com", "10.107.0.5"},
		{"anything.example.com", "10.107.0.5"},
		{"specific.example.com", "10.107.0.10"}, // exact beats wildcard
	}

	c := new(mdns.Client)
	c.Timeout = 2 * time.Second

	for _, tc := range tests {
		msg := new(mdns.Msg)
		msg.SetQuestion(mdns.Fqdn(tc.name), mdns.TypeA)
		resp, _, err := c.Exchange(msg, "127.0.0.1:15353")
		if err != nil {
			fmt.Printf("   FAIL: query for %s failed: %v\n", tc.name, err)
			os.Exit(1)
		}
		if len(resp.Answer) == 0 {
			fmt.Printf("   FAIL: no answer for %s\n", tc.name)
			os.Exit(1)
		}
		a, ok := resp.Answer[0].(*mdns.A)
		if !ok {
			fmt.Printf("   FAIL: unexpected answer type for %s\n", tc.name)
			os.Exit(1)
		}
		if !a.A.Equal(net.ParseIP(tc.expected)) {
			fmt.Printf("   FAIL: %s resolved to %s, expected %s\n", tc.name, a.A, tc.expected)
			os.Exit(1)
		}
		fmt.Printf("   ✓ %s → %s\n", tc.name, a.A)
	}

	// Test unmanaged domain returns no answer (forwarded, but no upstream)
	fmt.Println("\n6. Testing unmanaged domain (should not resolve)...")
	msg := new(mdns.Msg)
	msg.SetQuestion(mdns.Fqdn("notmanaged.other.com"), mdns.TypeA)
	resp, _, err := c.Exchange(msg, "127.0.0.1:15353")
	if err != nil {
		// Timeout or SERVFAIL is expected (no upstream configured)
		fmt.Println("   ✓ Unmanaged domain correctly not resolved")
	} else if len(resp.Answer) == 0 {
		fmt.Println("   ✓ Unmanaged domain returned empty (correct)")
	} else {
		fmt.Println("   FAIL: unmanaged domain should not resolve")
		os.Exit(1)
	}

	fmt.Println("\n✅ PASS: Linux wildcard DNS — resolv.conf management + DNS resolution working")
}
