package ngrokapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestIsNotFound(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil error", nil, false},
		{"404 error", fmt.Errorf("API error 404: not found"), true},
		{"other error", fmt.Errorf("something else"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsNotFound(tt.err); got != tt.want {
				t.Errorf("IsNotFound(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestIsAuthError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil error", nil, false},
		{"401 error", fmt.Errorf("API error 401: unauthorized"), true},
		{"403 error", fmt.Errorf("API error 403: forbidden"), true},
		{"404 error", fmt.Errorf("API error 404: not found"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsAuthError(tt.err); got != tt.want {
				t.Errorf("IsAuthError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestListBoundEndpoints_Pagination(t *testing.T) {
	requestCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if auth := r.Header.Get("Authorization"); auth != "Bearer test-key" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		w.Header().Set("Content-Type", "application/json")

		if strings.HasSuffix(r.URL.Path, "/bound_endpoints") && requestCount == 0 {
			requestCount++
			json.NewEncoder(w).Encode(map[string]interface{}{
				"endpoints": []Endpoint{
					{ID: "ep1", URL: "http://ep1.ngrok.app"},
					{ID: "ep2", URL: "http://ep2.ngrok.app"},
				},
				"next_page_uri": "/kubernetes_operators/op123/bound_endpoints?before_id=ep2",
			})
			return
		}

		requestCount++
		json.NewEncoder(w).Encode(map[string]interface{}{
			"endpoints": []Endpoint{
				{ID: "ep3", URL: "http://ep3.ngrok.app"},
			},
			"next_page_uri": "",
		})
	}))
	defer ts.Close()

	client := NewClient("test-key")
	client.baseURL = ts.URL

	endpoints, err := client.ListBoundEndpoints(context.Background(), "op123")
	if err != nil {
		t.Fatalf("ListBoundEndpoints() error = %v", err)
	}
	if len(endpoints) != 3 {
		t.Fatalf("expected 3 endpoints, got %d", len(endpoints))
	}
	if endpoints[0].ID != "ep1" || endpoints[1].ID != "ep2" || endpoints[2].ID != "ep3" {
		t.Errorf("unexpected endpoint IDs: %v, %v, %v", endpoints[0].ID, endpoints[1].ID, endpoints[2].ID)
	}
}

func TestListBoundEndpoints_EmptyOperatorID(t *testing.T) {
	client := NewClient("test-key")
	_, err := client.ListBoundEndpoints(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty operatorID, got nil")
	}
}

func TestCreateKubernetesOperator(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/kubernetes_operators" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if auth := r.Header.Get("Authorization"); auth != "Bearer test-key" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(KubernetesOperator{
			ID:          "op_test123",
			Description: "test operator",
			Binding: &KubernetesOperatorBinding{
				Cert: KubernetesOperatorCert{
					Cert:      "-----BEGIN CERTIFICATE-----\nMIIB...\n-----END CERTIFICATE-----",
					NotBefore: "2025-01-01T00:00:00Z",
					NotAfter:  "2026-01-01T00:00:00Z",
				},
			},
		})
	}))
	defer ts.Close()

	client := NewClient("test-key")
	client.baseURL = ts.URL

	op, err := client.CreateKubernetesOperator(context.Background(), &KubernetesOperatorCreate{
		Description: "test operator",
		Binding: &KubernetesOperatorBindingCreate{
			CSR: "test-csr",
		},
	})
	if err != nil {
		t.Fatalf("CreateKubernetesOperator() error = %v", err)
	}
	if op.ID != "op_test123" {
		t.Errorf("expected ID op_test123, got %s", op.ID)
	}
	if op.Binding == nil || op.Binding.Cert.Cert == "" {
		t.Error("expected binding cert to be set")
	}
}
