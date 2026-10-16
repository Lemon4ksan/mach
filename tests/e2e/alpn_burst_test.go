// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package e2e_test

import (
	"context"
	"crypto/tls"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lemon4ksan/foundation/testing/assert"
	"github.com/lemon4ksan/mach/proto/semantic/status"

	h1client "github.com/lemon4ksan/mach/client/h1"
	h2client "github.com/lemon4ksan/mach/client/h2"
	machhttp "github.com/lemon4ksan/mach/proto/http"
	h1server "github.com/lemon4ksan/mach/server/h1"
	h2server "github.com/lemon4ksan/mach/server/h2"
)

// startALPNServer starts a TLS listener that dynamically dispatches to H1 or H2
// based on TLS ALPN negotiation.
func startALPNServer(t *testing.T) (string, *tls.Config, func()) {
	t.Helper()

	serverTLS, clientTLS := generateTestTLSConfig(t)
	serverTLS.NextProtos = []string{"h2", "http/1.1"}

	ln, err := tls.Listen("tcp", "127.0.0.1:0", serverTLS)
	if err != nil {
		t.Fatalf("startALPNServer: tls.Listen failed: %v", err)
	}

	var (
		connsMu sync.Mutex
		conns   []net.Conn
		closed  bool
	)

	h1Handler := h1server.ConnHandler{
		Handler: func(req *h1server.Request, res *h1server.Response) error {
			res.StatusCode = status.OK
			res.Headers.Set("Content-Type", "text/plain")
			res.Headers.Set("X-Protocol", "http/1.1")
			res.Body = append(res.Body[:0], "hello from h1"...)
			return nil
		},
	}

	h2Handler := func(req *h2server.ServerRequest, res *h2server.ServerResponse) error {
		res.StatusCode = status.OK
		res.Headers.Set("Content-Type", "text/plain")
		res.Headers.Set("X-Protocol", "h2")
		res.Body = []byte("hello from h2")
		return nil
	}

	go func() {
		for {
			rawConn, err := ln.Accept()
			if err != nil {
				return
			}

			connsMu.Lock()
			if closed {
				_ = rawConn.Close()
				connsMu.Unlock()
				return
			}
			conns = append(conns, rawConn)
			connsMu.Unlock()

			go func(c net.Conn) {
				tlsConn, ok := c.(*tls.Conn)
				if !ok {
					_ = c.Close()
					return
				}

				// Perform handshake with a short timeout to prevent hung handshakes
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()

				if err := tlsConn.HandshakeContext(ctx); err != nil {
					_ = tlsConn.Close()
					return
				}

				negotiated := tlsConn.ConnectionState().NegotiatedProtocol
				switch negotiated {
				case "h2":
					wrapped := &h2WindowUpdateConn{Conn: tlsConn}
					sc := h2server.NewServerConn(wrapped, h2Handler)
					_ = sc.Serve()
				case "http/1.1", "":
					_ = h1Handler.ServeConn(tlsConn)
				default:
					_ = tlsConn.Close()
				}
			}(rawConn)
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

	return ln.Addr().String(), clientTLS, cleanup
}

// TestALPN_HighConcurrencyMixedBurst fires a burst of mixed H1 and H2 TLS connections
// concurrently against the ALPN server.
func TestALPN_HighConcurrencyMixedBurst(t *testing.T) {
	t.Parallel()

	addr, clientTLSBase, cleanup := startALPNServer(t)
	defer cleanup()

	const concurrency = 60
	var (
		wg        sync.WaitGroup
		h1Success atomic.Int64
		h2Success atomic.Int64
	)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		workerID := i
		isH2 := (workerID % 2) == 0

		go func() {
			defer wg.Done()

			tlsConf := clientTLSBase.Clone()
			if isH2 {
				tlsConf.NextProtos = []string{"h2"}
			} else {
				tlsConf.NextProtos = []string{"http/1.1"}
			}

			dialer := &net.Dialer{Timeout: 3 * time.Second}
			rawConn, err := tls.DialWithDialer(dialer, "tcp", addr, tlsConf)
			if err != nil {
				t.Errorf("worker %d dial failed: %v", workerID, err)
				return
			}
			defer rawConn.Close()

			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()

			req := machhttp.AcquireRequest()
			resp := machhttp.AcquireResponse()
			defer machhttp.ReleaseRequest(req)
			defer machhttp.ReleaseResponse(resp)

			req.Header.SetMethod("GET")
			req.SetRequestURI("/mixed-alpn")
			req.Header.SetHost(addr)

			if isH2 {
				nc := h2client.NewConn(rawConn, h2client.ConnOpts{})
				if err := nc.Handshake(); err != nil {
					t.Errorf("worker %d h2 handshake failed: %v", workerID, err)
					return
				}
				if err := nc.Do(ctx, req, resp); err != nil {
					t.Errorf("worker %d h2 request failed: %v", workerID, err)
					return
				}
				if resp.StatusCode() == 200 && string(resp.Body()) == "hello from h2" {
					h2Success.Add(1)
				} else {
					t.Errorf("worker %d unexpected h2 response: status=%d body=%s", workerID, resp.StatusCode(), resp.Body())
				}
			} else {
				h1Conn := h1client.NewClientConn(rawConn)
				if err := h1Conn.Do(ctx, req, resp); err != nil {
					t.Errorf("worker %d h1 request failed: %v", workerID, err)
					return
				}
				if resp.StatusCode() == 200 && string(resp.Body()) == "hello from h1" {
					h1Success.Add(1)
				} else {
					t.Errorf("worker %d unexpected h1 response: status=%d body=%s", workerID, resp.StatusCode(), resp.Body())
				}
			}
		}()
	}

	wg.Wait()

	assert.Equal(t, int64(concurrency/2), h1Success.Load())
	assert.Equal(t, int64(concurrency/2), h2Success.Load())
}

// TestALPN_SimulatedConnectionDropsDuringHandshake tests server resilience when clients
// abruptly drop or reset connections during TLS handshake or immediately after.
func TestALPN_SimulatedConnectionDropsDuringHandshake(t *testing.T) {
	t.Parallel()

	addr, _, cleanup := startALPNServer(t)
	defer cleanup()

	const drops = 30
	var wg sync.WaitGroup

	for i := 0; i < drops; i++ {
		wg.Add(1)
		workerID := i
		go func() {
			defer wg.Done()

			conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
			if err != nil {
				return
			}

			switch workerID % 3 {
			case 0:
				// Close immediately after TCP connect without sending anything
				_ = conn.Close()
			case 1:
				// Send garbage / truncated ClientHello and close
				_, _ = conn.Write([]byte{0x16, 0x03, 0x01, 0x00, 0x05, 0x01})
				_ = conn.Close()
			case 2:
				// Send partial bytes, wait briefly, then close
				_, _ = conn.Write([]byte{0x00, 0x01, 0x02})
				time.Sleep(10 * time.Millisecond)
				_ = conn.Close()
			}
		}()
	}

	wg.Wait()
}

// TestALPN_SimulatedConnectionDropsAfterHandshake tests server resilience when clients
// complete the TLS handshake with ALPN negotiation, and then immediately disconnect
// without issuing any HTTP requests.
func TestALPN_SimulatedConnectionDropsAfterHandshake(t *testing.T) {
	t.Parallel()

	addr, clientTLSBase, cleanup := startALPNServer(t)
	defer cleanup()

	const count = 20
	var wg sync.WaitGroup

	for i := 0; i < count; i++ {
		wg.Add(1)
		isH2 := (i % 2) == 0
		go func() {
			defer wg.Done()

			tlsConf := clientTLSBase.Clone()
			if isH2 {
				tlsConf.NextProtos = []string{"h2"}
			} else {
				tlsConf.NextProtos = []string{"http/1.1"}
			}

			rawConn, err := tls.Dial("tcp", addr, tlsConf)
			if err != nil {
				return
			}
			// Immediately abort the connection without sending HTTP frames or requests
			_ = rawConn.Close()
		}()
	}

	wg.Wait()
}
