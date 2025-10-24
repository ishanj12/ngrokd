package listener

import (
	"context"
	"fmt"
	"net"
	"sync"

	"github.com/go-logr/logr"
	"github.com/ishanjain/ngrok-forward-proxy/pkg/forwarder"
)

// StatusCallback is called when connection events occur
type StatusCallback interface {
	RecordConnection(endpointName string)
	RecordConnectionClose(endpointName string)
	RecordError(endpointName string)
}

// Manager manages local TCP listeners for bound endpoints
type Manager struct {
	forwarder      *forwarder.Forwarder
	logger         logr.Logger
	statusCallback StatusCallback

	mu        sync.RWMutex
	listeners map[string]*activeListener // key: endpoint name
}

type activeListener struct {
	endpoint forwarder.BoundEndpoint
	listener net.Listener
	cancel   context.CancelFunc
}

// New creates a new listener Manager
func New(fwd *forwarder.Forwarder, logger logr.Logger) *Manager {
	return &Manager{
		forwarder:      fwd,
		logger:         logger,
		listeners:      make(map[string]*activeListener),
		statusCallback: nil,
	}
}

// SetStatusCallback sets the callback for status updates
func (m *Manager) SetStatusCallback(cb StatusCallback) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.statusCallback = cb
}

// StartListener creates and starts a local listener for the given bound endpoint
func (m *Manager) StartListener(ctx context.Context, endpoint forwarder.BoundEndpoint) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if already listening
	if _, exists := m.listeners[endpoint.Name]; exists {
		return fmt.Errorf("listener already exists for endpoint %s", endpoint.Name)
	}

	// Create local listener on specific IP:port
	localIP := endpoint.LocalAddress
	if localIP == "" {
		localIP = "127.0.0.1"
	}
	addr := fmt.Sprintf("%s:%d", localIP, endpoint.LocalPort)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to create listener on %s: %w", addr, err)
	}

	m.logger.Info("started local listener",
		"endpoint", endpoint.Name,
		"address", addr,
		"target", endpoint.URI)

	// Create cancellable context
	listenerCtx, cancel := context.WithCancel(ctx)

	active := &activeListener{
		endpoint: endpoint,
		listener: listener,
		cancel:   cancel,
	}

	m.listeners[endpoint.Name] = active

	// Start accepting connections in background
	go m.acceptConnections(listenerCtx, active)

	return nil
}

// StopListener stops the listener for the given endpoint
func (m *Manager) StopListener(endpointName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	active, exists := m.listeners[endpointName]
	if !exists {
		return fmt.Errorf("no listener found for endpoint %s", endpointName)
	}

	m.logger.Info("stopping listener", "endpoint", endpointName)

	// Cancel context and close listener
	active.cancel()
	active.listener.Close()

	delete(m.listeners, endpointName)

	return nil
}

// acceptConnections accepts and forwards connections in a loop
func (m *Manager) acceptConnections(ctx context.Context, active *activeListener) {
	m.logger.Info("accept loop started", "endpoint", active.endpoint.Name, "address", active.listener.Addr().String())
	
	for {
		// Check if context was cancelled before accepting
		if ctx.Err() != nil {
			m.logger.Info("listener context cancelled", "endpoint", active.endpoint.Name)
			return
		}

		m.logger.V(1).Info("waiting for connection", "endpoint", active.endpoint.Name)
		
		// Accept connection (this blocks until a connection arrives)
		conn, err := active.listener.Accept()
		if err != nil {
			// Check if context was cancelled or listener closed
			if ctx.Err() != nil {
				return
			}

			m.logger.Error(err, "failed to accept connection",
				"endpoint", active.endpoint.Name)
			continue
		}

		// Log HTTP requests nicely
		m.logger.Info("→",
			"from", conn.RemoteAddr().String(),
			"to", active.endpoint.URI)

		// Record connection
		if m.statusCallback != nil {
			m.statusCallback.RecordConnection(active.endpoint.Name)
		}

		// Forward connection in background
		go func(c net.Conn) {
			defer c.Close()
			defer func() {
				if m.statusCallback != nil {
					m.statusCallback.RecordConnectionClose(active.endpoint.Name)
				}
			}()

			if err := m.forwarder.ForwardConnection(c, active.endpoint); err != nil {
				m.logger.Error(err, "failed to forward connection",
					"endpoint", active.endpoint.Name)
				if m.statusCallback != nil {
					m.statusCallback.RecordError(active.endpoint.Name)
				}
			}
		}(conn)
	}
}

// ListActiveEndpoints returns a list of all active endpoint names
func (m *Manager) ListActiveEndpoints() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	endpoints := make([]string, 0, len(m.listeners))
	for name := range m.listeners {
		endpoints = append(endpoints, name)
	}

	return endpoints
}

// Close stops all listeners
func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.logger.Info("closing all listeners")

	for name, active := range m.listeners {
		active.cancel()
		active.listener.Close()
		delete(m.listeners, name)
	}

	return nil
}
