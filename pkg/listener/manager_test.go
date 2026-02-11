package listener

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"math/big"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/ishanjain/ngrok-forward-proxy/pkg/cert"
	"github.com/ishanjain/ngrok-forward-proxy/pkg/forwarder"
)

func newTestManager(t *testing.T) *Manager {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(time.Hour),
	}
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("failed to create certificate: %v", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("failed to marshal key: %v", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatalf("failed to create X509 key pair: %v", err)
	}

	fwd, err := forwarder.New(forwarder.Config{
		CertHolder: cert.NewCertHolder(tlsCert),
		Logger:     logr.Discard(),
	})
	if err != nil {
		t.Fatalf("failed to create forwarder: %v", err)
	}

	return New(fwd, logr.Discard())
}

func TestManager_StartStopListener(t *testing.T) {
	mgr := newTestManager(t)

	ep := forwarder.BoundEndpoint{
		Name:         "test-ep-1",
		URI:          "http://test.ngrok.app",
		Port:         80,
		LocalPort:    19999,
		LocalAddress: "127.0.0.1",
	}

	if err := mgr.StartListener(context.Background(), ep); err != nil {
		t.Fatalf("StartListener() error = %v", err)
	}

	active := mgr.ListActiveEndpoints()
	if len(active) != 1 || active[0] != "test-ep-1" {
		t.Fatalf("expected [test-ep-1], got %v", active)
	}

	if err := mgr.StopListener("test-ep-1"); err != nil {
		t.Fatalf("StopListener() error = %v", err)
	}

	active = mgr.ListActiveEndpoints()
	if len(active) != 0 {
		t.Fatalf("expected empty, got %v", active)
	}
}

func TestManager_StartDuplicateListener(t *testing.T) {
	mgr := newTestManager(t)
	defer mgr.Close()

	ep := forwarder.BoundEndpoint{
		Name:         "dup-ep",
		URI:          "http://dup.ngrok.app",
		Port:         80,
		LocalPort:    19998,
		LocalAddress: "127.0.0.1",
	}

	if err := mgr.StartListener(context.Background(), ep); err != nil {
		t.Fatalf("first StartListener() error = %v", err)
	}

	err := mgr.StartListener(context.Background(), ep)
	if err == nil {
		t.Fatal("expected error for duplicate listener, got nil")
	}
}

func TestManager_StopNonexistentListener(t *testing.T) {
	mgr := newTestManager(t)
	defer mgr.Close()

	err := mgr.StopListener("does-not-exist")
	if err == nil {
		t.Fatal("expected error for nonexistent listener, got nil")
	}
}

func TestManager_CloseAll(t *testing.T) {
	mgr := newTestManager(t)

	ep1 := forwarder.BoundEndpoint{
		Name:         "close-ep-1",
		URI:          "http://close1.ngrok.app",
		Port:         80,
		LocalPort:    19997,
		LocalAddress: "127.0.0.1",
	}
	ep2 := forwarder.BoundEndpoint{
		Name:         "close-ep-2",
		URI:          "http://close2.ngrok.app",
		Port:         80,
		LocalPort:    19996,
		LocalAddress: "127.0.0.1",
	}

	if err := mgr.StartListener(context.Background(), ep1); err != nil {
		t.Fatalf("StartListener(ep1) error = %v", err)
	}
	if err := mgr.StartListener(context.Background(), ep2); err != nil {
		t.Fatalf("StartListener(ep2) error = %v", err)
	}

	active := mgr.ListActiveEndpoints()
	if len(active) != 2 {
		t.Fatalf("expected 2 active endpoints, got %d", len(active))
	}

	if err := mgr.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	active = mgr.ListActiveEndpoints()
	if len(active) != 0 {
		t.Fatalf("expected empty after Close(), got %v", active)
	}
}
