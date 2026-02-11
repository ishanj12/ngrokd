package health

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/go-logr/logr"
)

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return port
}

func newTestServer(t *testing.T) (*Server, string) {
	t.Helper()
	port := freePort(t)
	s := NewServer(Config{
		Address: "127.0.0.1",
		Port:    port,
		Logger:  logr.Discard(),
	})
	base := fmt.Sprintf("http://127.0.0.1:%d", port)
	return s, base
}

func TestHealthServer_RegisterUnregister(t *testing.T) {
	s := NewServer(Config{Port: 1, Logger: logr.Discard()})

	s.RegisterEndpoint("ep1", "127.0.0.1:8080", "tcp://remote:443")

	s.mu.RLock()
	_, ok := s.endpoints["ep1"]
	s.mu.RUnlock()
	if !ok {
		t.Fatal("endpoint ep1 should be registered")
	}

	s.UnregisterEndpoint("ep1")

	s.mu.RLock()
	_, ok = s.endpoints["ep1"]
	s.mu.RUnlock()
	if ok {
		t.Fatal("endpoint ep1 should be unregistered")
	}
}

func TestHealthServer_ReadyState(t *testing.T) {
	s := NewServer(Config{Port: 1, Logger: logr.Discard()})

	s.mu.RLock()
	if s.ready {
		t.Fatal("ready should be false by default")
	}
	s.mu.RUnlock()

	s.SetReady(true)

	s.mu.RLock()
	if !s.ready {
		t.Fatal("ready should be true after SetReady(true)")
	}
	s.mu.RUnlock()
}

func TestHealthServer_HealthEndpoint(t *testing.T) {
	s, base := newTestServer(t)
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	defer s.Stop(context.Background())
	time.Sleep(50 * time.Millisecond)

	resp, err := http.Get(base + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 with no endpoints, got %d", resp.StatusCode)
	}

	s.RegisterEndpoint("ep1", "127.0.0.1:9000", "tcp://remote:443")

	resp, err = http.Get(base + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 with active endpoint, got %d", resp.StatusCode)
	}
}

func TestHealthServer_ReadyEndpoint(t *testing.T) {
	s, base := newTestServer(t)
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	defer s.Stop(context.Background())
	time.Sleep(50 * time.Millisecond)

	resp, err := http.Get(base + "/readyz")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when not ready, got %d", resp.StatusCode)
	}

	s.SetReady(true)

	resp, err = http.Get(base + "/readyz")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 when ready, got %d", resp.StatusCode)
	}
}

func TestHealthServer_StatusEndpoint(t *testing.T) {
	s, base := newTestServer(t)
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	defer s.Stop(context.Background())
	time.Sleep(50 * time.Millisecond)

	s.RegisterEndpoint("web", "127.0.0.1:8080", "tcp://backend:443")

	resp, err := http.Get(base + "/status")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var status Status
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		t.Fatalf("failed to decode status: %v", err)
	}

	ep, ok := status.Endpoints["web"]
	if !ok {
		t.Fatal("endpoint 'web' not found in status")
	}
	if ep.LocalAddress != "127.0.0.1:8080" {
		t.Fatalf("unexpected local address: %s", ep.LocalAddress)
	}
	if ep.TargetURI != "tcp://backend:443" {
		t.Fatalf("unexpected target URI: %s", ep.TargetURI)
	}
}

func TestHealthServer_RecordConnection(t *testing.T) {
	s := NewServer(Config{Port: 1, Logger: logr.Discard()})

	s.RegisterEndpoint("ep1", "127.0.0.1:8080", "tcp://remote:443")

	s.RecordConnection("ep1")

	s.mu.RLock()
	ep := s.endpoints["ep1"]
	if ep.Connections != 1 {
		t.Fatalf("expected 1 connection, got %d", ep.Connections)
	}
	if ep.TotalConnections != 1 {
		t.Fatalf("expected 1 total connection, got %d", ep.TotalConnections)
	}
	s.mu.RUnlock()

	s.RecordConnectionClose("ep1")

	s.mu.RLock()
	ep = s.endpoints["ep1"]
	if ep.Connections != 0 {
		t.Fatalf("expected 0 connections after close, got %d", ep.Connections)
	}
	if ep.TotalConnections != 1 {
		t.Fatalf("total connections should remain 1, got %d", ep.TotalConnections)
	}
	s.mu.RUnlock()
}
