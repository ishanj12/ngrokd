package listener

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/ishanjain/ngrok-forward-proxy/pkg/forwarder"
	"github.com/ishanjain/ngrok-forward-proxy/pkg/routing"
	"github.com/ishanjain/ngrok-forward-proxy/pkg/sni"
)

// sharedListener handles a single TCP listener that routes connections
// to different wildcard endpoints based on TLS SNI or HTTP Host header.
type sharedListener struct {
	key      string // e.g. "wildcard:example.com:443"
	port     int    // original endpoint port (e.g. 443)
	listener net.Listener
	cancel   context.CancelFunc
	table    *routing.Table

	// endpoints tracks which endpoint IDs use this shared listener.
	// When empty, the shared listener should be stopped.
	endpoints map[string]bool
}

// StartSharedListener creates a shared listener on the given address:port.
// Multiple wildcard endpoints can share the same listener; connections are
// routed by TLS SNI or HTTP Host header.
// Returns true if a new listener was created, false if it already existed.
func (m *Manager) StartSharedListener(ctx context.Context, key, addr string, port int, table *routing.Table) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.sharedListeners == nil {
		m.sharedListeners = make(map[string]*sharedListener)
	}

	if _, exists := m.sharedListeners[key]; exists {
		return false, nil
	}

	listenAddr := fmt.Sprintf("%s:%d", addr, port)
	ln, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return false, fmt.Errorf("failed to create shared listener on %s: %w", listenAddr, err)
	}

	listenerCtx, cancel := context.WithCancel(ctx)

	sl := &sharedListener{
		key:       key,
		port:      port,
		listener:  ln,
		cancel:    cancel,
		table:     table,
		endpoints: make(map[string]bool),
	}

	m.sharedListeners[key] = sl

	go m.acceptSharedConnections(listenerCtx, sl)

	m.logger.Info("started shared listener",
		"key", key,
		"address", listenAddr)

	return true, nil
}

// AddSharedEndpoint registers an endpoint ID on an existing shared listener.
func (m *Manager) AddSharedEndpoint(key, endpointID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if sl, exists := m.sharedListeners[key]; exists {
		sl.endpoints[endpointID] = true
	}
}

// RemoveSharedEndpoint unregisters an endpoint from a shared listener.
// Returns true if the shared listener was stopped (no endpoints left).
func (m *Manager) RemoveSharedEndpoint(key, endpointID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	sl, exists := m.sharedListeners[key]
	if !exists {
		return false
	}

	delete(sl.endpoints, endpointID)

	if len(sl.endpoints) == 0 {
		sl.cancel()
		sl.listener.Close()
		delete(m.sharedListeners, key)
		m.logger.Info("stopped shared listener (no endpoints left)", "key", key)
		return true
	}

	return false
}

// HasSharedListener returns true if a shared listener exists for the given key.
func (m *Manager) HasSharedListener(key string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.sharedListeners == nil {
		return false
	}
	_, exists := m.sharedListeners[key]
	return exists
}

func (m *Manager) acceptSharedConnections(ctx context.Context, sl *sharedListener) {
	m.logger.Info("shared accept loop started",
		"key", sl.key,
		"address", sl.listener.Addr().String())

	for {
		if ctx.Err() != nil {
			return
		}

		conn, err := sl.listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			m.logger.Error(err, "shared listener accept error", "key", sl.key)
			time.Sleep(100 * time.Millisecond)
			continue
		}

		go m.handleSharedConnection(ctx, sl, conn)
	}
}

func (m *Manager) handleSharedConnection(ctx context.Context, sl *sharedListener, conn net.Conn) {
	defer conn.Close()

	hostname, routedConn, err := extractHostname(conn, sl.port)
	if err != nil {
		m.logger.V(1).Info("could not extract hostname from shared connection",
			"key", sl.key,
			"remote", conn.RemoteAddr().String(),
			"error", err)
		return
	}

	if !sl.table.IsManaged(hostname) {
		m.logger.V(1).Info("unmanaged hostname on shared listener",
			"key", sl.key,
			"hostname", hostname)
		return
	}

	m.logger.Info("→",
		"from", conn.RemoteAddr().String(),
		"to", fmt.Sprintf("%s:%d", hostname, sl.port),
		"via", sl.key)

	endpoint := forwarder.BoundEndpoint{
		Name:          sl.key,
		URI:           fmt.Sprintf("https://%s:%d", hostname, sl.port),
		Port:          sl.port,
		RequestedHost: hostname,
	}

	if err := m.forwarder.ForwardConnection(ctx, routedConn, endpoint); err != nil {
		m.logger.V(1).Info("shared connection closed with error",
			"key", sl.key,
			"hostname", hostname,
			"error", err)
	}
}

// extractHostname peeks at a connection to determine the target hostname.
// For TLS connections, extracts SNI from ClientHello.
// For HTTP connections, extracts the Host header.
// Returns the hostname and a wrapped connection that replays peeked bytes.
func extractHostname(conn net.Conn, port int) (string, net.Conn, error) {
	// Try TLS SNI first (covers HTTPS and other TLS protocols)
	serverName, wrapped, err := sni.PeekClientHelloConn(conn)
	if err == nil && serverName != "" {
		return serverName, wrapped, nil
	}

	// Not TLS — try to read HTTP Host header
	// wrapped already has peeked bytes buffered
	hostname, httpConn, httpErr := extractHTTPHost(wrapped)
	if httpErr == nil && hostname != "" {
		return hostname, httpConn, nil
	}

	return "", wrapped, fmt.Errorf("could not extract hostname: tls=%v http=%v", err, httpErr)
}

// extractHTTPHost peeks at a connection to extract the HTTP Host header.
func extractHTTPHost(conn net.Conn) (string, net.Conn, error) {
	br := bufio.NewReaderSize(conn, 8192)

	peek, err := br.Peek(4)
	if err != nil {
		wrapped := &readerConn{Reader: br, Conn: conn}
		return "", wrapped, err
	}

	if !looksLikeHTTP(peek) {
		wrapped := &readerConn{Reader: br, Conn: conn}
		return "", wrapped, fmt.Errorf("not HTTP")
	}

	req, err := http.ReadRequest(br)
	if err != nil {
		wrapped := &readerConn{Reader: br, Conn: conn}
		return "", wrapped, fmt.Errorf("failed to read HTTP request: %w", err)
	}

	host := req.Host
	if h, _, splitErr := net.SplitHostPort(host); splitErr == nil {
		host = h
	}

	// We consumed the request bytes — we need to replay them.
	// Reconstruct and prepend the request bytes.
	var buf strings.Builder
	req.Write(&buf)
	combined := &prefixConn{
		prefix: []byte(buf.String()),
		Conn:   conn,
	}

	return host, combined, nil
}

func looksLikeHTTP(b []byte) bool {
	methods := []string{"GET ", "POST", "PUT ", "DELE", "PATC", "HEAD", "OPTI", "CONN"}
	s := string(b)
	for _, m := range methods {
		if strings.HasPrefix(s, m) {
			return true
		}
	}
	return false
}

type readerConn struct {
	Reader *bufio.Reader
	net.Conn
}

func (r *readerConn) Read(p []byte) (int, error) {
	return r.Reader.Read(p)
}

// prefixConn replays prefix bytes before reading from the underlying conn.
type prefixConn struct {
	prefix []byte
	off    int
	net.Conn
}

func (c *prefixConn) Read(p []byte) (int, error) {
	if c.off < len(c.prefix) {
		n := copy(p, c.prefix[c.off:])
		c.off += n
		return n, nil
	}
	return c.Conn.Read(p)
}
