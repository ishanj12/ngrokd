package health

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/go-logr/logr"
)

// Status represents the agent's operational status
type Status struct {
	Healthy   bool                     `json:"healthy"`
	Ready     bool                     `json:"ready"`
	Uptime    string                   `json:"uptime"`
	Endpoints map[string]EndpointStatus `json:"endpoints"`
	StartTime time.Time                `json:"start_time"`
}

// EndpointStatus represents the status of a single endpoint
type EndpointStatus struct {
	Name            string    `json:"name"`
	LocalAddress    string    `json:"local_address"`
	TargetURI       string    `json:"target_uri"`
	Active          bool      `json:"active"`
	Connections     int64     `json:"connections"`
	TotalConnections int64    `json:"total_connections"`
	LastActivity    time.Time `json:"last_activity,omitempty"`
	Errors          int64     `json:"errors"`
}

// Server provides health check and status endpoints
type Server struct {
	addr      string
	server    *http.Server
	logger    logr.Logger
	startTime time.Time

	mu        sync.RWMutex
	endpoints map[string]*EndpointStatus
	ready     bool
}

// Config holds the health server configuration
type Config struct {
	Address string
	Port    int
	Logger  logr.Logger
}

// NewServer creates a new health check server
func NewServer(config Config) *Server {
	if config.Port == 0 {
		config.Port = 8081
	}

	addr := fmt.Sprintf("%s:%d", config.Address, config.Port)
	if config.Address == "" {
		addr = fmt.Sprintf(":%d", config.Port)
	}

	s := &Server{
		addr:      addr,
		logger:    config.Logger,
		startTime: time.Now(),
		endpoints: make(map[string]*EndpointStatus),
		ready:     false,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/healthz", s.handleHealth)
	mux.HandleFunc("/ready", s.handleReady)
	mux.HandleFunc("/readyz", s.handleReady)
	mux.HandleFunc("/status", s.handleStatus)

	s.server = &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	return s
}

// Start starts the health check server
func (s *Server) Start() error {
	s.logger.Info("Starting health check server", "address", s.addr)

	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Error(err, "Health server error")
		}
	}()

	return nil
}

// Stop stops the health check server
func (s *Server) Stop(ctx context.Context) error {
	s.logger.Info("Stopping health check server")
	return s.server.Shutdown(ctx)
}

// SetReady marks the agent as ready
func (s *Server) SetReady(ready bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ready = ready
}

// RegisterEndpoint registers an endpoint for status tracking
func (s *Server) RegisterEndpoint(name, localAddr, targetURI string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.endpoints[name] = &EndpointStatus{
		Name:         name,
		LocalAddress: localAddr,
		TargetURI:    targetURI,
		Active:       true,
	}
}

// UnregisterEndpoint removes an endpoint from tracking
func (s *Server) UnregisterEndpoint(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.endpoints, name)
}

// RecordConnection records a new connection for an endpoint
func (s *Server) RecordConnection(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if ep, exists := s.endpoints[name]; exists {
		ep.Connections++
		ep.TotalConnections++
		ep.LastActivity = time.Now()
	}
}

// RecordConnectionClose records a connection closure
func (s *Server) RecordConnectionClose(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if ep, exists := s.endpoints[name]; exists {
		ep.Connections--
	}
}

// RecordError records an error for an endpoint
func (s *Server) RecordError(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if ep, exists := s.endpoints[name]; exists {
		ep.Errors++
	}
}

// handleHealth handles /health and /healthz requests
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Agent is healthy if at least one active endpoint exists
	healthy := false
	for _, ep := range s.endpoints {
		if ep.Active {
			healthy = true
			break
		}
	}

	if healthy {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "healthy\n")
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprintf(w, "unhealthy\n")
	}
}

// handleReady handles /ready and /readyz requests
func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	ready := s.ready
	s.mu.RUnlock()

	if ready {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "ready\n")
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprintf(w, "not ready\n")
	}
}

// handleStatus handles /status requests
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	status := Status{
		Healthy:   len(s.endpoints) > 0,
		Ready:     s.ready,
		Uptime:    time.Since(s.startTime).String(),
		StartTime: s.startTime,
		Endpoints: make(map[string]EndpointStatus),
	}

	for name, ep := range s.endpoints {
		status.Endpoints[name] = *ep
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}
