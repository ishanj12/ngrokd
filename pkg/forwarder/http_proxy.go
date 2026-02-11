package forwarder

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
)

// hostRewritingProxy rewrites the Host header on every HTTP request flowing
// from localConn to ngrokConn, then copies the response bytes back raw.
// For non-HTTP traffic it falls back to a raw bidirectional copy.
func hostRewritingProxy(localConn, ngrokConn net.Conn, targetHost string, targetPort int) error {
	br := bufio.NewReader(localConn)

	peek, err := br.Peek(4)
	if err != nil || !looksLikeHTTP(peek) {
		combined := &readerConn{Reader: br, Conn: localConn}
		return rawProxy(combined, ngrokConn)
	}

	hostVal := targetHost
	if targetPort != 80 && targetPort != 443 {
		hostVal = fmt.Sprintf("%s:%d", targetHost, targetPort)
	}

	errChan := make(chan error, 2)

	// localConn → ngrokConn: parse each HTTP request, rewrite Host, write
	go func() {
		bw := bufio.NewWriter(ngrokConn)
		for {
			req, err := http.ReadRequest(br)
			if err != nil {
				errChan <- err
				return
			}
			req.Host = hostVal
			req.Header.Set("Host", hostVal)
			err = req.Write(bw)
			req.Body.Close()
			if err != nil {
				errChan <- err
				return
			}
			if err := bw.Flush(); err != nil {
				errChan <- err
				return
			}
		}
	}()

	// ngrokConn → localConn: raw copy (responses don't need rewriting)
	go func() {
		_, err := io.Copy(localConn, ngrokConn)
		errChan <- err
	}()

	return <-errChan
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
	io.Reader
	net.Conn
}

func (r *readerConn) Read(p []byte) (int, error) {
	return r.Reader.Read(p)
}

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
