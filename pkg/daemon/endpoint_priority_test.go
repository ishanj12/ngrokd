package daemon

import (
	"sort"
	"testing"

	"github.com/ngrok/ngrokd/pkg/ngrokapi"
)

func TestEndpointBindPriority(t *testing.T) {
	tests := []struct {
		url      string
		wantPrio int
	}{
		{"https://api.example.com:443", 0},
		{"http://web.example.com:80", 0},
		{"tls://*.example.com:443", 0},
		{"tcp://db.example.com:443", 1},
		{"tcp://raw.example.com:5432", 1},
	}

	for _, tt := range tests {
		ep := ngrokapi.Endpoint{URL: tt.url}
		got := endpointBindPriority(ep)
		if got != tt.wantPrio {
			t.Errorf("endpointBindPriority(%q) = %d, want %d", tt.url, got, tt.wantPrio)
		}
	}
}

func TestEndpointSortOrder(t *testing.T) {
	// Simulate CircleCI's scenario: TCP :443 and TLS wildcard :443
	endpoints := []ngrokapi.Endpoint{
		{ID: "tcp-443", URL: "tcp://db.example.com:443"},
		{ID: "tls-wildcard", URL: "tls://*.example.com:443"},
	}

	sort.SliceStable(endpoints, func(i, j int) bool {
		return endpointBindPriority(endpoints[i]) < endpointBindPriority(endpoints[j])
	})

	if endpoints[0].ID != "tls-wildcard" {
		t.Errorf("expected tls-wildcard first, got %s", endpoints[0].ID)
	}
	if endpoints[1].ID != "tcp-443" {
		t.Errorf("expected tcp-443 second, got %s", endpoints[1].ID)
	}
}

func TestEndpointSortOrderMixed(t *testing.T) {
	// Multiple protocols competing for various ports
	endpoints := []ngrokapi.Endpoint{
		{ID: "tcp-443", URL: "tcp://raw.example.com:443"},
		{ID: "http-80", URL: "http://web.example.com:80"},
		{ID: "tcp-5432", URL: "tcp://db.example.com:5432"},
		{ID: "https-443", URL: "https://api.example.com:443"},
		{ID: "tls-443", URL: "tls://*.example.com:443"},
	}

	sort.SliceStable(endpoints, func(i, j int) bool {
		return endpointBindPriority(endpoints[i]) < endpointBindPriority(endpoints[j])
	})

	// All shared-eligible (http/https/tls) should come before tcp
	for i, ep := range endpoints {
		prio := endpointBindPriority(ep)
		if i < 3 && prio != 0 {
			t.Errorf("position %d: expected shared-eligible (prio 0), got %q (prio %d)", i, ep.URL, prio)
		}
		if i >= 3 && prio != 1 {
			t.Errorf("position %d: expected non-shared (prio 1), got %q (prio %d)", i, ep.URL, prio)
		}
	}
}
