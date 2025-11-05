//go:build windows

package main

import (
	"net"
	
	"github.com/Microsoft/go-winio"
)

// dialSocket connects to a Windows named pipe
func dialSocket(socketPath string) (net.Conn, error) {
	return winio.DialPipe(socketPath, nil)
}
