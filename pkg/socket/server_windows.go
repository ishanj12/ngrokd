//go:build windows

package socket

import (
	"fmt"
	"net"
	
	"github.com/Microsoft/go-winio"
)

// createListener creates a Windows named pipe listener
func (s *Server) createListener() (net.Listener, error) {
	// Windows named pipe path: \\.\pipe\ngrokd
	listener, err := winio.ListenPipe(s.socketPath, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create named pipe: %w", err)
	}
	
	s.logger.Info("Named pipe listener created", "path", s.socketPath)
	
	return listener, nil
}
