package socket

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"

	"github.com/go-logr/logr"
)

// DaemonController interface for daemon operations
type DaemonController interface {
	GetStatus() StatusResponse
	ListEndpoints() []EndpointInfo
	SetAPIKey(key string) error
}

// Command represents a command from the ngrok client
type Command struct {
	Command string   `json:"command"`
	Args    []string `json:"args,omitempty"`
}

// Response represents a response to the client
type Response struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

// StatusResponse contains daemon status information
type StatusResponse struct {
	Registered     bool   `json:"registered"`
	OperatorID     string `json:"operator_id,omitempty"`
	EndpointCount  int    `json:"endpoint_count"`
	IngressEndpoint string `json:"ingress_endpoint"`
}

// EndpointInfo contains bound endpoint information
type EndpointInfo struct {
	ID              string `json:"id"`
	Hostname        string `json:"hostname"`
	IP              string `json:"ip"`
	Port            int    `json:"port"`
	URL             string `json:"url"`
	LocalListener   bool   `json:"local_listener"`    // True if listener is active
	NetworkPort     int    `json:"network_port"`      // Network port if not virtual
	ListenInterface string `json:"listen_interface"`  // "virtual", "0.0.0.0", or specific IP
}

// Server handles unix socket communication
type Server struct {
	socketPath string
	daemon     DaemonController
	listener   net.Listener
	logger     logr.Logger
}

// NewServer creates a new unix socket server
func NewServer(socketPath string, daemon DaemonController, logger logr.Logger) *Server {
	return &Server{
		socketPath: socketPath,
		daemon:     daemon,
		logger:     logger,
	}
}

// Start starts the unix socket server
func (s *Server) Start() error {
	// Remove old socket if exists
	os.Remove(s.socketPath)
	
	listener, err := net.Listen("unix", s.socketPath)
	if err != nil {
		return fmt.Errorf("failed to create unix socket: %w", err)
	}
	
	s.listener = listener
	
	// Set permissions (readable/writable by all for easier CLI access)
	if err := os.Chmod(s.socketPath, 0666); err != nil {
		s.logger.Error(err, "Failed to set socket permissions")
	}
	
	s.logger.Info("Unix socket server started", "path", s.socketPath)
	
	go s.acceptLoop()
	return nil
}

// Stop stops the unix socket server
func (s *Server) Stop() error {
	if s.listener != nil {
		s.listener.Close()
	}
	os.Remove(s.socketPath)
	return nil
}

func (s *Server) acceptLoop() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			// Server stopped
			return
		}
		go s.handleConnection(conn)
	}
}

func (s *Server) handleConnection(conn net.Conn) {
	defer conn.Close()
	
	// Read command
	reader := bufio.NewReader(conn)
	var cmd Command
	if err := json.NewDecoder(reader).Decode(&cmd); err != nil {
		s.sendError(conn, fmt.Sprintf("failed to decode command: %v", err))
		return
	}
	
	s.logger.V(1).Info("Received command", "command", cmd.Command, "args", cmd.Args)
	
	// Execute command
	resp := s.executeCommand(cmd)
	
	// Send response
	if err := json.NewEncoder(conn).Encode(resp); err != nil {
		s.logger.Error(err, "Failed to send response")
	}
}

func (s *Server) executeCommand(cmd Command) Response {
	switch cmd.Command {
	case "status":
		return Response{Success: true, Data: s.daemon.GetStatus()}
		
	case "list":
		return Response{Success: true, Data: s.daemon.ListEndpoints()}
		
	case "set-api-key":
		if len(cmd.Args) == 0 {
			return Response{Success: false, Error: "API key required"}
		}
		err := s.daemon.SetAPIKey(cmd.Args[0])
		if err != nil {
			return Response{Success: false, Error: err.Error()}
		}
		return Response{Success: true, Data: "API key set successfully"}
		
	default:
		return Response{Success: false, Error: fmt.Sprintf("unknown command: %s", cmd.Command)}
	}
}

func (s *Server) sendError(conn net.Conn, msg string) {
	json.NewEncoder(conn).Encode(Response{Success: false, Error: msg})
}
