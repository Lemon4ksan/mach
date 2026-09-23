// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package e2e_test

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/lemon4ksan/foundation/net/quic"
	h1client "github.com/lemon4ksan/mach/client/h1"
	h2client "github.com/lemon4ksan/mach/client/h2"
	h3client "github.com/lemon4ksan/mach/client/h3"
	coreh2 "github.com/lemon4ksan/mach/proto/h2"
	h1server "github.com/lemon4ksan/mach/server/h1"
	h2server "github.com/lemon4ksan/mach/server/h2"
	h3server "github.com/lemon4ksan/mach/server/h3"
)

// generateTestTLSConfig generates in-memory self-signed ECDSA certificates for secure test listeners.
func generateTestTLSConfig(t *testing.T) (*tls.Config, *tls.Config) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate ecdsa key: %v", err)
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"mach-e2e-test"},
		},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("failed to create certificate: %v", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	keyBytes, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("failed to marshal ec private key: %v", err)
	}

	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes})

	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatalf("failed to parse x509 key pair: %v", err)
	}

	serverTLS := &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
		NextProtos:   []string{"h3", "h2", "http/1.1"},
	}

	clientTLS := &tls.Config{
		InsecureSkipVerify: true, //nolint:gosec // in-memory test certificates
		NextProtos:         []string{"h3", "h2", "http/1.1"},
	}

	return serverTLS, clientTLS
}

// startH1Server starts a loopback TCP listener and runs server/h1.ConnHandler on accepted connections.
func startH1Server(t *testing.T, handler h1server.HandlerFunc) (string, func()) {
	t.Helper()

	ch := h1server.ConnHandler{
		Handler: handler,
	}

	return startH1ServerWithOpts(t, ch)
}

// startH1ServerWithOpts starts a loopback TCP listener with custom h1server.ConnHandler configuration.
func startH1ServerWithOpts(t *testing.T, ch h1server.ConnHandler) (string, func()) {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("startH1Server: listen failed: %v", err)
	}

	var (
		connsMu sync.Mutex
		conns   []net.Conn
		closed  bool
	)

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}

			connsMu.Lock()
			if closed {
				_ = conn.Close()
				connsMu.Unlock()
				return
			}
			conns = append(conns, conn)
			connsMu.Unlock()

			go func(c net.Conn) {
				_ = ch.ServeConn(c)
			}(conn)
		}
	}()

	cleanup := func() {
		connsMu.Lock()
		closed = true
		_ = ln.Close()
		for _, c := range conns {
			_ = c.Close()
		}
		connsMu.Unlock()
	}

	return ln.Addr().String(), cleanup
}

// dialH1Client connects to an H1 server and returns an opaque *h1client.ClientConn and raw net.Conn.
func dialH1Client(t *testing.T, addr string) (*h1client.ClientConn, net.Conn) {
	t.Helper()

	c, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		t.Fatalf("dialH1Client: dial failed: %v", err)
	}

	return h1client.NewClientConn(c), c
}

// startH2Server starts a loopback TCP listener and runs server/h2.ServerConn on accepted connections.
func startH2Server(t *testing.T, handler h2server.ServerHandlerFunc) (string, func()) {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("startH2Server: listen failed: %v", err)
	}

	var (
		connsMu sync.Mutex
		conns   []net.Conn
		closed  bool
	)

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}

			connsMu.Lock()
			if closed {
				_ = conn.Close()
				connsMu.Unlock()
				return
			}
			conns = append(conns, conn)
			connsMu.Unlock()

			go func(c net.Conn) {
				wrappedConn := &h2WindowUpdateConn{Conn: c}
				sc := h2server.NewServerConn(wrappedConn, handler)
				_ = sc.Serve()
			}(conn)
		}
	}()

	cleanup := func() {
		connsMu.Lock()
		closed = true
		_ = ln.Close()
		for _, c := range conns {
			_ = c.Close()
		}
		connsMu.Unlock()
	}

	return ln.Addr().String(), cleanup
}

// dialH2Client connects to an H2 server, performs the initial handshake, and returns an opaque *h2client.Conn.
func dialH2Client(t *testing.T, addr string, opts h2client.ConnOpts) (*h2client.Conn, net.Conn) {
	t.Helper()

	c, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		t.Fatalf("dialH2Client: dial failed: %v", err)
	}

	nc := h2client.NewConn(c, opts)
	if err := nc.Handshake(); err != nil {
		_ = c.Close()
		t.Fatalf("dialH2Client: handshake failed: %v", err)
	}

	return nc, c
}

// startH3Server starts a loopback QUIC listener and runs server/h3.ServerConn on accepted sessions.
func startH3Server(t *testing.T, handler h3server.ServerHandlerFunc) (string, *tls.Config, func()) {
	t.Helper()

	serverTLS, clientTLS := generateTestTLSConfig(t)

	listener, err := quic.ListenAddr("127.0.0.1:0", serverTLS, quic.WithDatagrams(true))
	if err != nil {
		t.Fatalf("startH3Server: quic.ListenAddr failed: %v", err)
	}

	var (
		sessionsMu sync.Mutex
		sessions   []*quic.Conn
		closed     bool
	)

	go func() {
		for {
			conn, err := listener.Accept(context.Background())
			if err != nil {
				return
			}

			sessionsMu.Lock()
			if closed {
				_ = conn.CloseWithError(0, "")
				sessionsMu.Unlock()
				return
			}
			sessions = append(sessions, conn)
			sessionsMu.Unlock()

			sc := h3server.NewServerConn(conn, handler)
			go func() {
				_ = sc.Serve()
			}()
		}
	}()

	cleanup := func() {
		sessionsMu.Lock()
		closed = true
		_ = listener.Close()
		for _, s := range sessions {
			_ = s.CloseWithError(0, "")
		}
		sessionsMu.Unlock()
	}

	return listener.Addr().String(), clientTLS, cleanup
}

// dialH3Client dials a loopback QUIC session and initializes client/h3.ClientConn with control streams.
func dialH3Client(t *testing.T, addr string, clientTLS *tls.Config) (*h3client.ClientConn, *quic.Conn) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	quicConn, err := quic.DialAddr(ctx, addr, clientTLS, quic.WithDatagrams(true))
	if err != nil {
		t.Fatalf("dialH3Client: quic.DialAddr failed: %v", err)
	}

	cc, err := h3client.NewClientConn(quicConn, nil)
	if err != nil {
		_ = quicConn.CloseWithError(0, "")
		t.Fatalf("dialH3Client: NewClientConn failed: %v", err)
	}

	return cc, quicConn
}

// h2WindowUpdateConn wraps a server-side net.Conn to emit a connection-level WINDOW_UPDATE frame
// on stream 0 immediately after the server's initial SETTINGS frame.
// This establishes the client's connection flow-control window per RFC 9113 §6.9.
type h2WindowUpdateConn struct {
	net.Conn
	once sync.Once
}

func (w *h2WindowUpdateConn) Write(b []byte) (int, error) {
	var isHandshake bool
	w.once.Do(func() {
		isHandshake = true
		bw := bufio.NewWriter(w.Conn)

		// 1. Send initial server SETTINGS with 16MB stream window (RFC 9113 §6.5.2)
		st := &coreh2.Settings{}
		st.SetMaxConcurrentStreams(1000)
		st.SetMaxFrameSize(coreh2.DefaultMaxLen)
		st.SetMaxWindowSize(16 << 20)
		stFr := coreh2.AcquireFrameHeader()
		stFr.SetBody(st)
		_, _ = stFr.WriteTo(bw)
		coreh2.ReleaseFrameHeader(stFr)

		// 2. Send initial connection-level WINDOW_UPDATE on stream 0 (RFC 9113 §6.9)
		wuFr := coreh2.AcquireFrameHeader()
		wuFr.SetStream(0)
		wu := coreh2.AcquireFrame(coreh2.FrameWindowUpdate).(*coreh2.WindowUpdate)
		wu.SetIncrement(16 << 20)
		wuFr.SetBody(wu)
		_, _ = wuFr.WriteTo(bw)
		_ = bw.Flush()
		coreh2.ReleaseFrameHeader(wuFr)
	})

	if isHandshake {
		return len(b), nil
	}

	return w.Conn.Write(b)
}

