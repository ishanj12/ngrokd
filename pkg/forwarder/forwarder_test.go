package forwarder

import (
	"io"
	"net"
	"testing"
	"time"
)

func TestExtractHost(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"http://my-app.ngrok.app:81", "my-app.ngrok.app"},
		{"https://api.example.com", "api.example.com"},
		{"https://api.example.com:443", "api.example.com"},
		{"tcp://db.internal:5432", "db.internal"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := extractHost(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("extractHost(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestRawProxy(t *testing.T) {
	t.Run("local_to_remote", func(t *testing.T) {
		local1, local2 := net.Pipe()
		remote1, remote2 := net.Pipe()

		done := make(chan error, 1)
		go func() {
			done <- rawProxy(local1, remote1)
		}()

		msg := []byte("hello from local")
		go func() {
			local2.Write(msg)
			local2.Close()
		}()

		buf := make([]byte, 256)
		n, err := remote2.Read(buf)
		if err != nil && err != io.EOF {
			t.Fatalf("read from remote: %v", err)
		}
		if string(buf[:n]) != "hello from local" {
			t.Fatalf("got %q, want %q", string(buf[:n]), "hello from local")
		}

		remote2.Close()

		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("rawProxy did not return in time")
		}
	})

	t.Run("remote_to_local", func(t *testing.T) {
		local1, local2 := net.Pipe()
		remote1, remote2 := net.Pipe()

		done := make(chan error, 1)
		go func() {
			done <- rawProxy(local1, remote1)
		}()

		reply := []byte("hello from remote")
		go func() {
			remote2.Write(reply)
			remote2.Close()
		}()

		buf := make([]byte, 256)
		n, err := local2.Read(buf)
		if err != nil && err != io.EOF {
			t.Fatalf("read from local: %v", err)
		}
		if string(buf[:n]) != "hello from remote" {
			t.Fatalf("got %q, want %q", string(buf[:n]), "hello from remote")
		}

		local2.Close()

		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("rawProxy did not return in time")
		}
	})
}
