//go:build !windows

package socket

import (
	"fmt"
	"net"
	"os"
)

// createListener creates a Unix domain socket listener
func (s *Server) createListener() (net.Listener, error) {
	// Remove old socket if exists
	os.Remove(s.socketPath)
	
	listener, err := net.Listen("unix", s.socketPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create unix socket: %w", err)
	}
	
	// Set permissions (readable/writable by all for easier CLI access)
	if err := os.Chmod(s.socketPath, 0666); err != nil {
		s.logger.Error(err, "Failed to set socket permissions")
	}
	
	return listener, nil
}
