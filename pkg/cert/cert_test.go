package cert

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func generateSelfSignedCert(t *testing.T) (keyPEM, certPEM []byte) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{Organization: []string{"test"}},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
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
	return keyPEM, certPEM
}

func makeTLSCert(t *testing.T) tls.Certificate {
	t.Helper()
	keyPEM, certPEM := generateSelfSignedCert(t)
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	return cert
}

// --- CertHolder tests ---

func TestCertHolder_NewAndCurrent(t *testing.T) {
	cert := makeTLSCert(t)
	h := NewCertHolder(cert)

	got := h.Current()
	if len(got.Certificate) != len(cert.Certificate) {
		t.Fatalf("expected %d certs, got %d", len(cert.Certificate), len(got.Certificate))
	}
	for i := range cert.Certificate {
		if string(got.Certificate[i]) != string(cert.Certificate[i]) {
			t.Fatalf("certificate[%d] mismatch", i)
		}
	}
}

func TestCertHolder_Update(t *testing.T) {
	cert1 := makeTLSCert(t)
	cert2 := makeTLSCert(t)

	h := NewCertHolder(cert1)
	h.Update(cert2)

	got := h.Current()
	if string(got.Certificate[0]) == string(cert1.Certificate[0]) {
		t.Fatal("expected certificate to be updated, but still holds old cert")
	}
	if string(got.Certificate[0]) != string(cert2.Certificate[0]) {
		t.Fatal("expected certificate to match cert2")
	}
}

func TestCertHolder_GetClientCertificate(t *testing.T) {
	cert := makeTLSCert(t)
	h := NewCertHolder(cert)

	got, err := h.GetClientCertificate(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil certificate")
	}
	if len(got.Certificate) == 0 {
		t.Fatal("expected certificate to have at least one entry")
	}
}

func TestCertHolder_ConcurrentAccess(t *testing.T) {
	cert := makeTLSCert(t)
	h := NewCertHolder(cert)

	var wg sync.WaitGroup
	const goroutines = 50

	for i := 0; i < goroutines; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			newCert := makeTLSCert(t)
			h.Update(newCert)
		}()
		go func() {
			defer wg.Done()
			c, err := h.GetClientCertificate(nil)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if c == nil {
				t.Error("expected non-nil certificate")
			}
		}()
	}
	wg.Wait()
}

// --- Provisioner tests ---

func TestProvisioner_GenerateKeyAndCSR(t *testing.T) {
	p := NewProvisioner(t.TempDir())

	keyPEM, csrPEM, err := p.GenerateKeyAndCSR()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(keyPEM) == 0 {
		t.Fatal("expected non-empty private key PEM")
	}
	if len(csrPEM) == 0 {
		t.Fatal("expected non-empty CSR PEM")
	}

	keyBlock, _ := pem.Decode(keyPEM)
	if keyBlock == nil {
		t.Fatal("failed to decode private key PEM")
	}
	if keyBlock.Type != "EC PRIVATE KEY" {
		t.Fatalf("expected key type 'EC PRIVATE KEY', got %q", keyBlock.Type)
	}

	csrBlock, _ := pem.Decode(csrPEM)
	if csrBlock == nil {
		t.Fatal("failed to decode CSR PEM")
	}
	if csrBlock.Type != "CERTIFICATE REQUEST" {
		t.Fatalf("expected CSR type 'CERTIFICATE REQUEST', got %q", csrBlock.Type)
	}

	_, err = x509.ParseCertificateRequest(csrBlock.Bytes)
	if err != nil {
		t.Fatalf("failed to parse CSR: %v", err)
	}
}

func TestProvisioner_SaveAndLoadCertificate(t *testing.T) {
	dir := t.TempDir()
	p := NewProvisioner(dir)

	keyPEM, certPEM := generateSelfSignedCert(t)

	if err := p.SaveCertificate(keyPEM, certPEM); err != nil {
		t.Fatalf("SaveCertificate error: %v", err)
	}

	loaded, err := p.LoadCertificate()
	if err != nil {
		t.Fatalf("LoadCertificate error: %v", err)
	}

	expected, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatalf("X509KeyPair error: %v", err)
	}

	if len(loaded.Certificate) != len(expected.Certificate) {
		t.Fatalf("certificate count mismatch: got %d, want %d", len(loaded.Certificate), len(expected.Certificate))
	}
	if string(loaded.Certificate[0]) != string(expected.Certificate[0]) {
		t.Fatal("loaded certificate does not match saved certificate")
	}
}

func TestProvisioner_CertificateExists(t *testing.T) {
	dir := t.TempDir()
	p := NewProvisioner(dir)

	if p.CertificateExists() {
		t.Fatal("expected CertificateExists to return false before saving")
	}

	keyPEM, certPEM := generateSelfSignedCert(t)
	if err := p.SaveCertificate(keyPEM, certPEM); err != nil {
		t.Fatalf("SaveCertificate error: %v", err)
	}

	if !p.CertificateExists() {
		t.Fatal("expected CertificateExists to return true after saving")
	}
}

func TestProvisioner_EnsureCSR(t *testing.T) {
	dir := t.TempDir()
	p := NewProvisioner(dir)

	keyPEM, _ := generateSelfSignedCert(t)
	keyPath := filepath.Join(dir, "tls.key")
	if err := os.WriteFile(keyPath, keyPEM, 0600); err != nil {
		t.Fatalf("failed to write key file: %v", err)
	}

	csrPath := filepath.Join(dir, "tls.csr")
	if _, err := os.Stat(csrPath); err == nil {
		t.Fatal("expected CSR file to not exist yet")
	}

	csrPEM, err := p.EnsureCSR()
	if err != nil {
		t.Fatalf("EnsureCSR error: %v", err)
	}
	if len(csrPEM) == 0 {
		t.Fatal("expected non-empty CSR PEM")
	}

	block, _ := pem.Decode(csrPEM)
	if block == nil || block.Type != "CERTIFICATE REQUEST" {
		t.Fatal("expected valid CERTIFICATE REQUEST PEM block")
	}

	if _, err := os.Stat(csrPath); err != nil {
		t.Fatalf("expected CSR file to exist after EnsureCSR: %v", err)
	}

	csrPEM2, err := p.EnsureCSR()
	if err != nil {
		t.Fatalf("second EnsureCSR error: %v", err)
	}
	if string(csrPEM2) != string(csrPEM) {
		t.Fatal("expected second EnsureCSR to return cached CSR")
	}
}
