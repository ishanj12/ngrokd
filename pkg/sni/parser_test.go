package sni

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"testing"
	"time"
)

func selfSignedCert(t *testing.T) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(time.Hour),
		DNSNames:     []string{"localhost"},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyDER, _ := x509.MarshalECPrivateKey(key)
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	return cert
}

func TestPeekClientHelloConn_ExtractsSNI(t *testing.T) {
	cert := selfSignedCert(t)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	sniCh := make(chan string, 1)
	errCh := make(chan error, 1)

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			errCh <- err
			return
		}
		defer conn.Close()

		serverName, wrapped, err := PeekClientHelloConn(conn)
		if err != nil {
			errCh <- err
			return
		}
		sniCh <- serverName

		// Complete TLS handshake using the wrapped conn to prove bytes are replayed
		tlsConn := tls.Server(wrapped, &tls.Config{
			Certificates: []tls.Certificate{cert},
		})
		if err := tlsConn.Handshake(); err != nil {
			errCh <- err
			return
		}
		tlsConn.Close()
	}()

	// Client: connect with specific ServerName
	clientConn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer clientConn.Close()

	tlsClient := tls.Client(clientConn, &tls.Config{
		ServerName:         "myapp.example.com",
		InsecureSkipVerify: true,
	})
	if err := tlsClient.Handshake(); err != nil {
		t.Fatalf("client handshake failed: %v", err)
	}
	tlsClient.Close()

	select {
	case sni := <-sniCh:
		if sni != "myapp.example.com" {
			t.Fatalf("expected SNI 'myapp.example.com', got %q", sni)
		}
	case err := <-errCh:
		t.Fatalf("server error: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}
}

func TestPeekClientHelloConn_DifferentSNI(t *testing.T) {
	cert := selfSignedCert(t)

	for _, expected := range []string{
		"api.company.io",
		"db.internal.cluster.local",
		"a.b.c.d.e.f.example.com",
	} {
		t.Run(expected, func(t *testing.T) {
			ln, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer ln.Close()

			sniCh := make(chan string, 1)
			errCh := make(chan error, 1)

			go func() {
				conn, err := ln.Accept()
				if err != nil {
					errCh <- err
					return
				}
				defer conn.Close()

				sni, wrapped, err := PeekClientHelloConn(conn)
				if err != nil {
					errCh <- err
					return
				}
				sniCh <- sni

				tlsConn := tls.Server(wrapped, &tls.Config{Certificates: []tls.Certificate{cert}})
				tlsConn.Handshake()
				tlsConn.Close()
			}()

			clientConn, err := net.Dial("tcp", ln.Addr().String())
			if err != nil {
				t.Fatal(err)
			}
			defer clientConn.Close()

			tlsClient := tls.Client(clientConn, &tls.Config{
				ServerName:         expected,
				InsecureSkipVerify: true,
			})
			tlsClient.Handshake()
			tlsClient.Close()

			select {
			case sni := <-sniCh:
				if sni != expected {
					t.Fatalf("expected SNI %q, got %q", expected, sni)
				}
			case err := <-errCh:
				t.Fatalf("error: %v", err)
			case <-time.After(5 * time.Second):
				t.Fatal("timeout")
			}
		})
	}
}

func TestPeekClientHelloConn_NotTLS(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	go func() {
		client.Write([]byte("GET / HTTP/1.1\r\nHost: example.com\r\n\r\n"))
	}()

	_, wrapped, err := PeekClientHelloConn(server)
	if err != ErrNotTLS {
		t.Fatalf("expected ErrNotTLS, got %v", err)
	}

	// Verify wrapped conn still has the data
	buf := make([]byte, 4)
	n, _ := wrapped.Read(buf)
	if string(buf[:n]) != "GET " {
		t.Fatalf("expected 'GET ', got %q", string(buf[:n]))
	}
}

func TestPeekClientHelloConn_WrappedConnReplaysBytes(t *testing.T) {
	cert := selfSignedCert(t)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	dataCh := make(chan string, 1)
	errCh := make(chan error, 1)

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			errCh <- err
			return
		}
		defer conn.Close()

		_, wrapped, err := PeekClientHelloConn(conn)
		if err != nil {
			errCh <- err
			return
		}

		// Use wrapped conn for TLS
		tlsConn := tls.Server(wrapped, &tls.Config{Certificates: []tls.Certificate{cert}})
		if err := tlsConn.Handshake(); err != nil {
			errCh <- err
			return
		}

		// Read application data
		buf := make([]byte, 1024)
		n, err := tlsConn.Read(buf)
		if err != nil && err != io.EOF {
			errCh <- err
			return
		}
		dataCh <- string(buf[:n])
		tlsConn.Close()
	}()

	clientConn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer clientConn.Close()

	tlsClient := tls.Client(clientConn, &tls.Config{
		ServerName:         "test.example.com",
		InsecureSkipVerify: true,
	})
	if err := tlsClient.Handshake(); err != nil {
		t.Fatalf("handshake: %v", err)
	}
	tlsClient.Write([]byte("hello world"))
	tlsClient.Close()

	select {
	case data := <-dataCh:
		if data != "hello world" {
			t.Fatalf("expected 'hello world', got %q", data)
		}
	case err := <-errCh:
		t.Fatalf("error: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}
}
