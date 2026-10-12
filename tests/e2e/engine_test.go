// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package e2e_test

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lemon4ksan/foundation/testing/assert"
	"github.com/lemon4ksan/foundation/testing/require"
	"github.com/lemon4ksan/mach"
	"github.com/lemon4ksan/mach/client"
	machhttp "github.com/lemon4ksan/mach/proto/http"
	"github.com/lemon4ksan/mach/proto/semantic/status"
	h1server "github.com/lemon4ksan/mach/server/h1"
	h2server "github.com/lemon4ksan/mach/server/h2"
	h3server "github.com/lemon4ksan/mach/server/h3"
)

func startTLSH1Server(t *testing.T, serverTLS *tls.Config, handler h1server.HandlerFunc) (string, func()) {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	tlsLn := tls.NewListener(ln, serverTLS)
	ch := h1server.ConnHandler{Handler: handler}

	var (
		connsMu sync.Mutex
		conns   []net.Conn
		closed  bool
	)

	go func() {
		for {
			conn, err := tlsLn.Accept()
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
		_ = tlsLn.Close()
		for _, c := range conns {
			_ = c.Close()
		}
		connsMu.Unlock()
	}

	return tlsLn.Addr().String(), cleanup
}

func startTLSH2Server(t *testing.T, serverTLS *tls.Config, handler h2server.ServerHandlerFunc) (string, func()) {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	tlsLn := tls.NewListener(ln, serverTLS)

	var (
		connsMu sync.Mutex
		conns   []net.Conn
		closed  bool
	)

	go func() {
		for {
			conn, err := tlsLn.Accept()
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
		_ = tlsLn.Close()
		for _, c := range conns {
			_ = c.Close()
		}
		connsMu.Unlock()
	}

	return tlsLn.Addr().String(), cleanup
}

// ============================================================================
// Automatic ALPN Negotiation Tests
// ============================================================================

func TestEngine_E2E_ALPN_Negotiation_HTTP2(t *testing.T) {
	t.Parallel()

	serverTLS, clientTLS := generateTestTLSConfig(t)
	serverTLS.NextProtos = []string{"h2"}

	addr, cleanup := startTLSH2Server(t, serverTLS, func(req *h2server.ServerRequest, res *h2server.ServerResponse) error {
		assert.Equal(t, "GET", req.Method)
		assert.Equal(t, "/alpn-h2", req.Path)

		res.StatusCode = status.OK
		res.Headers.Set("Content-Type", "text/plain")
		res.Body = []byte("negotiated-h2")

		return nil
	})
	defer cleanup()

	engine := client.NewEngine(
		client.WithTLSConfig(clientTLS),
	)
	defer func() { _ = engine.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req := machhttp.AcquireRequest()
	resp := machhttp.AcquireResponse()
	defer machhttp.ReleaseRequest(req)
	defer machhttp.ReleaseResponse(resp)

	req.Header.SetMethod("GET")
	req.SetRequestURI("https://" + addr + "/alpn-h2")

	err := engine.Do(ctx, req, resp)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode())
	assert.Equal(t, "negotiated-h2", string(resp.Body()))
}

func TestEngine_E2E_ALPN_Negotiation_HTTP1(t *testing.T) {
	t.Parallel()

	serverTLS, clientTLS := generateTestTLSConfig(t)
	serverTLS.NextProtos = []string{"http/1.1"}

	addr, cleanup := startTLSH1Server(t, serverTLS, func(req *h1server.Request, res *h1server.Response) error {
		assert.Equal(t, "GET", req.Method)
		assert.Equal(t, "/alpn-h1", req.Path)

		res.StatusCode = status.OK
		res.Headers.Set("Content-Type", "text/plain")
		res.Body = []byte("negotiated-h1")

		return nil
	})
	defer cleanup()

	engine := client.NewEngine(
		client.WithTLSConfig(clientTLS),
	)
	defer func() { _ = engine.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req := machhttp.AcquireRequest()
	resp := machhttp.AcquireResponse()
	defer machhttp.ReleaseRequest(req)
	defer machhttp.ReleaseResponse(resp)

	req.Header.SetMethod("GET")
	req.SetRequestURI("https://" + addr + "/alpn-h1")

	err := engine.Do(ctx, req, resp)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode())
	assert.Equal(t, "negotiated-h1", string(resp.Body()))
}

func TestEngine_E2E_Plaintext_HTTP1(t *testing.T) {
	t.Parallel()

	addr, cleanup := startH1Server(t, func(req *h1server.Request, res *h1server.Response) error {
		res.StatusCode = status.OK
		res.Body = []byte("plaintext-response")

		return nil
	})
	defer cleanup()

	engine := client.NewEngine()
	defer func() { _ = engine.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req := machhttp.AcquireRequest()
	resp := machhttp.AcquireResponse()
	defer machhttp.ReleaseRequest(req)
	defer machhttp.ReleaseResponse(resp)

	req.Header.SetMethod("GET")
	req.SetRequestURI("http://" + addr + "/plain")

	err := engine.Do(ctx, req, resp)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode())
	assert.Equal(t, "plaintext-response", string(resp.Body()))
}

// ============================================================================
// PoolManager Reuse Tests
// ============================================================================

func TestEngine_E2E_PoolManager_Reuse(t *testing.T) {
	t.Parallel()

	reqCount := atomic.Int32{}
	addr, cleanup := startH1Server(t, func(req *h1server.Request, res *h1server.Response) error {
		num := reqCount.Add(1)
		res.StatusCode = status.OK
		res.Body = fmt.Appendf(nil, "pool-ack-%d", num)

		return nil
	})
	defer cleanup()

	engine := client.NewEngine(
		client.WithMaxConnsPerHost(10),
	)
	defer func() { _ = engine.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Execute 5 sequential requests on the same origin; they must reuse the pooled socket
	for i := 1; i <= 5; i++ {
		req := machhttp.AcquireRequest()
		resp := machhttp.AcquireResponse()

		req.Header.SetMethod("GET")
		req.SetRequestURI(fmt.Sprintf("http://%s/reuse-%d", addr, i))

		err := engine.Do(ctx, req, resp)
		require.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode())
		assert.Equal(t, fmt.Sprintf("pool-ack-%d", i), string(resp.Body()))

		machhttp.ReleaseRequest(req)
		machhttp.ReleaseResponse(resp)
	}
}

// ============================================================================
// Transparent Alt-Svc Upgrade to HTTP/3 (RFC 7838 / RFC 9114) Tests
// ============================================================================

func TestEngine_E2E_TransparentAltSvcUpgrade_To_HTTP3(t *testing.T) {
	t.Parallel()

	// 1. Start HTTP/3 QUIC server
	h3Addr, h3ClientTLS, h3Cleanup := startH3Server(t, func(req *h3server.ServerRequest, res *h3server.ServerResponse) error {
		assert.Equal(t, "GET", req.Method)
		assert.Equal(t, "/upgrade-test", req.Path)

		res.StatusCode = status.OK
		res.Headers.Set("X-Protocol", "HTTP/3")
		res.Body = []byte("h3-upgraded-payload")

		return nil
	})
	defer h3Cleanup()

	_, h3PortStr, err := net.SplitHostPort(h3Addr)
	require.NoError(t, err)
	h3Port, err := strconv.Atoi(h3PortStr)
	require.NoError(t, err)

	// 2. Start TLS H2 TCP server that advertises Alt-Svc pointing to the H3 port
	serverTLS, clientTLS := generateTestTLSConfig(t)
	serverTLS.NextProtos = []string{"h2"}

	h2Addr, h2Cleanup := startTLSH2Server(t, serverTLS, func(req *h2server.ServerRequest, res *h2server.ServerResponse) error {
		res.StatusCode = status.OK
		res.Headers.Set("Alt-Svc", fmt.Sprintf(`h3=":%d"; ma=3600`, h3Port))
		res.Headers.Set("X-Protocol", "HTTP/2")
		res.Body = []byte("h2-origin-payload")

		return nil
	})
	defer h2Cleanup()

	// 3. Create intelligent Engine configured with test certificates
	engine := client.NewEngine(
		client.WithTLSConfig(clientTLS),
		client.WithH3ClientTLS(h3ClientTLS),
	)
	defer func() { _ = engine.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// --- Round 1: Initial request to TCP origin ---
	// Client does not know about Alt-Svc yet; connects via H2 over TLS.
	req1 := machhttp.AcquireRequest()
	resp1 := machhttp.AcquireResponse()
	defer machhttp.ReleaseRequest(req1)
	defer machhttp.ReleaseResponse(resp1)

	req1.Header.SetMethod("GET")
	req1.SetRequestURI("https://" + h2Addr + "/upgrade-test")

	err = engine.Do(ctx, req1, resp1)
	require.NoError(t, err)
	assert.Equal(t, 200, resp1.StatusCode())
	assert.Equal(t, "h2-origin-payload", string(resp1.Body()))
	assert.Equal(t, "HTTP/2", string(resp1.Header.Peek("X-Protocol")))

	// Verify Alt-Svc cache recorded the H3 endpoint
	expectedAltAddr := fmt.Sprintf("127.0.0.1:%d", h3Port)
	cachedAddr, ok := engine.AltSvcCache.GetH3(h2Addr)
	require.True(t, ok)
	assert.Equal(t, expectedAltAddr, cachedAddr)

	// --- Round 2: Second request to origin ---
	// Client detects cached Alt-Svc entry; transparently upgrades to HTTP/3 over QUIC!
	req2 := machhttp.AcquireRequest()
	resp2 := machhttp.AcquireResponse()
	defer machhttp.ReleaseRequest(req2)
	defer machhttp.ReleaseResponse(resp2)

	req2.Header.SetMethod("GET")
	req2.SetRequestURI("https://" + h2Addr + "/upgrade-test")

	err = engine.Do(ctx, req2, resp2)
	require.NoError(t, err)
	assert.Equal(t, 200, resp2.StatusCode())
	assert.Equal(t, "h3-upgraded-payload", string(resp2.Body()))
	assert.Equal(t, "HTTP/3", string(resp2.Header.Peek("X-Protocol")))

	// --- Round 3: Subsequent request reuses active HTTP/3 connection ---
	req3 := machhttp.AcquireRequest()
	resp3 := machhttp.AcquireResponse()
	defer machhttp.ReleaseRequest(req3)
	defer machhttp.ReleaseResponse(resp3)

	req3.Header.SetMethod("GET")
	req3.SetRequestURI("https://" + h2Addr + "/upgrade-test")

	err = engine.Do(ctx, req3, resp3)
	require.NoError(t, err)
	assert.Equal(t, 200, resp3.StatusCode())
	assert.Equal(t, "h3-upgraded-payload", string(resp3.Body()))
	assert.Equal(t, "HTTP/3", string(resp3.Header.Peek("X-Protocol")))
}

func TestEngine_E2E_AltSvc_Fallback_OnFailure(t *testing.T) {
	t.Parallel()

	serverTLS, clientTLS := generateTestTLSConfig(t)
	serverTLS.NextProtos = []string{"h2"}

	// Origin advertises an Alt-Svc pointing to an unreachable UDP port (e.g. 59999)
	h2Addr, h2Cleanup := startTLSH2Server(t, serverTLS, func(req *h2server.ServerRequest, res *h2server.ServerResponse) error {
		res.StatusCode = status.OK
		res.Headers.Set("Alt-Svc", `h3=":59999"; ma=3600`)
		res.Body = []byte("fallback-origin-success")

		return nil
	})
	defer h2Cleanup()

	engine := client.NewEngine(
		client.WithTLSConfig(clientTLS),
	)
	defer func() { _ = engine.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 1. First request receives Alt-Svc header pointing to dead port
	req1 := machhttp.AcquireRequest()
	resp1 := machhttp.AcquireResponse()
	defer machhttp.ReleaseRequest(req1)
	defer machhttp.ReleaseResponse(resp1)

	req1.Header.SetMethod("GET")
	req1.SetRequestURI("https://" + h2Addr + "/fallback")

	err := engine.Do(ctx, req1, resp1)
	require.NoError(t, err)
	assert.Equal(t, 200, resp1.StatusCode())
	assert.Equal(t, "fallback-origin-success", string(resp1.Body()))

	// 2. Second request attempts H3, detects failure, and transparently falls back to origin
	req2 := machhttp.AcquireRequest()
	resp2 := machhttp.AcquireResponse()
	defer machhttp.ReleaseRequest(req2)
	defer machhttp.ReleaseResponse(resp2)

	req2.Header.SetMethod("GET")
	req2.SetRequestURI("https://" + h2Addr + "/fallback")

	err = engine.Do(ctx, req2, resp2)
	require.NoError(t, err)
	assert.Equal(t, 200, resp2.StatusCode())
	assert.Equal(t, "fallback-origin-success", string(resp2.Body()))
}

func TestEngine_E2E_AltSvc_ClearInvalidatesCache(t *testing.T) {
	t.Parallel()

	sendClear := atomic.Bool{}

	serverTLS, clientTLS := generateTestTLSConfig(t)
	serverTLS.NextProtos = []string{"h2"}

	h2Addr, h2Cleanup := startTLSH2Server(t, serverTLS, func(req *h2server.ServerRequest, res *h2server.ServerResponse) error {
		res.StatusCode = status.OK
		if sendClear.Load() {
			res.Headers.Set("Alt-Svc", "clear")
		} else {
			res.Headers.Set("Alt-Svc", `h3=":8443"; ma=3600`)
		}
		res.Body = []byte("ok")

		return nil
	})
	defer h2Cleanup()

	engine := client.NewEngine(
		client.WithTLSConfig(clientTLS),
	)
	defer func() { _ = engine.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 1. Initial request caches Alt-Svc
	req1 := machhttp.AcquireRequest()
	resp1 := machhttp.AcquireResponse()
	defer machhttp.ReleaseRequest(req1)
	defer machhttp.ReleaseResponse(resp1)

	req1.Header.SetMethod("GET")
	req1.SetRequestURI("https://" + h2Addr + "/clear-test")

	err := engine.Do(ctx, req1, resp1)
	require.NoError(t, err)
	_, ok := engine.AltSvcCache.GetH3(h2Addr)
	assert.True(t, ok)

	// 2. Second request sends "Alt-Svc: clear"
	sendClear.Store(true)
	req2 := machhttp.AcquireRequest()
	resp2 := machhttp.AcquireResponse()
	defer machhttp.ReleaseRequest(req2)
	defer machhttp.ReleaseResponse(resp2)

	req2.Header.SetMethod("GET")
	req2.SetRequestURI("https://" + h2Addr + "/clear-test")

	err = engine.Do(ctx, req2, resp2)
	require.NoError(t, err)

	// Alt-Svc cache must be invalidated
	_, ok = engine.AltSvcCache.GetH3(h2Addr)
	assert.False(t, ok)
}

// ============================================================================
// mach.Engine Contract & Universal Stream Interface Tests
// ============================================================================

func TestEngine_E2E_MachEngine_DialAndStream(t *testing.T) {
	t.Parallel()

	addr, cleanup := startH1Server(t, func(req *h1server.Request, res *h1server.Response) error {
		res.StatusCode = status.OK
		res.Body = []byte("stream-response-ok")

		return nil
	})
	defer cleanup()

	var engine mach.Engine = client.NewEngine()
	defer func() { _ = engine.(io.Closer).Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := engine.Dial(ctx, addr)
	require.NoError(t, err)
	defer conn.Close()

	stream, err := conn.OpenStream(ctx)
	require.NoError(t, err)
	defer stream.Close()

	// Write raw HTTP request
	rawReq := fmt.Sprintf("GET /stream-test HTTP/1.1\r\nHost: %s\r\n\r\n", addr)
	_, err = stream.Write([]byte(rawReq))
	require.NoError(t, err)

	// Read frame via universal mach.Frame interface
	frame, err := stream.ReadFrame()
	require.NoError(t, err)
	assert.Equal(t, mach.FrameData, frame.Type())
	assert.Contains(t, string(frame.Payload()), "HTTP/1.1 200 OK")
}

// ============================================================================
// High-Concurrency Multi-Protocol Load Test
// ============================================================================

func TestEngine_E2E_ConcurrentMultiProtocolLoad(t *testing.T) {
	t.Parallel()

	serverTLS, clientTLS := generateTestTLSConfig(t)
	serverTLS.NextProtos = []string{"h2", "http/1.1"}

	// H1 server
	h1Addr, h1Cleanup := startH1Server(t, func(req *h1server.Request, res *h1server.Response) error {
		res.StatusCode = status.OK
		res.Body = []byte("h1-concurrent-ack")

		return nil
	})
	defer h1Cleanup()

	// H2 server
	h2Addr, h2Cleanup := startTLSH2Server(t, serverTLS, func(req *h2server.ServerRequest, res *h2server.ServerResponse) error {
		res.StatusCode = status.OK
		res.Body = []byte("h2-concurrent-ack")

		return nil
	})
	defer h2Cleanup()

	engine := client.NewEngine(
		client.WithTLSConfig(clientTLS),
		client.WithMaxConnsPerHost(50),
	)
	defer func() { _ = engine.Close() }()

	concurrency := 20
	requestsPerWorker := 10
	var wg sync.WaitGroup
	errs := make([]error, concurrency)

	for workerID := range concurrency {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			for j := range requestsPerWorker {
				req := machhttp.AcquireRequest()
				resp := machhttp.AcquireResponse()

				var expected string
				if (id+j)%2 == 0 {
					req.Header.SetMethod("GET")
					req.SetRequestURI("/concur")
					req.Header.SetHost(h1Addr)
					expected = "h1-concurrent-ack"
				} else {
					req.Header.SetMethod("GET")
					req.SetRequestURI("https://" + h2Addr + "/concur")
					req.Header.SetHost(h2Addr)
					expected = "h2-concurrent-ack"
				}

				if err := engine.Do(ctx, req, resp); err != nil {
					errs[id] = fmt.Errorf("worker %d failed on request %d: %w", id, j, err)
					machhttp.ReleaseRequest(req)
					machhttp.ReleaseResponse(resp)
					return
				}

				if string(resp.Body()) != expected {
					errs[id] = fmt.Errorf("worker %d got unexpected body %q, want %q", id, string(resp.Body()), expected)
					machhttp.ReleaseRequest(req)
					machhttp.ReleaseResponse(resp)
					return
				}

				machhttp.ReleaseRequest(req)
				machhttp.ReleaseResponse(resp)
			}
		}(workerID)
	}

	wg.Wait()

	for i, err := range errs {
		require.NoErrorf(t, err, "worker %d failed", i)
	}
}
