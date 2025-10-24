package forwarder

import (
	"bufio"
	"io"
	"net"
	"net/http"
)

// rewriteHTTPHost rewrites the Host header in an HTTP request
func rewriteHTTPHost(localConn, ngrokConn net.Conn, targetHost string) error {
	// Read the HTTP request
	reader := bufio.NewReader(localConn)
	req, err := http.ReadRequest(reader)
	if err != nil {
		// Not HTTP or malformed - fall back to raw proxy
		return rawProxy(localConn, ngrokConn)
	}

	// Rewrite Host header
	req.Host = targetHost
	req.Header.Set("Host", targetHost)

	// Write modified request to ngrok
	if err := req.Write(ngrokConn); err != nil {
		return err
	}

	// Now do bidirectional copy for the rest
	errChan := make(chan error, 2)

	// Copy responses: ngrok → local
	go func() {
		_, err := io.Copy(localConn, ngrokConn)
		errChan <- err
	}()

	// Copy remaining request data (if any): local → ngrok
	go func() {
		_, err := io.Copy(ngrokConn, reader)
		errChan <- err
	}()

	return <-errChan
}

// rawProxy does simple bidirectional copy (for non-HTTP or when HTTP parsing fails)
func rawProxy(localConn, ngrokConn net.Conn) error {
	errChan := make(chan error, 2)

	go func() {
		_, err := io.Copy(ngrokConn, localConn)
		errChan <- err
	}()

	go func() {
		_, err := io.Copy(localConn, ngrokConn)
		errChan <- err
	}()

	return <-errChan
}
