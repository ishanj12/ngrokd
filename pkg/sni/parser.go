package sni

import (
	"bufio"
	"encoding/binary"
	"errors"
	"io"
	"net"
)

var (
	ErrNotTLS        = errors.New("not a TLS handshake")
	ErrNotClientHello = errors.New("not a ClientHello message")
	ErrNoSNI         = errors.New("no SNI extension found")
)

// PeekClientHelloConn peeks at a connection to extract the TLS SNI server name.
// It returns the server name and a wrapped connection that replays the peeked bytes.
// If the connection is not TLS or has no SNI, it returns an error and a wrapped
// connection that can still be read normally.
func PeekClientHelloConn(conn net.Conn) (serverName string, wrapped net.Conn, err error) {
	br := bufio.NewReaderSize(conn, 16384)

	// Peek at TLS record header (5 bytes): content_type(1) + version(2) + length(2)
	hdr, err := br.Peek(5)
	if err != nil {
		return "", &bufferedConn{Reader: br, Conn: conn}, ErrNotTLS
	}

	// content_type 22 = Handshake
	if hdr[0] != 22 {
		return "", &bufferedConn{Reader: br, Conn: conn}, ErrNotTLS
	}

	recordLen := int(binary.BigEndian.Uint16(hdr[3:5]))
	if recordLen < 4 || recordLen > 16384 {
		return "", &bufferedConn{Reader: br, Conn: conn}, ErrNotTLS
	}

	// Peek the full record: 5-byte header + payload
	total := 5 + recordLen
	record, err := br.Peek(total)
	if err != nil {
		return "", &bufferedConn{Reader: br, Conn: conn}, ErrNotTLS
	}

	payload := record[5:]

	// Handshake message type: 1 = ClientHello
	if payload[0] != 1 {
		return "", &bufferedConn{Reader: br, Conn: conn}, ErrNotClientHello
	}

	sni, err := parseClientHello(payload[4:]) // skip type(1) + length(3)
	wrapped = &bufferedConn{Reader: br, Conn: conn}
	return sni, wrapped, err
}

// parseClientHello extracts the SNI from a ClientHello message body.
func parseClientHello(data []byte) (string, error) {
	if len(data) < 38 {
		return "", ErrNoSNI
	}

	// Skip: client_version(2) + random(32)
	pos := 34

	// Session ID (variable length)
	if pos >= len(data) {
		return "", ErrNoSNI
	}
	sessionIDLen := int(data[pos])
	pos += 1 + sessionIDLen

	// Cipher suites (variable length, 2-byte length prefix)
	if pos+2 > len(data) {
		return "", ErrNoSNI
	}
	cipherSuitesLen := int(binary.BigEndian.Uint16(data[pos : pos+2]))
	pos += 2 + cipherSuitesLen

	// Compression methods (variable length, 1-byte length prefix)
	if pos >= len(data) {
		return "", ErrNoSNI
	}
	compMethodsLen := int(data[pos])
	pos += 1 + compMethodsLen

	// Extensions (2-byte total length prefix)
	if pos+2 > len(data) {
		return "", ErrNoSNI
	}
	extensionsLen := int(binary.BigEndian.Uint16(data[pos : pos+2]))
	pos += 2

	end := pos + extensionsLen
	if end > len(data) {
		end = len(data)
	}

	// Walk extensions looking for SNI (type 0x0000)
	for pos+4 <= end {
		extType := binary.BigEndian.Uint16(data[pos : pos+2])
		extLen := int(binary.BigEndian.Uint16(data[pos+2 : pos+4]))
		pos += 4

		if pos+extLen > end {
			break
		}

		if extType == 0 {
			return parseSNIExtension(data[pos : pos+extLen])
		}

		pos += extLen
	}

	return "", ErrNoSNI
}

// parseSNIExtension parses the SNI extension data.
func parseSNIExtension(data []byte) (string, error) {
	if len(data) < 2 {
		return "", ErrNoSNI
	}

	// Server name list length (2 bytes)
	listLen := int(binary.BigEndian.Uint16(data[0:2]))
	if listLen+2 > len(data) {
		return "", ErrNoSNI
	}

	pos := 2
	listEnd := 2 + listLen

	for pos+3 <= listEnd {
		nameType := data[pos]
		nameLen := int(binary.BigEndian.Uint16(data[pos+1 : pos+3]))
		pos += 3

		if pos+nameLen > listEnd {
			break
		}

		// name_type 0 = host_name
		if nameType == 0 {
			return string(data[pos : pos+nameLen]), nil
		}

		pos += nameLen
	}

	return "", ErrNoSNI
}

// bufferedConn wraps a net.Conn with a bufio.Reader so that peeked bytes
// are replayed on subsequent reads.
type bufferedConn struct {
	io.Reader
	net.Conn
}

func (c *bufferedConn) Read(p []byte) (int, error) {
	return c.Reader.Read(p)
}
