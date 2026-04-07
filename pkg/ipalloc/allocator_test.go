package ipalloc

import (
	"net"
	"path/filepath"
	"testing"

	"github.com/go-logr/logr"
)

func TestAllocator_AllocateIPForPort(t *testing.T) {
	a := NewAllocator("10.107.0.0/16", logr.Discard())
	ip, err := a.AllocateIPForPort("host1", 80)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ip == "" {
		t.Fatal("expected non-empty IP")
	}
	parsed := net.ParseIP(ip)
	if parsed == nil {
		t.Fatalf("invalid IP: %s", ip)
	}
	_, subnet, _ := net.ParseCIDR("10.107.0.0/16")
	if !subnet.Contains(parsed) {
		t.Fatalf("IP %s not in subnet 10.107.0.0/16", ip)
	}
}

func TestAllocator_SameHostSameIP(t *testing.T) {
	a := NewAllocator("10.107.0.0/16", logr.Discard())
	ip1, err := a.AllocateIPForPort("host1", 80)
	if err != nil {
		t.Fatalf("first alloc: %v", err)
	}
	ip2, err := a.AllocateIPForPort("host1", 80)
	if err != nil {
		t.Fatalf("second alloc: %v", err)
	}
	if ip1 != ip2 {
		t.Fatalf("expected same IP, got %s and %s", ip1, ip2)
	}
}

func TestAllocator_DifferentHostsDifferentIPs(t *testing.T) {
	a := NewAllocator("10.107.0.0/16", logr.Discard())
	ip1, err := a.AllocateIPForPort("host1", 80)
	if err != nil {
		t.Fatalf("host1: %v", err)
	}
	ip2, err := a.AllocateIPForPort("host2", 80)
	if err != nil {
		t.Fatalf("host2: %v", err)
	}
	if ip1 == ip2 {
		t.Fatalf("expected different IPs for same port, got %s for both", ip1)
	}
}

func TestAllocator_IPReuseWithDifferentPorts(t *testing.T) {
	a := NewAllocator("10.107.0.0/16", logr.Discard())
	ip1, err := a.AllocateIPForPort("host1", 80)
	if err != nil {
		t.Fatalf("host1: %v", err)
	}
	ip2, err := a.AllocateIPForPort("host2", 443)
	if err != nil {
		t.Fatalf("host2: %v", err)
	}
	if ip1 != ip2 {
		t.Fatalf("expected IP reuse for different ports, got %s and %s", ip1, ip2)
	}
}

func TestAllocator_ReleaseIP(t *testing.T) {
	a := NewAllocator("10.107.0.0/16", logr.Discard())
	_, err := a.AllocateIPForPort("host1", 80)
	if err != nil {
		t.Fatalf("alloc: %v", err)
	}
	a.ReleaseIP("host1")
	mappings := a.GetAllMappings()
	if _, exists := mappings["host1"]; exists {
		t.Fatal("host1 should not be in mappings after release")
	}
}

func TestAllocator_SaveLoadPersistentMappings(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mappings.json")

	a1 := NewAllocator("10.107.0.0/16", logr.Discard())
	ip1, err := a1.AllocateIPForPort("host1", 80)
	if err != nil {
		t.Fatalf("alloc host1: %v", err)
	}
	ip2, err := a1.AllocateIPForPort("host2", 443)
	if err != nil {
		t.Fatalf("alloc host2: %v", err)
	}
	if err := a1.SavePersistentMappings(path); err != nil {
		t.Fatalf("save: %v", err)
	}

	a2 := NewAllocator("10.107.0.0/16", logr.Discard())
	if err := a2.LoadPersistentMappings(path); err != nil {
		t.Fatalf("load: %v", err)
	}
	loaded := a2.GetAllMappings()
	if loaded["host1"] != ip1 {
		t.Fatalf("host1: want %s, got %s", ip1, loaded["host1"])
	}
	if loaded["host2"] != ip2 {
		t.Fatalf("host2: want %s, got %s", ip2, loaded["host2"])
	}
}

func TestAllocator_ReleaseIPForPort_PartialRelease(t *testing.T) {
	a := NewAllocator("10.107.0.0/16", logr.Discard())
	ip1, err := a.AllocateIPForPort("host1", 80)
	if err != nil {
		t.Fatalf("alloc port 80: %v", err)
	}
	ip2, err := a.AllocateIPForPort("host1", 443)
	if err != nil {
		t.Fatalf("alloc port 443: %v", err)
	}
	if ip1 != ip2 {
		t.Fatalf("expected same IP for same hostname, got %s and %s", ip1, ip2)
	}

	// Release port 80 — IP should NOT be freed
	_, ipFreed := a.ReleaseIPForPort("host1", 80)
	if ipFreed {
		t.Fatal("IP should not be freed when port 443 is still active")
	}
	mappings := a.GetAllMappings()
	if _, exists := mappings["host1"]; !exists {
		t.Fatal("host1 mapping should still exist after partial release")
	}

	// Release port 443 — IP should be freed
	_, ipFreed = a.ReleaseIPForPort("host1", 443)
	if !ipFreed {
		t.Fatal("IP should be freed when all ports are released")
	}
	mappings = a.GetAllMappings()
	if _, exists := mappings["host1"]; exists {
		t.Fatal("host1 mapping should not exist after full release")
	}
}

func TestAllocator_ReleaseIPForPort_SinglePort(t *testing.T) {
	a := NewAllocator("10.107.0.0/16", logr.Discard())
	_, err := a.AllocateIPForPort("host1", 80)
	if err != nil {
		t.Fatalf("alloc: %v", err)
	}
	_, ipFreed := a.ReleaseIPForPort("host1", 80)
	if !ipFreed {
		t.Fatal("IP should be freed when single port is released")
	}
	mappings := a.GetAllMappings()
	if _, exists := mappings["host1"]; exists {
		t.Fatal("host1 should not be in mappings after release")
	}
}

func TestAllocator_ReleaseIPForPort_NonExistent(t *testing.T) {
	a := NewAllocator("10.107.0.0/16", logr.Discard())
	ip, ipFreed := a.ReleaseIPForPort("nonexistent", 80)
	if ip != "" {
		t.Fatalf("expected empty IP, got %s", ip)
	}
	if ipFreed {
		t.Fatal("expected ipFreed=false for nonexistent hostname")
	}
}

func TestAllocator_CleanupDoesNotBreakSiblingPorts(t *testing.T) {
	a := NewAllocator("10.107.0.0/16", logr.Discard())
	ip1, err := a.AllocateIPForPort("host1", 80)
	if err != nil {
		t.Fatalf("alloc port 80: %v", err)
	}
	ip2, err := a.AllocateIPForPort("host1", 443)
	if err != nil {
		t.Fatalf("alloc port 443: %v", err)
	}
	if ip1 != ip2 {
		t.Fatalf("expected same IP, got %s and %s", ip1, ip2)
	}

	// Release port 80
	a.ReleaseIPForPort("host1", 80)

	// Re-allocate port 80 — should get the same IP back
	ip3, err := a.AllocateIPForPort("host1", 80)
	if err != nil {
		t.Fatalf("re-alloc port 80: %v", err)
	}
	if ip3 != ip1 {
		t.Fatalf("expected same IP %s after re-alloc, got %s", ip1, ip3)
	}
}

func TestParseHostname(t *testing.T) {
	tests := []struct {
		input    string
		wantHost string
		wantPort int
		wantErr  bool
	}{
		{"https://api.company.com", "api.company.com", 443, false},
		{"http://service:8080", "service", 8080, false},
		{"tcp://db.internal:5432", "db.internal", 5432, false},
		{"https://api.company.com:8443", "api.company.com", 8443, false},
		{"invalid", "", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			host, port, err := ParseHostname(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if host != tt.wantHost {
				t.Errorf("host = %q, want %q", host, tt.wantHost)
			}
			if port != tt.wantPort {
				t.Errorf("port = %d, want %d", port, tt.wantPort)
			}
		})
	}
}
