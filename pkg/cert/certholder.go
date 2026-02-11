package cert

import (
	"crypto/tls"
	"sync"
)

// CertHolder provides thread-safe access to a TLS certificate that can be
// dynamically updated. It implements the signature expected by
// tls.Config.GetClientCertificate for zero-downtime certificate rotation.
type CertHolder struct {
	mu   sync.RWMutex
	cert *tls.Certificate
}

// NewCertHolder creates a new CertHolder initialized with the given certificate.
func NewCertHolder(cert tls.Certificate) *CertHolder {
	return &CertHolder{
		cert: &cert,
	}
}

// GetClientCertificate returns the current certificate. This method signature
// matches tls.Config.GetClientCertificate and is called on each TLS handshake.
func (h *CertHolder) GetClientCertificate(*tls.CertificateRequestInfo) (*tls.Certificate, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.cert, nil
}

// Update replaces the current certificate with a new one. In-flight connections
// are unaffected; new connections will use the updated certificate.
func (h *CertHolder) Update(cert tls.Certificate) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.cert = &cert
}

// Current returns a copy of the current certificate.
func (h *CertHolder) Current() tls.Certificate {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return *h.cert
}
