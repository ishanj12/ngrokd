//go:build !windows

package main

import "net"

// dialSocket connects to a Unix domain socket
func dialSocket(socketPath string) (net.Conn, error) {
	return net.Dial("unix", socketPath)
}
