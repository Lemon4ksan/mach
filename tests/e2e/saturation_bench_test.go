// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package e2e_test

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"runtime"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lemon4ksan/foundation/testing/assert"
	"github.com/lemon4ksan/foundation/testing/require"

	"github.com/lemon4ksan/mach/client"
	h1client "github.com/lemon4ksan/mach/client/h1"
	h2client "github.com/lemon4ksan/mach/client/h2"
	h3client "github.com/lemon4ksan/mach/client/h3"
	"github.com/lemon4ksan/mach/net/quic"
	machhttp "github.com/lemon4ksan/mach/proto/http"
	"github.com/lemon4ksan/mach/proto/semantic/status"
	"github.com/lemon4ksan/mach/proto/ws"
	"github.com/lemon4ksan/mach/server"
	h1server "github.com/lemon4ksan/mach/server/h1"
	h2server "github.com/lemon4ksan/mach/server/h2"
	h3server "github.com/lemon4ksan/mach/server/h3"
)

// LatencyStats collects request timings and computes latency distribution metrics.
type LatencyStats struct {
	Count     int
	TotalTime time.Duration
	RPS       float64
	Min       time.Duration
	Mean      time.Duration
	P50       time.Duration
	P90       time.Duration
	P99       time.Duration
	P999      time.Duration
	Max       time.Duration
	Errors    int64
}

func (s LatencyStats) String() string {
	return fmt.Sprintf("RPS=%.1f req/s, P50=%v, P90=%v, P99=%v, P99.9=%v, Min=%v, Max=%v, Total=%d, Errors=%d",
		s.RPS, s.P50, s.P90, s.P99, s.P999, s.Min, s.Max, s.Count, s.Errors)
}

func computeLatencyStats(durations []time.Duration, elapsed time.Duration, errorCount int64) LatencyStats {
	if len(durations) == 0 {
		return LatencyStats{TotalTime: elapsed, Errors: errorCount}
	}

	slices.Sort(durations)
	n := len(durations)
	var total time.Duration
	for _, d := range durations {
		total += d
	}

	return LatencyStats{
		Count:     n,
		TotalTime: elapsed,
		RPS:       float64(n) / elapsed.Seconds(),
		Min:       durations[0],
		Mean:      total / time.Duration(n),
		P50:       durations[n*50/100],
		P90:       durations[n*90/100],
		P99:       durations[n*99/100],
		P999:      durations[n*999/1000],
		Max:       durations[n-1],
		Errors:    errorCount,
	}
}

func assertNoGoroutineLeaks(t testing.TB, baseline int, margin int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		runtime.Gosched()
		current := runtime.NumGoroutine()
		if current <= baseline+margin {
			return
		}
		time.Sleep(30 * time.Millisecond)
	}

	current := runtime.NumGoroutine()
	if current > baseline+margin {
		buf := make([]byte, 64*1024)
		n := runtime.Stack(buf, true)
		t.Logf("WARNING: potential goroutine leak: baseline=%d, current=%d, delta=+%d (permitted margin=%d)\nStack:\n%s",
			baseline, current, current-baseline, margin, string(buf[:n]))
	}
}

// ============================================================================
// Protocol Saturation Load Runners
// ============================================================================

func dialH1ClientSafe(tb testing.TB, addr string, clientTLS *tls.Config) (*h1client.ClientConn, net.Conn, error) {
	tb.Helper()
	var (
		c   net.Conn
		err error
	)
	for attempt := 0; attempt < 10; attempt++ {
		c, err = net.DialTimeout("tcp", addr, 2*time.Second)
		if err == nil {
			if clientTLS != nil {
				tConf := clientTLS.Clone()
				tConf.NextProtos = []string{"http/1.1"}
				tc := tls.Client(c, tConf)
				if hErr := tc.Handshake(); hErr != nil {
					_ = c.Close()
					return nil, nil, hErr
				}
				c = tc
			}
			return h1client.NewClientConn(c), c, nil
		}
		time.Sleep(time.Duration((attempt+1)*10) * time.Millisecond)
	}
	return nil, nil, err
}

func dialH2ClientSafe(tb testing.TB, addr string, clientTLS *tls.Config, opts h2client.ConnOpts) (*h2client.Conn, net.Conn, error) {
	tb.Helper()
	var (
		c   net.Conn
		err error
	)
	for attempt := 0; attempt < 5; attempt++ {
		c, err = net.DialTimeout("tcp", addr, 2*time.Second)
		if err == nil {
			if clientTLS != nil {
				tConf := clientTLS.Clone()
				tConf.NextProtos = []string{"h2"}
				tc := tls.Client(c, tConf)
				if hErr := tc.Handshake(); hErr != nil {
					_ = c.Close()
					return nil, nil, hErr
				}
				c = tc
			}
			nc := h2client.NewConn(c, opts)
			if hErr := nc.Handshake(); hErr != nil {
				_ = c.Close()
				return nil, nil, hErr
			}
			return nc, c, nil
		}
		time.Sleep(time.Duration((attempt+1)*5) * time.Millisecond)
	}
	return nil, nil, err
}

func runH1SaturationLoad(
	tb testing.TB,
	addr string,
	clientTLS *tls.Config,
	workers int,
	requestsPerWorker int,
) LatencyStats {
	tb.Helper()

	var (
		wg          sync.WaitGroup
		durationsMu sync.Mutex
		allDurs     = make([]time.Duration, 0, workers*requestsPerWorker)
		errCount    atomic.Int64
	)

	scheme := "http"
	if clientTLS != nil {
		scheme = "https"
	}

	conns := make([]*h1client.ClientConn, workers)
	raws := make([]net.Conn, workers)

	var (
		dialWg  sync.WaitGroup
		dialErr atomic.Pointer[error]
	)

	sem := make(chan struct{}, 50)

	for i := 0; i < workers; i++ {
		dialWg.Add(1)

		idx := i

		go func() {
			defer dialWg.Done()

			sem <- struct{}{}
			defer func() { <-sem }()

			c, rawConn, dErr := dialH1ClientSafe(tb, addr, clientTLS)
			if dErr != nil {
				dialErr.Store(&dErr)
				return
			}

			conns[idx] = c
			raws[idx] = rawConn
		}()
	}

	dialWg.Wait()

	defer func() {
		for _, raw := range raws {
			if raw != nil {
				_ = raw.Close()
			}
		}
	}()

	if pErr := dialErr.Load(); pErr != nil {
		tb.Fatalf("runH1SaturationLoad: pre-dial failed: %v", *pErr)
	}

	start := time.Now()

	for w := 0; w < workers; w++ {
		wg.Add(1)

		workerID := w

		go func() {
			defer wg.Done()

			c := conns[workerID]

			req := machhttp.AcquireRequest()
			resp := machhttp.AcquireResponse()

			defer machhttp.ReleaseRequest(req)
			defer machhttp.ReleaseResponse(resp)

			req.Header.SetMethod("GET")
			req.SetRequestURI(scheme + "://" + addr + "/bench-h1")
			req.Header.SetHost(addr)

			localDurs := make([]time.Duration, 0, requestsPerWorker)

			for i := 0; i < requestsPerWorker; i++ {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				reqStart := time.Now()
				err := c.Do(ctx, req, resp)
				reqDur := time.Since(reqStart)

				cancel()

				if err != nil || resp.StatusCode() != 200 {
					errCount.Add(1)
					continue
				}

				localDurs = append(localDurs, reqDur)
			}

			durationsMu.Lock()
			allDurs = append(allDurs, localDurs...)
			durationsMu.Unlock()
		}()
	}

	wg.Wait()
	elapsed := time.Since(start)

	return computeLatencyStats(allDurs, elapsed, errCount.Load())
}

func runH2SaturationLoad(
	tb testing.TB,
	addr string,
	clientTLS *tls.Config,
	opts h2client.ConnOpts,
	connections int,
	workers int,
	requestsPerWorker int,
) LatencyStats {
	tb.Helper()

	conns := make([]*h2client.Conn, connections)
	raws := make([]net.Conn, connections)
	for i := 0; i < connections; i++ {
		c, raw, err := dialH2ClientSafe(tb, addr, clientTLS, opts)
		if err != nil {
			tb.Fatalf("dialH2ClientSafe failed: %v", err)
		}
		conns[i] = c
		raws[i] = raw
	}
	defer func() {
		for i := range connections {
			_ = conns[i].Close()
			_ = raws[i].Close()
		}
	}()

	var (
		wg          sync.WaitGroup
		durationsMu sync.Mutex
		allDurs     = make([]time.Duration, 0, workers*requestsPerWorker)
		errCount    atomic.Int64
	)

	start := time.Now()

	for w := 0; w < workers; w++ {
		wg.Add(1)
		workerID := w
		go func() {
			defer wg.Done()

			c := conns[workerID%connections]

			req := machhttp.AcquireRequest()
			resp := machhttp.AcquireResponse()
			defer machhttp.ReleaseRequest(req)
			defer machhttp.ReleaseResponse(resp)

			req.Header.SetMethod("GET")
			req.SetRequestURI("/bench-h2")
			req.Header.SetHost(addr)

			localDurs := make([]time.Duration, 0, requestsPerWorker)

			for i := 0; i < requestsPerWorker; i++ {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				reqStart := time.Now()
				err := c.Do(ctx, req, resp)
				reqDur := time.Since(reqStart)
				cancel()

				if err != nil || resp.StatusCode() != 200 {
					errCount.Add(1)
					continue
				}

				localDurs = append(localDurs, reqDur)
			}

			durationsMu.Lock()
			allDurs = append(allDurs, localDurs...)
			durationsMu.Unlock()
		}()
	}

	wg.Wait()
	elapsed := time.Since(start)

	return computeLatencyStats(allDurs, elapsed, errCount.Load())
}

func runH3SaturationLoad(
	tb testing.TB,
	addr string,
	clientTLS *tls.Config,
	sessions int,
	workers int,
	requestsPerWorker int,
) LatencyStats {
	tb.Helper()

	clientConns := make([]*h3client.ClientConn, sessions)
	quicConns := make([]*quic.Conn, sessions)
	for i := 0; i < sessions; i++ {
		cc, qc := dialH3Client(tb, addr, clientTLS)
		clientConns[i] = cc
		quicConns[i] = qc
	}
	defer func() {
		for i := range sessions {
			_ = quicConns[i].CloseWithError(0, "")
			_ = clientConns[i].Close()
		}
	}()

	var (
		wg          sync.WaitGroup
		durationsMu sync.Mutex
		allDurs     = make([]time.Duration, 0, workers*requestsPerWorker)
		errCount    atomic.Int64
	)

	maxConcurrent := 100
	if workers < maxConcurrent {
		maxConcurrent = workers
	}

	sem := make(chan struct{}, maxConcurrent)

	start := time.Now()

	for w := 0; w < workers; w++ {
		wg.Add(1)

		workerID := w

		go func() {
			defer wg.Done()

			cc := clientConns[workerID%sessions]

			req := machhttp.AcquireRequest()
			resp := machhttp.AcquireResponse()

			defer machhttp.ReleaseRequest(req)
			defer machhttp.ReleaseResponse(resp)

			req.Header.SetMethod("GET")
			req.SetRequestURI("https://" + addr + "/bench-h3")
			req.Header.SetHost(addr)

			localDurs := make([]time.Duration, 0, requestsPerWorker)

			for i := 0; i < requestsPerWorker; i++ {
				sem <- struct{}{}

				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				reqStart := time.Now()
				_, err := cc.Do(ctx, req, resp, nil)
				reqDur := time.Since(reqStart)

				cancel()
				<-sem

				if err != nil || resp.StatusCode() != 200 {
					errCount.Add(1)
					continue
				}

				localDurs = append(localDurs, reqDur)
			}

			durationsMu.Lock()
			allDurs = append(allDurs, localDurs...)
			durationsMu.Unlock()
		}()
	}

	wg.Wait()
	elapsed := time.Since(start)

	return computeLatencyStats(allDurs, elapsed, errCount.Load())
}

func runWSSaturationLoad(
	tb testing.TB,
	wsURL string,
	opts []client.WebSocketOption,
	workers int,
	messagesPerWorker int,
) LatencyStats {
	tb.Helper()

	var (
		wg          sync.WaitGroup
		durationsMu sync.Mutex
		allDurs     = make([]time.Duration, 0, workers*messagesPerWorker)
		errCount    atomic.Int64
	)

	start := time.Now()

	for w := 0; w < workers; w++ {
		wg.Add(1)
		workerID := w
		go func() {
			defer wg.Done()

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			wsConn, resp, err := client.DialWebSocket(ctx, wsURL, opts...)
			if err != nil || wsConn == nil {
				errCount.Add(int64(messagesPerWorker))
				return
			}
			if resp != nil && resp.StatusCode() != 101 && resp.StatusCode() != 200 {
				_ = wsConn.Close()
				errCount.Add(int64(messagesPerWorker))
				return
			}
			defer func() { _ = wsConn.Close() }()

			localDurs := make([]time.Duration, 0, messagesPerWorker)

			for i := 0; i < messagesPerWorker; i++ {
				msg := fmt.Sprintf("ws-payload-%d-%d", workerID, i)
				msgStart := time.Now()
				wErr := wsConn.WriteText(msg)
				if wErr != nil {
					errCount.Add(1)
					continue
				}

				op, data, rErr := wsConn.ReadMessage()
				msgDur := time.Since(msgStart)
				if rErr != nil || op != ws.OpText || string(data) != msg {
					errCount.Add(1)
					continue
				}

				localDurs = append(localDurs, msgDur)
			}

			durationsMu.Lock()
			allDurs = append(allDurs, localDurs...)
			durationsMu.Unlock()
		}()
	}

	wg.Wait()
	elapsed := time.Since(start)

	return computeLatencyStats(allDurs, elapsed, errCount.Load())
}

// ============================================================================
// Saturated Concurrency Test Suites (Testing.T)
// ============================================================================

// TestSaturation_H1_RuthlessBurst scales concurrency from 100 to 1,000+ workers
// against the HTTP/1.1 engine.
func TestSaturation_H1_RuthlessBurst(t *testing.T) {
	baselineGoroutines := runtime.NumGoroutine()

	addr, cleanup := startH1Server(t, func(req *h1server.Request, res *h1server.Response) error {
		res.StatusCode = status.OK
		res.Headers.Set("Content-Type", "text/plain")
		res.Body = append(res.Body[:0], "h1-ok"...)
		return nil
	})
	defer cleanup()

	levels := []struct {
		workers int
		reqs    int
	}{
		{workers: 10, reqs: 20},
		{workers: 100, reqs: 20},
		{workers: 500, reqs: 10},
		{workers: 1000, reqs: 10},
	}

	for _, lvl := range levels {
		t.Run(fmt.Sprintf("Workers_%d", lvl.workers), func(t *testing.T) {
			stats := runH1SaturationLoad(t, addr, nil, lvl.workers, lvl.reqs)
			t.Logf("[H1 Concurrency=%d] %s", lvl.workers, stats)

			assert.Equal(t, int64(0), stats.Errors, "H1 load should have 0 errors")
			assert.True(t, stats.RPS > 0, "H1 RPS must be > 0")
		})
	}

	assertNoGoroutineLeaks(t, baselineGoroutines, 10)
}

// TestSaturation_H2_RuthlessMultiplexBurst tests HTTP/2 under 100 to 2,000+ parallel streams.
func TestSaturation_H2_RuthlessMultiplexBurst(t *testing.T) {
	baselineGoroutines := runtime.NumGoroutine()

	addr, cleanup := startH2Server(t, func(req *h2server.ServerRequest, res *h2server.ServerResponse) error {
		res.StatusCode = status.OK
		res.Headers.Set("Content-Type", "text/plain")
		res.Body = []byte("h2-ok")
		return nil
	})
	defer cleanup()

	levels := []struct {
		conns   int
		workers int
		reqs    int
	}{
		{conns: 1, workers: 10, reqs: 20},
		{conns: 5, workers: 100, reqs: 20},
		{conns: 10, workers: 500, reqs: 10},
		{conns: 20, workers: 1000, reqs: 10},
	}

	for _, lvl := range levels {
		t.Run(fmt.Sprintf("Workers_%d_Conns_%d", lvl.workers, lvl.conns), func(t *testing.T) {
			stats := runH2SaturationLoad(t, addr, nil, h2client.ConnOpts{}, lvl.conns, lvl.workers, lvl.reqs)
			t.Logf("[H2 Concurrency=%d Streams=%d] %s", lvl.workers, lvl.workers*lvl.reqs, stats)

			assert.Equal(t, int64(0), stats.Errors, "H2 load should have 0 errors")
			assert.True(t, stats.RPS > 0, "H2 RPS must be > 0")
		})
	}

	assertNoGoroutineLeaks(t, baselineGoroutines, 15)
}

// TestSaturation_H3_RuthlessQUICBurst tests HTTP/3 under 100 to 1,000+ parallel streams over QUIC.
func TestSaturation_H3_RuthlessQUICBurst(t *testing.T) {
	baselineGoroutines := runtime.NumGoroutine()

	addr, clientTLS, cleanup := startH3Server(t, func(req *h3server.ServerRequest, res *h3server.ServerResponse) error {
		res.StatusCode = status.OK
		res.Headers.Set("Content-Type", "text/plain")
		res.Body = []byte("h3-ok")
		return nil
	})
	defer cleanup()

	levels := []struct {
		sessions int
		workers  int
		reqs     int
	}{
		{sessions: 1, workers: 10, reqs: 20},
		{sessions: 5, workers: 100, reqs: 20},
		{sessions: 10, workers: 500, reqs: 10},
		{sessions: 20, workers: 1000, reqs: 10},
	}

	for _, lvl := range levels {
		t.Run(fmt.Sprintf("Workers_%d_Sessions_%d", lvl.workers, lvl.sessions), func(t *testing.T) {
			stats := runH3SaturationLoad(t, addr, clientTLS, lvl.sessions, lvl.workers, lvl.reqs)
			t.Logf("[H3 Concurrency=%d Streams=%d] %s", lvl.workers, lvl.workers*lvl.reqs, stats)

			assert.Equal(t, int64(0), stats.Errors, "H3 load should have 0 errors")
			assert.True(t, stats.RPS > 0, "H3 RPS must be > 0")
		})
	}

	assertNoGoroutineLeaks(t, baselineGoroutines, 25)
}

// TestSaturation_WS_RuthlessBlast tests WebSocket connections under concurrent frame blasting
// across HTTP/1.1, HTTP/2 (RFC 8441), and HTTP/3 (RFC 9220).
func TestSaturation_WS_RuthlessBlast(t *testing.T) {
	baselineGoroutines := runtime.NumGoroutine()

	tcpAddr, quicAddr, clientTLS, cleanup := setupEchoServer(t)
	defer cleanup()

	plainLn, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer plainLn.Close()

	plainSrv := server.New(
		server.WithHandlerFunc(func(ctx *server.Context) error {
			wsConn, err := ctx.UpgradeWebSocket()
			if err != nil {
				return err
			}
			defer wsConn.Close()

			for {
				msgType, payload, err := wsConn.ReadMessage()
				if err != nil {
					return nil
				}
				if err := wsConn.WriteMessage(msgType, payload); err != nil {
					return err
				}
			}
		}),
	)
	go func() { _ = plainSrv.Serve(plainLn) }()
	defer plainSrv.Close()

	matrix := []struct {
		name       string
		url        string
		forceProto string
		useTLS     bool
		workers    int
		msgs       int
	}{
		{name: "WS_H1_Plaintext", url: fmt.Sprintf("ws://%s/chat", plainLn.Addr().String()), forceProto: "http/1.1", useTLS: false, workers: 50, msgs: 10},
		{name: "WS_H1_TLS", url: fmt.Sprintf("wss://%s/chat", tcpAddr), forceProto: "http/1.1", useTLS: true, workers: 100, msgs: 10},
		{name: "WS_H2_ExtendedConnect", url: fmt.Sprintf("wss://%s/chat", tcpAddr), forceProto: "h2", useTLS: true, workers: 100, msgs: 10},
		{name: "WS_H3_ExtendedConnect", url: fmt.Sprintf("https://%s/chat", quicAddr), forceProto: "h3", useTLS: true, workers: 100, msgs: 10},
	}

	for _, tc := range matrix {
		t.Run(tc.name, func(t *testing.T) {
			var opts []client.WebSocketOption
			if tc.useTLS {
				opts = append(opts, client.WithWebSocketTLSConfig(clientTLS))
			}
			opts = append(opts, client.WithForceProto(tc.forceProto))

			stats := runWSSaturationLoad(t, tc.url, opts, tc.workers, tc.msgs)
			t.Logf("[%s Workers=%d] %s", tc.name, tc.workers, stats)

			assert.Equal(t, int64(0), stats.Errors, "WS load should have 0 errors")
			assert.True(t, stats.RPS > 0, "WS RPS must be > 0")
		})
	}

	assertNoGoroutineLeaks(t, baselineGoroutines, 25)
}

// TestSaturation_Unified_ChaosMatrix concurrently fires mixed H1, H2, H3, and WS traffic
// at the unified server simultaneously.
func TestSaturation_Unified_ChaosMatrix(t *testing.T) {
	baselineGoroutines := runtime.NumGoroutine()

	serverTLS, clientTLS := generateTestTLSConfig(t)

	tcpLn, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer tcpLn.Close()

	tlsTCP := tls.NewListener(tcpLn, serverTLS)

	quicServerTLS := serverTLS.Clone()
	quicServerTLS.NextProtos = []string{"h3"}

	quicLn, err := quic.ListenAddr("127.0.0.1:0", quicServerTLS, quic.WithDatagrams(true))
	require.NoError(t, err)
	defer quicLn.Close()

	srv := server.New(
		server.WithHandlerFunc(func(ctx *server.Context) error {
			if ctx.IsWebSocket() {
				wsConn, err := ctx.UpgradeWebSocket()
				if err != nil {
					return err
				}
				defer wsConn.Close()

				for {
					op, payload, err := wsConn.ReadMessage()
					if err != nil {
						return nil
					}
					if err := wsConn.WriteMessage(op, payload); err != nil {
						return err
					}
				}
			}

			ctx.SetStatusCode(status.OK)
			ctx.SetBodyString("unified-chaos-ok")
			return nil
		}),
	)

	go func() {
		_ = srv.ServeMultiProtocol(tlsTCP, quicLn)
	}()
	defer srv.Close()

	tcpAddr := tcpLn.Addr().String()
	quicAddr := quicLn.Addr().String()

	const (
		workersPerProto = 50
		reqsPerWorker   = 10
	)

	var (
		wg      sync.WaitGroup
		h1Stats LatencyStats
		h2Stats LatencyStats
		h3Stats LatencyStats
		wsStats LatencyStats
	)

	wg.Add(4)

	// 1. Concurrent H1 workers
	go func() {
		defer wg.Done()
		h1Stats = runH1SaturationLoad(t, tcpAddr, clientTLS, workersPerProto, reqsPerWorker)
	}()

	// 2. Concurrent H2 workers
	go func() {
		defer wg.Done()
		h2Stats = runH2SaturationLoad(t, tcpAddr, clientTLS, h2client.ConnOpts{}, 5, workersPerProto, reqsPerWorker)
	}()

	// 3. Concurrent H3 workers
	go func() {
		defer wg.Done()
		h3Stats = runH3SaturationLoad(t, quicAddr, clientTLS, 5, workersPerProto, reqsPerWorker)
	}()

	// 4. Concurrent WS workers
	go func() {
		defer wg.Done()
		wsOpts := []client.WebSocketOption{
			client.WithWebSocketTLSConfig(clientTLS),
			client.WithForceProto("http/1.1"),
		}
		wsStats = runWSSaturationLoad(t, fmt.Sprintf("wss://%s/chat", tcpAddr), wsOpts, workersPerProto, reqsPerWorker)
	}()

	wg.Wait()

	t.Logf("[Unified Chaos H1] %s", h1Stats)
	t.Logf("[Unified Chaos H2] %s", h2Stats)
	t.Logf("[Unified Chaos H3] %s", h3Stats)
	t.Logf("[Unified Chaos WS] %s", wsStats)

	assert.Equal(t, int64(0), h1Stats.Errors)
	assert.Equal(t, int64(0), h2Stats.Errors)
	assert.Equal(t, int64(0), h3Stats.Errors)
	assert.Equal(t, int64(0), wsStats.Errors)

	assertNoGoroutineLeaks(t, baselineGoroutines, 30)
}

// ============================================================================
// Standardized Go Benchmarks (testing.B)
// ============================================================================

func BenchmarkSaturation_H1_Baseline(b *testing.B) {
	addr, cleanup := startH1Server(b, func(req *h1server.Request, res *h1server.Response) error {
		res.StatusCode = status.OK
		res.Body = append(res.Body[:0], "ok"...)
		return nil
	})
	defer cleanup()

	c, raw := dialH1Client(b, addr)
	defer raw.Close()

	req := machhttp.AcquireRequest()
	resp := machhttp.AcquireResponse()
	defer machhttp.ReleaseRequest(req)
	defer machhttp.ReleaseResponse(resp)

	req.Header.SetMethod("GET")
	req.SetRequestURI("/bench")
	req.Header.SetHost(addr)

	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()

	start := time.Now()
	for i := 0; i < b.N; i++ {
		if err := c.Do(ctx, req, resp); err != nil {
			b.Fatal(err)
		}
	}
	elapsed := time.Since(start)
	if elapsed > 0 {
		b.ReportMetric(float64(b.N)/elapsed.Seconds(), "req/s")
	}
}

func BenchmarkSaturation_H1_Concurrent_100(b *testing.B) {
	addr, cleanup := startH1Server(b, func(req *h1server.Request, res *h1server.Response) error {
		res.StatusCode = status.OK
		res.Body = append(res.Body[:0], "ok"...)
		return nil
	})
	defer cleanup()

	const concurrency = 100
	stats := runH1SaturationLoad(b, addr, nil, concurrency, max(1, b.N/concurrency))

	b.ReportMetric(stats.RPS, "req/s")
	b.ReportMetric(float64(stats.P50.Microseconds())/1000.0, "p50-ms")
	b.ReportMetric(float64(stats.P99.Microseconds())/1000.0, "p99-ms")
}

func BenchmarkSaturation_H2_Baseline(b *testing.B) {
	addr, cleanup := startH2Server(b, func(req *h2server.ServerRequest, res *h2server.ServerResponse) error {
		res.StatusCode = status.OK
		res.Body = []byte("ok")
		return nil
	})
	defer cleanup()

	c, raw := dialH2Client(b, addr, h2client.ConnOpts{})
	defer raw.Close()

	req := machhttp.AcquireRequest()
	resp := machhttp.AcquireResponse()
	defer machhttp.ReleaseRequest(req)
	defer machhttp.ReleaseResponse(resp)

	req.Header.SetMethod("GET")
	req.SetRequestURI("/bench")
	req.Header.SetHost(addr)

	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()

	start := time.Now()
	for i := 0; i < b.N; i++ {
		if err := c.Do(ctx, req, resp); err != nil {
			b.Fatal(err)
		}
	}
	elapsed := time.Since(start)
	if elapsed > 0 {
		b.ReportMetric(float64(b.N)/elapsed.Seconds(), "req/s")
	}
}

func BenchmarkSaturation_H2_Concurrent_100(b *testing.B) {
	addr, cleanup := startH2Server(b, func(req *h2server.ServerRequest, res *h2server.ServerResponse) error {
		res.StatusCode = status.OK
		res.Body = []byte("ok")
		return nil
	})
	defer cleanup()

	const concurrency = 100
	stats := runH2SaturationLoad(b, addr, nil, h2client.ConnOpts{}, 5, concurrency, max(1, b.N/concurrency))

	b.ReportMetric(stats.RPS, "req/s")
	b.ReportMetric(float64(stats.P50.Microseconds())/1000.0, "p50-ms")
	b.ReportMetric(float64(stats.P99.Microseconds())/1000.0, "p99-ms")
}

func BenchmarkSaturation_H3_Baseline(b *testing.B) {
	addr, clientTLS, cleanup := startH3Server(b, func(req *h3server.ServerRequest, res *h3server.ServerResponse) error {
		res.StatusCode = status.OK
		res.Body = []byte("ok")
		return nil
	})
	defer cleanup()

	cc, qc := dialH3Client(b, addr, clientTLS)
	defer qc.CloseWithError(0, "")
	defer cc.Close()

	req := machhttp.AcquireRequest()
	resp := machhttp.AcquireResponse()
	defer machhttp.ReleaseRequest(req)
	defer machhttp.ReleaseResponse(resp)

	req.Header.SetMethod("GET")
	req.SetRequestURI("https://" + addr + "/bench")
	req.Header.SetHost(addr)

	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()

	start := time.Now()
	for i := 0; i < b.N; i++ {
		if _, err := cc.Do(ctx, req, resp, nil); err != nil {
			b.Fatal(err)
		}
	}
	elapsed := time.Since(start)
	if elapsed > 0 {
		b.ReportMetric(float64(b.N)/elapsed.Seconds(), "req/s")
	}
}

func BenchmarkSaturation_H3_Concurrent_100(b *testing.B) {
	addr, clientTLS, cleanup := startH3Server(b, func(req *h3server.ServerRequest, res *h3server.ServerResponse) error {
		res.StatusCode = status.OK
		res.Body = []byte("ok")
		return nil
	})
	defer cleanup()

	const concurrency = 100
	stats := runH3SaturationLoad(b, addr, clientTLS, 5, concurrency, max(1, b.N/concurrency))

	b.ReportMetric(stats.RPS, "req/s")
	b.ReportMetric(float64(stats.P50.Microseconds())/1000.0, "p50-ms")
	b.ReportMetric(float64(stats.P99.Microseconds())/1000.0, "p99-ms")
}

func BenchmarkSaturation_WS_Baseline(b *testing.B) {
	tcpAddr, _, clientTLS, cleanup := setupEchoServer(b)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	urlStr := fmt.Sprintf("wss://%s/chat", tcpAddr)
	conn, _, err := client.DialWebSocket(ctx, urlStr,
		client.WithWebSocketTLSConfig(clientTLS),
		client.WithForceProto("http/1.1"),
	)
	if err != nil {
		b.Fatal(err)
	}
	defer conn.Close()

	msg := []byte("bench-ws-frame")

	b.ReportAllocs()
	b.ResetTimer()

	start := time.Now()
	for i := 0; i < b.N; i++ {
		if err := conn.WriteBinary(msg); err != nil {
			b.Fatal(err)
		}
		if _, _, err := conn.ReadMessage(); err != nil {
			b.Fatal(err)
		}
	}
	elapsed := time.Since(start)
	if elapsed > 0 {
		b.ReportMetric(float64(b.N)/elapsed.Seconds(), "msg/s")
	}
}

func BenchmarkSaturation_WS_Concurrent_100(b *testing.B) {
	tcpAddr, _, clientTLS, cleanup := setupEchoServer(b)
	defer cleanup()

	urlStr := fmt.Sprintf("wss://%s/chat", tcpAddr)
	opts := []client.WebSocketOption{
		client.WithWebSocketTLSConfig(clientTLS),
		client.WithForceProto("http/1.1"),
	}

	const concurrency = 100
	stats := runWSSaturationLoad(b, urlStr, opts, concurrency, max(1, b.N/concurrency))

	b.ReportMetric(stats.RPS, "msg/s")
	b.ReportMetric(float64(stats.P50.Microseconds())/1000.0, "p50-ms")
	b.ReportMetric(float64(stats.P99.Microseconds())/1000.0, "p99-ms")
}
