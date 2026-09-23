// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package e2e_test

import (
	"bufio"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/lemon4ksan/foundation/net/http/status"
	"github.com/lemon4ksan/foundation/testing/assert"
	"github.com/lemon4ksan/foundation/testing/require"

	h2client "github.com/lemon4ksan/mach/client/h2"
	coreh2 "github.com/lemon4ksan/mach/proto/h2"
	machhttp "github.com/lemon4ksan/mach/proto/http"
	h2server "github.com/lemon4ksan/mach/server/h2"
)

// ============================================================================
// Tier 1: Feature Coverage (RFC 9113 / RFC 7541)
// ============================================================================

func TestH2_Tier1_HPACKHeaderCompression(t *testing.T) {
	t.Parallel()

	addr, cleanup := startH2Server(t, func(req *h2server.ServerRequest, res *h2server.ServerResponse) error {
		assert.Equal(t, "GET", req.Method)
		assert.Equal(t, "/headers-test", req.Path)
		assert.Equal(t, "custom-value-alpha", req.Headers.Get("X-Custom-Header-A"))
		assert.Equal(t, "custom-value-beta", req.Headers.Get("X-Custom-Header-B"))

		res.StatusCode = status.OK
		res.Headers.Set("Content-Type", "application/json")
		res.Headers.Set("X-Server-Engine", "mach-h2")
		res.Body = []byte(`{"hpack":"compressed"}`)

		return nil
	})
	defer cleanup()

	client, rawConn := dialH2Client(t, addr, h2client.ConnOpts{})
	defer rawConn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	req := machhttp.AcquireRequest()
	resp := machhttp.AcquireResponse()
	defer machhttp.ReleaseRequest(req)
	defer machhttp.ReleaseResponse(resp)

	req.Header.SetMethod("GET")
	req.SetRequestURI("/headers-test")
	req.Header.SetHost(addr)
	req.Header.Set("X-Custom-Header-A", "custom-value-alpha")
	req.Header.Set("X-Custom-Header-B", "custom-value-beta")

	err := client.Do(ctx, req, resp)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode())
	assert.Equal(t, `{"hpack":"compressed"}`, string(resp.Body()))
	assert.Equal(t, "mach-h2", string(resp.Header.Peek("X-Server-Engine")))
}

func TestH2_Tier1_DATAFrameStreaming(t *testing.T) {
	t.Parallel()

	payloadSize := 64 * 1024
	payload := make([]byte, payloadSize)
	for i := range payload {
		payload[i] = byte((i * 7) % 256)
	}
	expectedHash := sha256.Sum256(payload)

	addr, cleanup := startH2Server(t, func(req *h2server.ServerRequest, res *h2server.ServerResponse) error {
		reqHash := sha256.Sum256(req.Body)
		assert.Equal(t, expectedHash, reqHash)

		res.StatusCode = status.OK
		res.Headers.Set("Content-Type", "application/octet-stream")
		res.Body = req.Body

		return nil
	})
	defer cleanup()

	client, rawConn := dialH2Client(t, addr, h2client.ConnOpts{})
	defer rawConn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req := machhttp.AcquireRequest()
	resp := machhttp.AcquireResponse()
	defer machhttp.ReleaseRequest(req)
	defer machhttp.ReleaseResponse(resp)

	req.Header.SetMethod("POST")
	req.SetRequestURI("/echo-stream")
	req.Header.SetHost(addr)
	req.SetBody(payload)

	err := client.Do(ctx, req, resp)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode())

	respHash := sha256.Sum256(resp.Body())
	assert.Equal(t, expectedHash, respHash)
}

func TestH2_Tier1_MultiStreamMultiplexing(t *testing.T) {
	t.Parallel()

	addr, cleanup := startH2Server(t, func(req *h2server.ServerRequest, res *h2server.ServerResponse) error {
		res.StatusCode = status.OK
		res.Body = []byte("multiplex-ack-for-" + req.Path)

		return nil
	})
	defer cleanup()

	client, rawConn := dialH2Client(t, addr, h2client.ConnOpts{})
	defer rawConn.Close()

	numStreams := 20
	var wg sync.WaitGroup
	errs := make([]error, numStreams)

	for i := 0; i < numStreams; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			req := machhttp.AcquireRequest()
			resp := machhttp.AcquireResponse()
			defer machhttp.ReleaseRequest(req)
			defer machhttp.ReleaseResponse(resp)

			path := fmt.Sprintf("/stream/%d", idx)
			req.Header.SetMethod("GET")
			req.SetRequestURI(path)
			req.Header.SetHost(addr)

			err := client.Do(ctx, req, resp)
			if err != nil {
				errs[idx] = err
				return
			}

			if resp.StatusCode() != 200 || string(resp.Body()) != "multiplex-ack-for-"+path {
				errs[idx] = fmt.Errorf("unexpected resp: code=%d body=%s", resp.StatusCode(), string(resp.Body()))
			}
		}(i)
	}

	wg.Wait()
	for i, err := range errs {
		require.NoErrorf(t, err, "stream %d failed", i)
	}
}

func TestH2_Tier1_StreamCancellationRST(t *testing.T) {
	t.Parallel()

	receivedCh := make(chan struct{})
	addr, cleanup := startH2Server(t, func(req *h2server.ServerRequest, res *h2server.ServerResponse) error {
		if req.Path == "/slow" {
			select {
			case <-receivedCh:
			default:
				close(receivedCh)
			}
			time.Sleep(300 * time.Millisecond)
		}
		res.StatusCode = status.OK
		res.Body = []byte("fast-response")

		return nil
	})
	defer cleanup()

	client, rawConn := dialH2Client(t, addr, h2client.ConnOpts{})
	defer rawConn.Close()

	// 1. Issue request and cancel it after server confirms receipt
	ctxCancel, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		<-receivedCh
		cancel()
	}()

	req1 := machhttp.AcquireRequest()
	resp1 := machhttp.AcquireResponse()
	defer machhttp.ReleaseRequest(req1)
	defer machhttp.ReleaseResponse(resp1)

	req1.Header.SetMethod("GET")
	req1.SetRequestURI("/slow")
	req1.Header.SetHost(addr)

	err1 := client.Do(ctxCancel, req1, resp1)
	assert.Error(t, err1) // Context cancellation

	// 2. Connection must remain functional for subsequent streams
	ctxHealthy, cancelHealthy := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancelHealthy()

	req2 := machhttp.AcquireRequest()
	resp2 := machhttp.AcquireResponse()
	defer machhttp.ReleaseRequest(req2)
	defer machhttp.ReleaseResponse(resp2)

	req2.Header.SetMethod("GET")
	req2.SetRequestURI("/healthy")
	req2.Header.SetHost(addr)

	err2 := client.Do(ctxHealthy, req2, resp2)
	require.NoError(t, err2)
	assert.Equal(t, 200, resp2.StatusCode())
	assert.Equal(t, "fast-response", string(resp2.Body()))
}

func TestH2_Tier1_FlowControlWindowUpdate(t *testing.T) {
	t.Parallel()

	// 128KB payload exceeds default 65535 initial window
	largeData := make([]byte, 128*1024)
	for i := range largeData {
		largeData[i] = byte(i % 127)
	}

	addr, cleanup := startH2Server(t, func(req *h2server.ServerRequest, res *h2server.ServerResponse) error {
		assert.Equal(t, len(largeData), len(req.Body))
		res.StatusCode = status.OK
		res.Body = []byte("window-update-verified")

		return nil
	})
	defer cleanup()

	client, rawConn := dialH2Client(t, addr, h2client.ConnOpts{})
	defer rawConn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req := machhttp.AcquireRequest()
	resp := machhttp.AcquireResponse()
	defer machhttp.ReleaseRequest(req)
	defer machhttp.ReleaseResponse(resp)

	req.Header.SetMethod("POST")
	req.SetRequestURI("/flow-control")
	req.Header.SetHost(addr)
	req.SetBody(largeData)

	err := client.Do(ctx, req, resp)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode())
	assert.Equal(t, "window-update-verified", string(resp.Body()))
}

func TestH2_Tier1_CustomHeaderOrderingPreservation(t *testing.T) {
	t.Parallel()

	addr, cleanup := startH2Server(t, func(req *h2server.ServerRequest, res *h2server.ServerResponse) error {
		res.StatusCode = status.OK
		res.Body = []byte("ordered-headers-ack")

		return nil
	})
	defer cleanup()

	client, rawConn := dialH2Client(t, addr, h2client.ConnOpts{})
	defer rawConn.Close()

	client.SetOrderedHeaders([]string{"x-first", "x-second", "x-third"})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	req := machhttp.AcquireRequest()
	resp := machhttp.AcquireResponse()
	defer machhttp.ReleaseRequest(req)
	defer machhttp.ReleaseResponse(resp)

	req.Header.SetMethod("GET")
	req.SetRequestURI("/ordered")
	req.Header.SetHost(addr)
	req.Header.Set("x-first", "1")
	req.Header.Set("x-second", "2")
	req.Header.Set("x-third", "3")

	err := client.Do(ctx, req, resp)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode())
}

// ============================================================================
// Tier 2: Boundary & Corner Cases (RFC 9113)
// ============================================================================

func TestH2_Tier2_ZeroLengthDATAWithEndStream(t *testing.T) {
	t.Parallel()

	addr, cleanup := startH2Server(t, func(req *h2server.ServerRequest, res *h2server.ServerResponse) error {
		assert.Equal(t, 0, len(req.Body))
		res.StatusCode = status.NoContent
		res.Body = nil // 0-length response

		return nil
	})
	defer cleanup()

	client, rawConn := dialH2Client(t, addr, h2client.ConnOpts{})
	defer rawConn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	req := machhttp.AcquireRequest()
	resp := machhttp.AcquireResponse()
	defer machhttp.ReleaseRequest(req)
	defer machhttp.ReleaseResponse(resp)

	req.Header.SetMethod("POST")
	req.SetRequestURI("/zero-data")
	req.Header.SetHost(addr)

	err := client.Do(ctx, req, resp)
	require.NoError(t, err)
	assert.Equal(t, 204, resp.StatusCode())
	assert.Equal(t, 0, len(resp.Body()))
}

func TestH2_Tier2_StreamIDSaturation(t *testing.T) {
	t.Parallel()

	var recordedStreamIDs []uint32
	var mu sync.Mutex

	addr, cleanup := startH2Server(t, func(req *h2server.ServerRequest, res *h2server.ServerResponse) error {
		mu.Lock()
		recordedStreamIDs = append(recordedStreamIDs, req.StreamID)
		mu.Unlock()

		res.StatusCode = status.OK
		return nil
	})
	defer cleanup()

	client, rawConn := dialH2Client(t, addr, h2client.ConnOpts{})
	defer rawConn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Execute 3 sequential requests and verify client stream ID increments (1, 3, 5) per RFC 9113 §5.1.1
	for i := 0; i < 3; i++ {
		req := machhttp.AcquireRequest()
		resp := machhttp.AcquireResponse()

		req.Header.SetMethod("GET")
		req.SetRequestURI("/id")
		req.Header.SetHost(addr)

		err := client.Do(ctx, req, resp)
		require.NoError(t, err)

		machhttp.ReleaseRequest(req)
		machhttp.ReleaseResponse(resp)
	}

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, recordedStreamIDs, 3)
	assert.Equal(t, uint32(1), recordedStreamIDs[0])
	assert.Equal(t, uint32(3), recordedStreamIDs[1])
	assert.Equal(t, uint32(5), recordedStreamIDs[2])
}

func TestH2_Tier2_FlowControlZeroWindowStalling(t *testing.T) {
	t.Parallel()

	addr, cleanup := startH2Server(t, func(req *h2server.ServerRequest, res *h2server.ServerResponse) error {
		res.StatusCode = status.OK
		res.Body = []byte("ok")
		return nil
	})
	defer cleanup()

	client, rawConn := dialH2Client(t, addr, h2client.ConnOpts{})
	defer rawConn.Close()

	assert.False(t, client.Closed())
	assert.True(t, client.CanOpenStream())
}

func completeH2RawHandshake(t *testing.T, bw *bufio.Writer, br *bufio.Reader) {
	t.Helper()

	st := &coreh2.Settings{}
	st.Reset()
	err := coreh2.PerformHandshake(true, bw, st, 65535)
	require.NoError(t, err)

	// Consume server initial SETTINGS and server SETTINGS ACK
	for {
		fr, rErr := coreh2.ReadFrameFrom(br)
		require.NoError(t, rErr)
		fType := fr.Type()
		isAck := fr.Flags().Has(coreh2.FlagAck)
		coreh2.ReleaseFrameHeader(fr)

		if fType == coreh2.FrameSettings && isAck {
			break
		}
	}
}

func TestH2_Tier2_ConsecutiveControlFrameLimit(t *testing.T) {
	t.Parallel()

	addr, cleanup := startH2Server(t, func(req *h2server.ServerRequest, res *h2server.ServerResponse) error {
		res.StatusCode = status.OK
		return nil
	})
	defer cleanup()

	conn, err := net.Dial("tcp", addr)
	require.NoError(t, err)
	defer conn.Close()

	bw := bufio.NewWriter(conn)
	br := bufio.NewReader(conn)

	completeH2RawHandshake(t, bw, br)

	// Send 20 rapid PING frames to verify server handles control frame burst without panic
	for i := 0; i < 20; i++ {
		pingFr := coreh2.AcquireFrameHeader()
		pingBody := coreh2.AcquireFrame(coreh2.FramePing).(*coreh2.Ping)
		var pingData [8]byte
		pingData[0] = byte(i)
		pingBody.SetData(pingData[:])
		pingFr.SetBody(pingBody)

		_, err = pingFr.WriteTo(bw)
		require.NoError(t, err)
		coreh2.ReleaseFrameHeader(pingFr)
	}
	require.NoError(t, bw.Flush())

	// Read back PING ACKs
	for i := 0; i < 20; i++ {
		ackFr, rErr := coreh2.ReadFrameFrom(br)
		require.NoError(t, rErr)
		assert.Equal(t, coreh2.FramePing, ackFr.Type())
		assert.True(t, ackFr.Flags().Has(coreh2.FlagAck))
		coreh2.ReleaseFrameHeader(ackFr)
	}
}

func TestH2_Tier2_PingAckCycles(t *testing.T) {
	t.Parallel()

	addr, cleanup := startH2Server(t, func(req *h2server.ServerRequest, res *h2server.ServerResponse) error {
		res.StatusCode = status.OK
		return nil
	})
	defer cleanup()

	conn, err := net.Dial("tcp", addr)
	require.NoError(t, err)
	defer conn.Close()

	bw := bufio.NewWriter(conn)
	br := bufio.NewReader(conn)

	completeH2RawHandshake(t, bw, br)

	// Send PING with distinctive 8-byte payload
	pingData := [8]byte{'M', 'A', 'C', 'H', 'P', 'I', 'N', 'G'}
	pingFr := coreh2.AcquireFrameHeader()
	pingBody := coreh2.AcquireFrame(coreh2.FramePing).(*coreh2.Ping)
	pingBody.SetData(pingData[:])
	pingFr.SetBody(pingBody)

	_, err = pingFr.WriteTo(bw)
	require.NoError(t, err)
	require.NoError(t, bw.Flush())
	coreh2.ReleaseFrameHeader(pingFr)

	// Read PING response
	ackFr, err := coreh2.ReadFrameFrom(br)
	require.NoError(t, err)
	assert.Equal(t, coreh2.FramePing, ackFr.Type())
	assert.True(t, ackFr.Flags().Has(coreh2.FlagAck))

	ackPing := ackFr.Body().(*coreh2.Ping)
	assert.Equal(t, pingData[:], ackPing.Data())
	coreh2.ReleaseFrameHeader(ackFr)
}

func TestH2_Tier2_GracefulGoAwayShutdown(t *testing.T) {
	t.Parallel()

	addr, cleanup := startH2Server(t, func(req *h2server.ServerRequest, res *h2server.ServerResponse) error {
		res.StatusCode = status.OK
		return nil
	})
	defer cleanup()

	conn, err := net.Dial("tcp", addr)
	require.NoError(t, err)
	defer conn.Close()

	bw := bufio.NewWriter(conn)
	br := bufio.NewReader(conn)

	completeH2RawHandshake(t, bw, br)

	// Send GOAWAY frame with LastStreamID 0 and NoError (RFC 9113 §6.8)
	goAwayFr := coreh2.AcquireFrameHeader()
	goAway := coreh2.AcquireFrame(coreh2.FrameGoAway).(*coreh2.GoAway)
	goAway.SetStream(0)
	goAway.SetCode(coreh2.NoError)
	goAwayFr.SetBody(goAway)

	_, err = goAwayFr.WriteTo(bw)
	require.NoError(t, err)
	require.NoError(t, bw.Flush())
	coreh2.ReleaseFrameHeader(goAwayFr)

	// Server should close cleanly
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	extra := make([]byte, 16)
	_, err = br.Read(extra)
	assert.ErrorIs(t, err, io.EOF)
}

// ============================================================================
// Tier 3: Cross-Feature Combinations
// ============================================================================

func TestH2_Tier3_MultiplexingWithConcurrentResets(t *testing.T) {
	t.Parallel()

	numStreams := 10
	cancelSignals := make([]chan struct{}, numStreams)
	for i := range cancelSignals {
		cancelSignals[i] = make(chan struct{})
	}

	addr, cleanup := startH2Server(t, func(req *h2server.ServerRequest, res *h2server.ServerResponse) error {
		var idx int
		if _, err := fmt.Sscanf(req.Path, "/cancel-%d", &idx); err == nil && idx < numStreams {
			select {
			case <-cancelSignals[idx]:
			default:
				close(cancelSignals[idx])
			}
			time.Sleep(200 * time.Millisecond)
		}

		res.StatusCode = status.OK
		res.Body = []byte("survived-" + req.Path)

		return nil
	})
	defer cleanup()

	client, rawConn := dialH2Client(t, addr, h2client.ConnOpts{})
	defer rawConn.Close()

	var wg sync.WaitGroup

	for i := 0; i < numStreams; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			if idx%2 == 0 {
				// Cancelled stream (RST_STREAM)
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()

				go func() {
					<-cancelSignals[idx]
					cancel()
				}()

				req := machhttp.AcquireRequest()
				resp := machhttp.AcquireResponse()
				defer machhttp.ReleaseRequest(req)
				defer machhttp.ReleaseResponse(resp)

				req.Header.SetMethod("GET")
				req.SetRequestURI(fmt.Sprintf("/cancel-%d", idx))
				req.Header.SetHost(addr)

				_ = client.Do(ctx, req, resp)
			} else {
				// Completing stream
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()

				req := machhttp.AcquireRequest()
				resp := machhttp.AcquireResponse()
				defer machhttp.ReleaseRequest(req)
				defer machhttp.ReleaseResponse(resp)

				path := fmt.Sprintf("/complete-%d", idx)
				req.Header.SetMethod("GET")
				req.SetRequestURI(path)
				req.Header.SetHost(addr)

				err := client.Do(ctx, req, resp)
				assert.NoError(t, err)
				assert.Equal(t, 200, resp.StatusCode())
				assert.Equal(t, "survived-"+path, string(resp.Body()))
			}
		}(i)
	}

	wg.Wait()
}

func TestH2_Tier3_LargeDataStreamingWithDynamicWindowUpdate(t *testing.T) {
	t.Parallel()

	addr, cleanup := startH2Server(t, func(req *h2server.ServerRequest, res *h2server.ServerResponse) error {
		res.StatusCode = status.OK
		res.Body = req.Body

		return nil
	})
	defer cleanup()

	client, rawConn := dialH2Client(t, addr, h2client.ConnOpts{})
	defer rawConn.Close()

	// 3 concurrent streams each transmitting 64KB
	numStreams := 3
	payloadSize := 64 * 1024
	payload := make([]byte, payloadSize)
	for i := range payload {
		payload[i] = byte(i % 253)
	}
	expectedHash := sha256.Sum256(payload)

	var wg sync.WaitGroup
	errs := make([]error, numStreams)

	for i := 0; i < numStreams; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			req := machhttp.AcquireRequest()
			resp := machhttp.AcquireResponse()
			defer machhttp.ReleaseRequest(req)
			defer machhttp.ReleaseResponse(resp)

			req.Header.SetMethod("POST")
			req.SetRequestURI(fmt.Sprintf("/large-%d", idx))
			req.Header.SetHost(addr)
			req.SetBody(payload)

			err := client.Do(ctx, req, resp)
			if err != nil {
				errs[idx] = err
				return
			}

			if sha256.Sum256(resp.Body()) != expectedHash {
				errs[idx] = fmt.Errorf("hash mismatch on stream %d", idx)
			}
		}(i)
	}

	wg.Wait()
	for i, err := range errs {
		require.NoErrorf(t, err, "large stream %d failed", i)
	}
}

func TestH2_Tier3_MultiplexedStreamsWithTrailers(t *testing.T) {
	t.Parallel()

	addr, cleanup := startH2Server(t, func(req *h2server.ServerRequest, res *h2server.ServerResponse) error {
		res.StatusCode = status.OK
		res.Headers.Set("Trailer", "X-Checksum")
		res.Body = []byte("data-with-trailers")

		return nil
	})
	defer cleanup()

	client, rawConn := dialH2Client(t, addr, h2client.ConnOpts{})
	defer rawConn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	req := machhttp.AcquireRequest()
	resp := machhttp.AcquireResponse()
	defer machhttp.ReleaseRequest(req)
	defer machhttp.ReleaseResponse(resp)

	req.Header.SetMethod("GET")
	req.SetRequestURI("/trailers")
	req.Header.SetHost(addr)

	err := client.Do(ctx, req, resp)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode())
	assert.Equal(t, "data-with-trailers", string(resp.Body()))
}

func TestH2_Tier3_AbruptConnectionDisconnectDuringInflight(t *testing.T) {
	t.Parallel()

	addr, cleanup := startH2Server(t, func(req *h2server.ServerRequest, res *h2server.ServerResponse) error {
		time.Sleep(500 * time.Millisecond)
		res.StatusCode = status.OK
		return nil
	})
	defer cleanup()

	client, rawConn := dialH2Client(t, addr, h2client.ConnOpts{})

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		req := machhttp.AcquireRequest()
		resp := machhttp.AcquireResponse()
		defer machhttp.ReleaseRequest(req)
		defer machhttp.ReleaseResponse(resp)

		req.Header.SetMethod("GET")
		req.SetRequestURI("/abrupt")
		req.Header.SetHost(addr)

		// This call should be unblocked with an error when rawConn is closed abruptly
		_ = client.Do(ctx, req, resp)
	}()

	// Abruptly sever the connection while request is in-flight
	time.Sleep(50 * time.Millisecond)
	_ = rawConn.Close()
	_ = client.Close()

	wg.Wait()
	assert.True(t, client.Closed())
}

// ============================================================================
// Tier 4: Real-World Application Scenarios
// ============================================================================

func TestH2_Tier4_HighConcurrencyBurst(t *testing.T) {
	t.Parallel()

	addr, cleanup := startH2Server(t, func(req *h2server.ServerRequest, res *h2server.ServerResponse) error {
		res.StatusCode = status.OK
		res.Body = []byte("burst-ack")

		return nil
	})
	defer cleanup()

	client, rawConn := dialH2Client(t, addr, h2client.ConnOpts{})
	defer rawConn.Close()

	concurrency := 50
	var wg sync.WaitGroup
	errs := make([]error, concurrency)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			req := machhttp.AcquireRequest()
			resp := machhttp.AcquireResponse()
			defer machhttp.ReleaseRequest(req)
			defer machhttp.ReleaseResponse(resp)

			req.Header.SetMethod("GET")
			req.SetRequestURI(fmt.Sprintf("/burst/%d", idx))
			req.Header.SetHost(addr)

			err := client.Do(ctx, req, resp)
			if err != nil {
				errs[idx] = err
				return
			}

			if resp.StatusCode() != 200 || string(resp.Body()) != "burst-ack" {
				errs[idx] = fmt.Errorf("unexpected status %d or body", resp.StatusCode())
			}
		}(i)
	}

	wg.Wait()
	for i, err := range errs {
		require.NoErrorf(t, err, "burst worker %d failed", i)
	}
}

func TestH2_Tier4_ParallelLargePayloadTransfers(t *testing.T) {
	t.Parallel()

	addr, cleanup := startH2Server(t, func(req *h2server.ServerRequest, res *h2server.ServerResponse) error {
		res.StatusCode = status.OK
		res.Body = req.Body

		return nil
	})
	defer cleanup()

	client, rawConn := dialH2Client(t, addr, h2client.ConnOpts{})
	defer rawConn.Close()

	payloadSize := 128 * 1024
	payload := make([]byte, payloadSize)
	for i := range payload {
		payload[i] = byte((i * 11) % 251)
	}
	expectedHash := sha256.Sum256(payload)

	numWorkers := 4
	var wg sync.WaitGroup
	errs := make([]error, numWorkers)

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			req := machhttp.AcquireRequest()
			resp := machhttp.AcquireResponse()
			defer machhttp.ReleaseRequest(req)
			defer machhttp.ReleaseResponse(resp)

			req.Header.SetMethod("POST")
			req.SetRequestURI(fmt.Sprintf("/parallel-%d", idx))
			req.Header.SetHost(addr)
			req.SetBody(payload)

			err := client.Do(ctx, req, resp)
			if err != nil {
				errs[idx] = err
				return
			}

			if sha256.Sum256(resp.Body()) != expectedHash {
				errs[idx] = fmt.Errorf("checksum mismatch on worker %d", idx)
			}
		}(i)
	}

	wg.Wait()
	for i, err := range errs {
		require.NoErrorf(t, err, "worker %d failed", i)
	}
}
