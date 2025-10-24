package cert

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
)

// Provisioner handles certificate generation and provisioning
type Provisioner struct {
	certDir string
}

// NewProvisioner creates a new certificate provisioner
func NewProvisioner(certDir string) *Provisioner {
	return &Provisioner{
		certDir: certDir,
	}
}

// GenerateKeyAndCSR generates a new ECDSA P-384 private key and CSR
func (p *Provisioner) GenerateKeyAndCSR(commonName string) (privateKeyPEM, csrPEM []byte, err error) {
	// Generate ECDSA P-384 private key (same as operator)
	privateKey, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate private key: %w", err)
	}

	// Marshal private key to DER format
	privateKeyBytes, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal private key: %w", err)
	}

	// Encode private key to PEM
	privateKeyPEM = pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: privateKeyBytes,
	})

	// Create CSR template
	// Note: ngrok API requires CommonName to be empty
	template := x509.CertificateRequest{
		Subject: pkix.Name{
			Organization: []string{"ngrok-forward-proxy"},
		},
		SignatureAlgorithm: x509.ECDSAWithSHA384,
	}

	// Create CSR
	csrDER, err := x509.CreateCertificateRequest(rand.Reader, &template, privateKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create CSR: %w", err)
	}

	// Encode CSR to PEM
	csrPEM = pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE REQUEST",
		Bytes: csrDER,
	})

	return privateKeyPEM, csrPEM, nil
}

// SaveCertificate saves the private key and certificate to disk
func (p *Provisioner) SaveCertificate(privateKeyPEM, certPEM []byte) error {
	// Create cert directory if it doesn't exist
	if err := os.MkdirAll(p.certDir, 0700); err != nil {
		return fmt.Errorf("failed to create cert directory: %w", err)
	}

	// Write private key
	keyPath := filepath.Join(p.certDir, "tls.key")
	if err := os.WriteFile(keyPath, privateKeyPEM, 0600); err != nil {
		return fmt.Errorf("failed to write private key: %w", err)
	}

	// Write certificate
	certPath := filepath.Join(p.certDir, "tls.crt")
	if err := os.WriteFile(certPath, certPEM, 0644); err != nil {
		return fmt.Errorf("failed to write certificate: %w", err)
	}

	return nil
}

// LoadCertificate loads the certificate from disk
func (p *Provisioner) LoadCertificate() (tls.Certificate, error) {
	keyPath := filepath.Join(p.certDir, "tls.key")
	certPath := filepath.Join(p.certDir, "tls.crt")

	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("failed to load certificate: %w", err)
	}

	return cert, nil
}

// CertificateExists checks if a certificate already exists on disk
func (p *Provisioner) CertificateExists() bool {
	keyPath := filepath.Join(p.certDir, "tls.key")
	certPath := filepath.Join(p.certDir, "tls.crt")

	_, keyErr := os.Stat(keyPath)
	_, certErr := os.Stat(certPath)

	return keyErr == nil && certErr == nil
}

// GetKeyPath returns the path to the private key file
func (p *Provisioner) GetKeyPath() string {
	return filepath.Join(p.certDir, "tls.key")
}

// GetCertPath returns the path to the certificate file
func (p *Provisioner) GetCertPath() string {
	return filepath.Join(p.certDir, "tls.crt")
}
