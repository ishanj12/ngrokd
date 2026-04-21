package forwarder

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/ishanjain/ngrok-forward-proxy/internal/mux"
	"github.com/ishanjain/ngrok-forward-proxy/pkg/cert"
	"github.com/ishanjain/ngrok-forward-proxy/pkg/sni"
)

// Config holds the configuration for the forwarder
type Config struct {
	// IngressEndpoint is the ngrok ingress endpoint to connect to
	// Default: kubernetes-binding-ingress.ngrok.io:443
	IngressEndpoint string

	// TLSCert is the client certificate for mTLS authentication (deprecated, use CertHolder)
	TLSCert tls.Certificate

	// CertHolder provides dynamic certificate access for hot-reload support.
	// If set, this takes precedence over TLSCert.
	CertHolder *cert.CertHolder

	// RootCAs is an optional custom CA pool for TLS verification
	RootCAs *tls.Config

	// DialTimeout is the timeout for establishing connections
	DialTimeout time.Duration

	// Logger for structured logging
	Logger logr.Logger
}

// BoundEndpoint represents a kubernetes bound endpoint in ngrok cloud
type BoundEndpoint struct {
	Name         string
	URI          string
	Port         int
	LocalPort    int
	LocalAddress string
	RequestedHost string // Original hostname from SNI/Host header (for wildcard/router routing)
}

// Forwarder handles forwarding traffic from local connections to ngrok bound endpoints
type Forwarder struct {
	config    Config
	tlsDialer *tls.Dialer
	logger    logr.Logger
}

// New creates a new Forwarder instance
func New(config Config) (*Forwarder, error) {
	if config.IngressEndpoint == "" {
		config.IngressEndpoint = "kubernetes-binding-ingress.ngrok.io:443"
	}

	if config.DialTimeout == 0 {
		config.DialTimeout = 15 * time.Second
	}

	// Load system root CAs
	rootCAs, err := x509.SystemCertPool()
	if err != nil {
		// Fall back to empty pool if system CAs can't be loaded
		rootCAs = x509.NewCertPool()
	}

	// Load custom ngrok CAs from /etc/ssl/certs/ngrok/ (same as operator)
	if err := loadNgrokCerts(rootCAs); err != nil {
		// Log but don't fail - just means no custom certs available
		config.Logger.V(1).Info("No custom ngrok certs loaded", "error", err)
	}

	tlsConfig := &tls.Config{
		RootCAs: rootCAs,
	}

	if config.CertHolder != nil {
		tlsConfig.GetClientCertificate = config.CertHolder.GetClientCertificate
	} else {
		tlsConfig.Certificates = []tls.Certificate{config.TLSCert}
	}

	// Override with custom RootCAs if provided
	if config.RootCAs != nil && config.RootCAs.RootCAs != nil {
		tlsConfig.RootCAs = config.RootCAs.RootCAs
	}

	tlsDialer := &tls.Dialer{
		NetDialer: &net.Dialer{
			Timeout: config.DialTimeout,
		},
		Config: tlsConfig,
	}

	return &Forwarder{
		config:    config,
		tlsDialer: tlsDialer,
		logger:    config.Logger,
	}, nil
}

// ForwardConnection forwards a single connection to the specified bound endpoint
func (f *Forwarder) ForwardConnection(ctx context.Context, localConn net.Conn, endpoint BoundEndpoint) error {
	// Silently forward - verbose logging only
	f.logger.V(1).Info("forwarding connection",
		"endpoint", endpoint.Name,
		"uri", endpoint.URI,
		"port", endpoint.Port)

	// Step 1: Establish mTLS connection to ngrok ingress
	f.logger.V(1).Info("dialing ingress endpoint", "address", f.config.IngressEndpoint)
	
	// Extract hostname for SNI
	hostname, _, _ := net.SplitHostPort(f.config.IngressEndpoint)
	
	// Clone TLS config and set ServerName for proper SNI
	tlsConfig := f.tlsDialer.Config.Clone()
	if hostname != "" {
		tlsConfig.ServerName = hostname
	}
	
	// Note: ngrok's server uses an intermediate CA not in system trust stores
	// We try to load custom CAs from /etc/ssl/certs/ngrok/ (like the operator does)
	// If that fails, we fall back to InsecureSkipVerify
	// This is safe because mTLS client cert authentication is still enforced
	if tlsConfig.RootCAs == nil || !hasNgrokCerts() {
		tlsConfig.InsecureSkipVerify = true
		f.logger.V(1).Info("Using InsecureSkipVerify (no custom ngrok CAs found at /etc/ssl/certs/ngrok/)")
	} else {
		f.logger.V(1).Info("Using custom ngrok CAs for server verification")
	}
	
	// Create temporary dialer with updated config
	dialer := &tls.Dialer{
		NetDialer: f.tlsDialer.NetDialer,
		Config:    tlsConfig,
	}
	
	ngrokConn, err := dialer.DialContext(ctx, "tcp", f.config.IngressEndpoint)
	if err != nil {
		return fmt.Errorf("failed to dial ingress endpoint %s: %w", f.config.IngressEndpoint, err)
	}
	defer ngrokConn.Close()

	f.logger.V(1).Info("mTLS connection established", "endpoint", f.config.IngressEndpoint)

	// Step 2: Determine the host for the binding protocol upgrade.
	// Use RequestedHost (from SNI/Host header) if set (shared/network mode),
	// otherwise extract from the endpoint URI (static endpoints).
	// For wildcard endpoints in virtual mode, peek at the connection to
	// extract the actual hostname so ngrok cloud, traffic policy, and
	// downstream services (e.g. nginx) see the real host, not "*.example.com".
	var host string
	if endpoint.RequestedHost != "" {
		host = endpoint.RequestedHost
	} else {
		host, err = extractHost(endpoint.URI)
		if err != nil {
			return fmt.Errorf("failed to parse endpoint URI: %w", err)
		}
	}

	// For wildcard endpoints, extract the actual hostname from the connection
	// so the binding protocol and downstream see "a.example.com" not "*.example.com".
	if strings.HasPrefix(host, "*.") {
		realHost, wrappedConn, peekErr := peekHostname(localConn, endpoint.Port)
		if peekErr == nil && realHost != "" {
			f.logger.V(1).Info("extracted real hostname for wildcard",
				"wildcard", host,
				"actual", realHost)
			host = realHost
			localConn = wrappedConn
		}
	}

	f.logger.V(1).Info("upgrading connection", "host", host, "port", endpoint.Port)

	// Step 3: Upgrade connection with binding protocol
	resp, err := mux.UpgradeToBindingConnection(f.logger, ngrokConn, host, endpoint.Port)
	if err != nil {
		return fmt.Errorf("failed to upgrade connection: %w", err)
	}

	f.logger.V(1).Info("connection upgraded",
		"endpointID", resp.EndpointID,
		"proto", resp.Proto)

	// Step 4: Proxy traffic. For exact endpoints, rewrite the Host header
	// to match the endpoint. For wildcard endpoints (where we extracted the
	// real hostname above), use rawProxy to preserve the original Host header.
	if endpoint.RequestedHost != "" || !strings.Contains(endpoint.URI, "*.") {
		err = hostRewritingProxy(localConn, ngrokConn, host, endpoint.Port)
	} else {
		err = rawProxy(localConn, ngrokConn)
	}
	if err != nil {
		f.logger.V(1).Info("connection closed with error", "error", err)
	} else {
		f.logger.V(1).Info("connection closed successfully")
	}

	return err
}

// hasNgrokCerts checks if custom ngrok certs directory exists
func hasNgrokCerts() bool {
	_, err := os.Stat("/etc/ssl/certs/ngrok/")
	return err == nil
}

// loadNgrokCerts loads custom ngrok CA certificates from /etc/ssl/certs/ngrok/
func loadNgrokCerts(pool *x509.CertPool) error {
	certsPath := "/etc/ssl/certs/ngrok/"
	
	entries, err := os.ReadDir(certsPath)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		certPath := certsPath + entry.Name()
		certPEM, err := os.ReadFile(certPath)
		if err != nil {
			continue
		}

		if ok := pool.AppendCertsFromPEM(certPEM); ok {
			// Successfully loaded cert
		}
	}

	return nil
}

// extractHost extracts the hostname from an endpoint URI (without port)
// Example: "http://my-app.ngrok.app:81" -> "my-app.ngrok.app"
func extractHost(endpointURI string) (string, error) {
	parsed, err := url.Parse(endpointURI)
	if err != nil {
		return "", err
	}

	host := parsed.Host
	if host == "" {
		host = parsed.Path
	}

	// Strip port if present
	hostname := host
	if h, _, err := net.SplitHostPort(host); err == nil {
		hostname = h
	}

	return hostname, nil
}

// peekHostname extracts the real hostname from a connection by peeking at
// TLS SNI (for HTTPS/TLS) or the HTTP Host header (for HTTP).
// Returns the hostname, a wrapped connection that replays peeked bytes, and any error.
func peekHostname(conn net.Conn, port int) (string, net.Conn, error) {
	// Try TLS SNI first
	serverName, wrapped, err := sni.PeekClientHelloConn(conn)
	if err == nil && serverName != "" {
		return serverName, wrapped, nil
	}

	// Not TLS — try HTTP Host header via the existing hostRewritingProxy peek logic
	// Use a bufio.Reader to peek at the data
	br := bufio.NewReaderSize(wrapped, 8192)
	peek, peekErr := br.Peek(4)
	readerW := &readerConn{Reader: br, Conn: wrapped}
	if peekErr != nil || !looksLikeHTTP(peek) {
		return "", readerW, fmt.Errorf("not HTTP")
	}

	// Peek at buffered data to find Host header
	avail := br.Buffered()
	if avail < 4 {
		avail = 4
	}
	peek, _ = br.Peek(avail)
	host := scanHostHeader(peek)
	if host == "" {
		return "", readerW, fmt.Errorf("no Host header found")
	}

	// Strip port from Host value
	if h, _, splitErr := net.SplitHostPort(host); splitErr == nil {
		host = h
	}

	return host, readerW, nil
}

// scanHostHeader scans raw HTTP header bytes for the Host header value.
func scanHostHeader(data []byte) string {
	s := string(data)
	idx := strings.Index(s, "\r\n")
	if idx < 0 {
		idx = strings.Index(s, "\n")
		if idx < 0 {
			return ""
		}
	}
	headers := s[idx:]

	for _, prefix := range []string{"\r\nHost: ", "\nHost: ", "\r\nhost: ", "\nhost: "} {
		i := strings.Index(strings.ToLower(headers), strings.ToLower(prefix))
		if i < 0 {
			continue
		}
		valStart := i + len(prefix)
		rest := headers[valStart:]
		end := strings.IndexAny(rest, "\r\n")
		if end < 0 {
			return ""
		}
		return strings.TrimSpace(rest[:end])
	}
	return ""
}
