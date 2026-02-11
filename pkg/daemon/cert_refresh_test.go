package daemon

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/ishanjain/ngrok-forward-proxy/pkg/cert"
	"github.com/ishanjain/ngrok-forward-proxy/pkg/config"
	"github.com/ishanjain/ngrok-forward-proxy/pkg/health"
	"github.com/ishanjain/ngrok-forward-proxy/pkg/ngrokapi"
	"github.com/ishanjain/ngrok-forward-proxy/pkg/socket"
)

func generateTestCert(t *testing.T, notBefore, notAfter time.Time) (keyPEM, certPEM []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return generateTestCertWithKey(t, key, notBefore, notAfter)
}

func generateTestCertWithKey(t *testing.T, key *ecdsa.PrivateKey, notBefore, notAfter time.Time) (keyPEM, certPEM []byte) {
	t.Helper()

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{Organization: []string{"test"}},
		NotBefore:    notBefore,
		NotAfter:     notAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}

	keyBytes, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}

	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes})
	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	return
}

func loadTestKey(t *testing.T, keyPEM []byte) *ecdsa.PrivateKey {
	t.Helper()
	block, _ := pem.Decode(keyPEM)
	if block == nil {
		t.Fatal("failed to decode key PEM")
	}
	key, err := x509.ParseECPrivateKey(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func setupTestDaemon(t *testing.T, certNotBefore, certNotAfter time.Time) (*Daemon, string) {
	t.Helper()
	dir := t.TempDir()

	keyPEM, certPEM := generateTestCert(t, certNotBefore, certNotAfter)
	certPath := filepath.Join(dir, "tls.crt")
	keyPath := filepath.Join(dir, "tls.key")
	if err := os.WriteFile(certPath, certPEM, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0600); err != nil {
		t.Fatal(err)
	}

	provisioner := cert.NewProvisioner(dir)
	if _, err := provisioner.EnsureCSR(); err != nil {
		t.Fatal(err)
	}

	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	healthSrv := health.NewServer(health.Config{
		Address: "127.0.0.1",
		Port:    0,
		Logger:  logr.Discard(),
	})

	d := &Daemon{
		config: &config.DaemonConfig{
			Server: config.ServerConfig{
				ClientCert:          certPath,
				ClientKey:           keyPath,
				CertRefreshInterval: 1,
			},
		},
		logger:             logr.Discard(),
		certHolder:         cert.NewCertHolder(tlsCert),
		operatorID:         "op_test123",
		registered:         true,
		endpoints:          make(map[string]socket.EndpointInfo),
		networkPortsByHost: make(map[string]int),
		ctx:                ctx,
		cancel:             cancel,
		healthServer:       healthSrv,
	}

	return d, dir
}

func TestRefreshCert_SkipsWhenCertStillValid(t *testing.T) {
	now := time.Now()
	d, _ := setupTestDaemon(t, now.Add(-1*time.Hour), now.Add(24*time.Hour))

	apiCalled := false
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiCalled = true
		t.Error("API should not be called when cert is still valid")
		http.Error(w, "should not be called", http.StatusInternalServerError)
	}))
	defer ts.Close()

	client := ngrokapi.NewClient("test-key")
	client.SetBaseURL(ts.URL)
	d.apiClient = client

	d.refreshCert()

	if apiCalled {
		t.Fatal("API was called when cert is still valid (remaining > 1/3 lifetime)")
	}
}

func TestRefreshCert_RenewsWhenCertApproachingExpiry(t *testing.T) {
	now := time.Now()
	d, dir := setupTestDaemon(t, now.Add(-48*time.Hour), now.Add(2*time.Hour))

	existingKeyPEM, _ := os.ReadFile(filepath.Join(dir, "tls.key"))
	key := loadTestKey(t, existingKeyPEM)
	_, renewedCertPEM := generateTestCertWithKey(t, key, now, now.Add(72*time.Hour))

	apiCalled := false
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiCalled = true
		if r.Method != http.MethodPatch {
			t.Errorf("expected PATCH, got %s", r.Method)
		}
		if r.URL.Path != "/kubernetes_operators/op_test123" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		var req ngrokapi.KubernetesOperatorUpdate
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("failed to decode request body: %v", err)
		}
		if req.Binding == nil || req.Binding.CSR == nil || *req.Binding.CSR == "" {
			t.Error("expected CSR in update request")
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ngrokapi.KubernetesOperator{
			ID: "op_test123",
			Binding: &ngrokapi.KubernetesOperatorBinding{
				Cert: ngrokapi.KubernetesOperatorCert{
					Cert:      string(renewedCertPEM),
					NotBefore: now.Format(time.RFC3339),
					NotAfter:  now.Add(72 * time.Hour).Format(time.RFC3339),
				},
			},
		})
	}))
	defer ts.Close()

	client := ngrokapi.NewClient("test-key")
	client.SetBaseURL(ts.URL)
	d.apiClient = client

	d.refreshCert()

	if !apiCalled {
		t.Fatal("API was not called when cert is approaching expiry")
	}

	savedCert, err := os.ReadFile(filepath.Join(dir, "tls.crt"))
	if err != nil {
		t.Fatalf("failed to read saved cert: %v", err)
	}
	if string(savedCert) != string(renewedCertPEM) {
		t.Error("saved cert does not match renewed cert from API")
	}

	currentCert := d.certHolder.Current()
	if len(currentCert.Certificate) == 0 {
		t.Fatal("CertHolder should have a certificate after renewal")
	}
	x509Current, err := x509.ParseCertificate(currentCert.Certificate[0])
	if err != nil {
		t.Fatalf("failed to parse current cert: %v", err)
	}
	expectedExpiry := now.Add(72 * time.Hour)
	diff := x509Current.NotAfter.Sub(expectedExpiry)
	if diff < -time.Second || diff > time.Second {
		t.Errorf("CertHolder cert expiry mismatch: got %v, expected ~%v", x509Current.NotAfter, expectedExpiry)
	}
}

func TestRefreshCert_ThresholdBoundary(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name        string
		notBefore   time.Time
		notAfter    time.Time
		expectRenew bool
	}{
		{
			name:        "just_above_threshold",
			notBefore:   now.Add(-6 * time.Hour),
			notAfter:    now.Add(3*time.Hour + 1*time.Minute),
			expectRenew: false,
		},
		{
			name:        "exactly_at_threshold",
			notBefore:   now.Add(-6 * time.Hour),
			notAfter:    now.Add(3 * time.Hour),
			expectRenew: true, // remaining == threshold, not strictly >, so renewal triggers
		},
		{
			name:        "just_below_threshold",
			notBefore:   now.Add(-6 * time.Hour),
			notAfter:    now.Add(3*time.Hour - 1*time.Minute),
			expectRenew: true,
		},
		{
			name:        "expired",
			notBefore:   now.Add(-48 * time.Hour),
			notAfter:    now.Add(-1 * time.Hour),
			expectRenew: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d, dir := setupTestDaemon(t, tt.notBefore, tt.notAfter)

			existingKeyPEM, _ := os.ReadFile(filepath.Join(dir, "tls.key"))
			key := loadTestKey(t, existingKeyPEM)

			apiCalled := false
			_, renewedCertPEM := generateTestCertWithKey(t, key, now, now.Add(72*time.Hour))
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				apiCalled = true
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(ngrokapi.KubernetesOperator{
					ID: "op_test123",
					Binding: &ngrokapi.KubernetesOperatorBinding{
						Cert: ngrokapi.KubernetesOperatorCert{
							Cert:      string(renewedCertPEM),
							NotBefore: now.Format(time.RFC3339),
							NotAfter:  now.Add(72 * time.Hour).Format(time.RFC3339),
						},
					},
				})
			}))
			defer ts.Close()

			client := ngrokapi.NewClient("test-key")
			client.SetBaseURL(ts.URL)
			d.apiClient = client

			d.refreshCert()

			if apiCalled != tt.expectRenew {
				t.Errorf("apiCalled = %v, want %v", apiCalled, tt.expectRenew)
			}
		})
	}
}

func TestRefreshCert_Handles404_ClearsState(t *testing.T) {
	now := time.Now()
	d, dir := setupTestDaemon(t, now.Add(-48*time.Hour), now.Add(2*time.Hour))

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch {
			http.Error(w, `{"msg":"not found"}`, http.StatusNotFound)
			return
		}
		http.Error(w, "unexpected request", http.StatusBadRequest)
	}))
	defer ts.Close()

	client := ngrokapi.NewClient("test-key")
	client.SetBaseURL(ts.URL)
	d.apiClient = client

	cfg := config.DaemonConfig{
		API:             config.APIConfig{Key: "test-key"},
		IngressEndpoint: "test-ingress:443",
		Server: config.ServerConfig{
			ClientCert: filepath.Join(dir, "tls.crt"),
			ClientKey:  filepath.Join(dir, "tls.key"),
		},
	}
	cfg.SetDefaults()
	d.config = &cfg
	d.configPath = filepath.Join(dir, "config.yml")

	d.refreshCert()

	d.mu.RLock()
	registered := d.registered
	operatorID := d.operatorID
	d.mu.RUnlock()

	if registered {
		t.Error("daemon should be marked unregistered after 404")
	}
	if operatorID != "" {
		t.Error("operator ID should be cleared after 404")
	}
}

func TestRefreshCert_Handles401(t *testing.T) {
	now := time.Now()
	d, _ := setupTestDaemon(t, now.Add(-48*time.Hour), now.Add(2*time.Hour))

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"msg":"unauthorized"}`, http.StatusUnauthorized)
	}))
	defer ts.Close()

	client := ngrokapi.NewClient("test-key")
	client.SetBaseURL(ts.URL)
	d.apiClient = client

	origOperatorID := d.operatorID
	d.refreshCert()

	if d.operatorID != origOperatorID {
		t.Error("operator ID should not change on 401 error")
	}
	if !d.registered {
		t.Error("registered flag should remain true on 401 error")
	}
}

func TestRefreshCert_Handles403(t *testing.T) {
	now := time.Now()
	d, _ := setupTestDaemon(t, now.Add(-48*time.Hour), now.Add(2*time.Hour))

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"msg":"forbidden"}`, http.StatusForbidden)
	}))
	defer ts.Close()

	client := ngrokapi.NewClient("test-key")
	client.SetBaseURL(ts.URL)
	d.apiClient = client

	origOperatorID := d.operatorID
	d.refreshCert()

	if d.operatorID != origOperatorID {
		t.Error("operator ID should not change on 403 error")
	}
}

func TestRefreshCert_NoCertReturned(t *testing.T) {
	now := time.Now()
	d, dir := setupTestDaemon(t, now.Add(-48*time.Hour), now.Add(2*time.Hour))

	origCert, _ := os.ReadFile(filepath.Join(dir, "tls.crt"))

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ngrokapi.KubernetesOperator{
			ID:      "op_test123",
			Binding: nil,
		})
	}))
	defer ts.Close()

	client := ngrokapi.NewClient("test-key")
	client.SetBaseURL(ts.URL)
	d.apiClient = client

	d.refreshCert()

	currentCert, _ := os.ReadFile(filepath.Join(dir, "tls.crt"))
	if string(currentCert) != string(origCert) {
		t.Error("cert on disk should not change when API returns no cert")
	}
}

func TestRefreshCert_MissingCertFile(t *testing.T) {
	now := time.Now()
	d, dir := setupTestDaemon(t, now.Add(-48*time.Hour), now.Add(2*time.Hour))

	os.Remove(filepath.Join(dir, "tls.crt"))

	apiCalled := false
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiCalled = true
	}))
	defer ts.Close()

	client := ngrokapi.NewClient("test-key")
	client.SetBaseURL(ts.URL)
	d.apiClient = client

	d.refreshCert()

	if apiCalled {
		t.Error("API should not be called when cert file is missing")
	}
}

func TestRefreshCert_InvalidCertPEM(t *testing.T) {
	now := time.Now()
	d, dir := setupTestDaemon(t, now.Add(-48*time.Hour), now.Add(2*time.Hour))

	os.WriteFile(filepath.Join(dir, "tls.crt"), []byte("not valid pem"), 0644)

	apiCalled := false
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiCalled = true
	}))
	defer ts.Close()

	client := ngrokapi.NewClient("test-key")
	client.SetBaseURL(ts.URL)
	d.apiClient = client

	d.refreshCert()

	if apiCalled {
		t.Error("API should not be called when cert PEM is invalid")
	}
}

func TestRefreshCert_MissingCSRFile(t *testing.T) {
	now := time.Now()
	d, dir := setupTestDaemon(t, now.Add(-48*time.Hour), now.Add(2*time.Hour))

	os.Remove(filepath.Join(dir, "tls.csr"))

	apiCalled := false
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiCalled = true
	}))
	defer ts.Close()

	client := ngrokapi.NewClient("test-key")
	client.SetBaseURL(ts.URL)
	d.apiClient = client

	d.refreshCert()

	if !apiCalled {
		t.Error("API should be called - EnsureCSR regenerates from private key")
	}
}

func TestRefreshCert_CertHolderHotReload(t *testing.T) {
	now := time.Now()
	d, dir := setupTestDaemon(t, now.Add(-48*time.Hour), now.Add(2*time.Hour))

	origCert := d.certHolder.Current()

	existingKeyPEM, _ := os.ReadFile(filepath.Join(dir, "tls.key"))
	key := loadTestKey(t, existingKeyPEM)
	_, renewedCertPEM := generateTestCertWithKey(t, key, now, now.Add(72*time.Hour))

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ngrokapi.KubernetesOperator{
			ID: "op_test123",
			Binding: &ngrokapi.KubernetesOperatorBinding{
				Cert: ngrokapi.KubernetesOperatorCert{
					Cert:      string(renewedCertPEM),
					NotBefore: now.Format(time.RFC3339),
					NotAfter:  now.Add(72 * time.Hour).Format(time.RFC3339),
				},
			},
		})
	}))
	defer ts.Close()

	client := ngrokapi.NewClient("test-key")
	client.SetBaseURL(ts.URL)
	d.apiClient = client

	d.refreshCert()

	newCert := d.certHolder.Current()
	if string(newCert.Certificate[0]) == string(origCert.Certificate[0]) {
		t.Error("CertHolder should have been updated with renewed cert")
	}
}

func TestRefreshCert_ConcurrentAccess(t *testing.T) {
	now := time.Now()
	d, dir := setupTestDaemon(t, now.Add(-48*time.Hour), now.Add(2*time.Hour))

	existingKeyPEM, _ := os.ReadFile(filepath.Join(dir, "tls.key"))
	key := loadTestKey(t, existingKeyPEM)
	_, renewedCertPEM := generateTestCertWithKey(t, key, now, now.Add(72*time.Hour))

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(10 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ngrokapi.KubernetesOperator{
			ID: "op_test123",
			Binding: &ngrokapi.KubernetesOperatorBinding{
				Cert: ngrokapi.KubernetesOperatorCert{
					Cert:      string(renewedCertPEM),
					NotBefore: now.Format(time.RFC3339),
					NotAfter:  now.Add(72 * time.Hour).Format(time.RFC3339),
				},
			},
		})
	}))
	defer ts.Close()

	client := ngrokapi.NewClient("test-key")
	client.SetBaseURL(ts.URL)
	d.apiClient = client

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			d.refreshCert()
		}()
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = d.certHolder.GetClientCertificate(nil)
		}()
	}
	wg.Wait()
}

func TestRefreshCert_APIServerDown(t *testing.T) {
	now := time.Now()
	d, dir := setupTestDaemon(t, now.Add(-48*time.Hour), now.Add(2*time.Hour))

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer ts.Close()

	client := ngrokapi.NewClient("test-key")
	client.SetBaseURL(ts.URL)
	d.apiClient = client

	origCert, _ := os.ReadFile(filepath.Join(dir, "tls.crt"))

	d.refreshCert()

	currentCert, _ := os.ReadFile(filepath.Join(dir, "tls.crt"))
	if string(currentCert) != string(origCert) {
		t.Error("cert on disk should not change on API server error")
	}
	if !d.registered {
		t.Error("should remain registered on transient API error")
	}
}

func TestCertRefreshLoop_RespectsContextCancel(t *testing.T) {
	now := time.Now()
	d, _ := setupTestDaemon(t, now.Add(-1*time.Hour), now.Add(24*time.Hour))

	d.config.Server.CertRefreshInterval = 1

	client := ngrokapi.NewClient("test-key")
	d.apiClient = client

	done := make(chan struct{})
	go func() {
		d.certRefreshLoop()
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)
	d.cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("certRefreshLoop did not stop after context cancellation")
	}
}

func TestRefreshCert_SendsCSRInRequest(t *testing.T) {
	now := time.Now()
	d, dir := setupTestDaemon(t, now.Add(-48*time.Hour), now.Add(2*time.Hour))

	csrPEM, err := os.ReadFile(filepath.Join(dir, "tls.csr"))
	if err != nil {
		t.Fatalf("failed to read CSR: %v", err)
	}

	var receivedCSR string
	_, renewedCertPEM := generateTestCert(t, now, now.Add(72*time.Hour))

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req ngrokapi.KubernetesOperatorUpdate
		json.NewDecoder(r.Body).Decode(&req)
		if req.Binding != nil && req.Binding.CSR != nil {
			receivedCSR = *req.Binding.CSR
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ngrokapi.KubernetesOperator{
			ID: "op_test123",
			Binding: &ngrokapi.KubernetesOperatorBinding{
				Cert: ngrokapi.KubernetesOperatorCert{
					Cert:      string(renewedCertPEM),
					NotBefore: now.Format(time.RFC3339),
					NotAfter:  now.Add(72 * time.Hour).Format(time.RFC3339),
				},
			},
		})
	}))
	defer ts.Close()

	client := ngrokapi.NewClient("test-key")
	client.SetBaseURL(ts.URL)
	d.apiClient = client

	d.refreshCert()

	if receivedCSR != string(csrPEM) {
		t.Error("CSR sent to API does not match stored CSR")
	}
}

func TestRefreshCert_PreservesKeyOnRenewal(t *testing.T) {
	now := time.Now()
	d, dir := setupTestDaemon(t, now.Add(-48*time.Hour), now.Add(2*time.Hour))

	origKey, err := os.ReadFile(filepath.Join(dir, "tls.key"))
	if err != nil {
		t.Fatal(err)
	}

	_, renewedCertPEM := generateTestCert(t, now, now.Add(72*time.Hour))

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ngrokapi.KubernetesOperator{
			ID: "op_test123",
			Binding: &ngrokapi.KubernetesOperatorBinding{
				Cert: ngrokapi.KubernetesOperatorCert{
					Cert:      string(renewedCertPEM),
					NotBefore: now.Format(time.RFC3339),
					NotAfter:  now.Add(72 * time.Hour).Format(time.RFC3339),
				},
			},
		})
	}))
	defer ts.Close()

	client := ngrokapi.NewClient("test-key")
	client.SetBaseURL(ts.URL)
	d.apiClient = client

	d.refreshCert()

	currentKey, err := os.ReadFile(filepath.Join(dir, "tls.key"))
	if err != nil {
		t.Fatal(err)
	}
	if string(currentKey) != string(origKey) {
		t.Error("private key on disk should not change during renewal (only cert rotates)")
	}
}
