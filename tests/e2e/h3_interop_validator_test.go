// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package e2e_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/lemon4ksan/foundation/encoding/varint"
	"github.com/lemon4ksan/foundation/testing/assert"
	"github.com/lemon4ksan/foundation/testing/require"
	"github.com/lemon4ksan/mach/net/quic"
	"github.com/lemon4ksan/mach/proto/codec/qpack"
	coreh3 "github.com/lemon4ksan/mach/proto/h3"
	machhttp "github.com/lemon4ksan/mach/proto/http"
	"github.com/lemon4ksan/mach/proto/semantic/status"
	h3server "github.com/lemon4ksan/mach/server/h3"
)

// ============================================================================
// Suite 1: High-Throughput Multi-Megabyte Transfers & Flow Control Reassembly
// ============================================================================

func TestH3_PayloadTransfer_1MB(t *testing.T) {
	payloadSize := 1024 * 1024 // 1 MB
	payload := make([]byte, payloadSize)
	for i := range payload {
		payload[i] = byte((i * 31) % 251)
	}
	expectedHash := sha256.Sum256(payload)

	addr, clientTLS, cleanup := startH3Server(t, func(req *h3server.ServerRequest, res *h3server.ServerResponse) error {
		reqHash := sha256.Sum256(req.Body)
		assert.Equal(t, expectedHash, reqHash)

		res.StatusCode = status.OK
		res.Headers.Set("Content-Type", "application/octet-stream")
		res.Body = req.Body

		return nil
	})
	defer cleanup()

	client, quicConn := dialH3Client(t, addr, clientTLS)
	defer quicConn.CloseWithError(0, "")
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	req := machhttp.AcquireRequest()
	resp := machhttp.AcquireResponse()
	defer machhttp.ReleaseRequest(req)
	defer machhttp.ReleaseResponse(resp)

	req.Header.SetMethod("POST")
	req.SetRequestURI("https://" + addr + "/large-1mb")
	req.SetBody(payload)

	_, err := client.Do(ctx, req, resp, nil)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode())

	respHash := sha256.Sum256(resp.Body())
	assert.Equal(t, expectedHash, respHash)
}

func TestH3_PayloadTransfer_5MB(t *testing.T) {
	payloadSize := 5 * 1024 * 1024 // 5 MB
	payload := make([]byte, payloadSize)
	for i := range payload {
		payload[i] = byte((i * 37) % 251)
	}
	expectedHash := sha256.Sum256(payload)

	addr, clientTLS, cleanup := startH3Server(t, func(req *h3server.ServerRequest, res *h3server.ServerResponse) error {
		reqHash := sha256.Sum256(req.Body)
		assert.Equal(t, expectedHash, reqHash)

		res.StatusCode = status.OK
		res.Headers.Set("Content-Type", "application/octet-stream")
		res.Body = req.Body

		return nil
	})
	defer cleanup()

	client, quicConn := dialH3Client(t, addr, clientTLS)
	defer quicConn.CloseWithError(0, "")
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req := machhttp.AcquireRequest()
	resp := machhttp.AcquireResponse()
	defer machhttp.ReleaseRequest(req)
	defer machhttp.ReleaseResponse(resp)

	req.Header.SetMethod("POST")
	req.SetRequestURI("https://" + addr + "/large-5mb")
	req.SetBody(payload)

	_, err := client.Do(ctx, req, resp, nil)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode())

	respHash := sha256.Sum256(resp.Body())
	assert.Equal(t, expectedHash, respHash)
}

func TestH3_PayloadTransfer_10MB(t *testing.T) {
	payloadSize := 10 * 1024 * 1024 // 10 MB
	payload := make([]byte, payloadSize)
	for i := range payload {
		payload[i] = byte((i * 41) % 251)
	}
	expectedHash := sha256.Sum256(payload)

	addr, clientTLS, cleanup := startH3Server(t, func(req *h3server.ServerRequest, res *h3server.ServerResponse) error {
		reqHash := sha256.Sum256(req.Body)
		assert.Equal(t, expectedHash, reqHash)

		res.StatusCode = status.OK
		res.Headers.Set("Content-Type", "application/octet-stream")
		res.Body = req.Body

		return nil
	})
	defer cleanup()

	client, quicConn := dialH3Client(t, addr, clientTLS)
	defer quicConn.CloseWithError(0, "")
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	req := machhttp.AcquireRequest()
	resp := machhttp.AcquireResponse()
	defer machhttp.ReleaseRequest(req)
	defer machhttp.ReleaseResponse(resp)

	req.Header.SetMethod("POST")
	req.SetRequestURI("https://" + addr + "/large-10mb")
	req.SetBody(payload)

	_, err := client.Do(ctx, req, resp, nil)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode())

	respHash := sha256.Sum256(resp.Body())
	assert.Equal(t, expectedHash, respHash)
}

// ============================================================================
// Suite 2: Chunked Streaming & Multi-Frame Flow
// ============================================================================

func TestH3_ChunkedStreaming_MultiDataFrame(t *testing.T) {
	t.Parallel()

	numChunks := 8
	chunkSize := 16 * 1024 // 16 KB per chunk = 128 KB total
	fullPayload := make([]byte, numChunks*chunkSize)
	for i := range fullPayload {
		fullPayload[i] = byte((i * 19) % 251)
	}
	expectedHash := sha256.Sum256(fullPayload)

	addr, clientTLS, cleanup := startH3Server(t, func(req *h3server.ServerRequest, res *h3server.ServerResponse) error {
		reqHash := sha256.Sum256(req.Body)
		assert.Equal(t, expectedHash, reqHash)

		res.StatusCode = status.OK
		res.Headers.Set("Content-Type", "application/octet-stream")
		res.Body = []byte("chunked-received-ok")

		return nil
	})
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	quicConn, err := quic.DialAddr(ctx, addr, clientTLS, quic.WithDatagrams(true))
	require.NoError(t, err)
	defer quicConn.CloseWithError(0, "")

	// Client sends unidirectional control stream with SETTINGS
	ctrlOut, err := quicConn.OpenUniStreamSync(ctx)
	require.NoError(t, err)
	var typeBuf [8]byte
	n := varint.Append(typeBuf[:0], coreh3.StreamTypeControl)
	_, err = ctrlOut.Write(n)
	require.NoError(t, err)
	st := &coreh3.Settings{}
	_, err = ctrlOut.Write(st.Encode())
	require.NoError(t, err)

	// Open bidirectional request stream
	reqStream, err := quicConn.OpenStreamSync(ctx)
	require.NoError(t, err)
	defer reqStream.Close()

	// 1. Write request HEADERS frame
	headers := []qpack.HeaderField{
		{Name: ":method", Value: "POST"},
		{Name: ":path", Value: "/chunked-stream"},
		{Name: ":scheme", Value: "https"},
		{Name: ":authority", Value: addr},
	}
	hBlock := qpack.NewEncoderWithDefaults(nil).EncodeHeaderList(0, headers, nil)
	var frameHdr [16]byte
	hdrBytes := varint.Append(frameHdr[:0], coreh3.FrameTypeHeaders)
	hdrBytes = varint.Append(hdrBytes, uint64(len(hBlock)))
	_, err = reqStream.Write(hdrBytes)
	require.NoError(t, err)
	_, err = reqStream.Write(hBlock)
	require.NoError(t, err)

	// 2. Write multiple discrete DATA frames (chunked streaming)
	for c := range numChunks {
		chunk := fullPayload[c*chunkSize : (c+1)*chunkSize]
		dataHdr := varint.Append(frameHdr[:0], coreh3.FrameTypeData)
		dataHdr = varint.Append(dataHdr, uint64(len(chunk)))
		_, err = reqStream.Write(dataHdr)
		require.NoError(t, err)
		_, err = reqStream.Write(chunk)
		require.NoError(t, err)
	}

	// Close write half (FIN)
	require.NoError(t, reqStream.Close())

	// Read response frames
	qr := varint.NewReader(reqStream)
	fType, err := varint.Read(qr)
	require.NoError(t, err)
	assert.Equal(t, coreh3.FrameTypeHeaders, fType)
	fLen, err := varint.Read(qr)
	require.NoError(t, err)
	respHdrBlock := make([]byte, fLen)
	_, err = io.ReadFull(reqStream, respHdrBlock)
	require.NoError(t, err)

	// Read response DATA frame
	dType, err := varint.Read(qr)
	require.NoError(t, err)
	assert.Equal(t, coreh3.FrameTypeData, dType)
	dLen, err := varint.Read(qr)
	require.NoError(t, err)
	respBody := make([]byte, dLen)
	_, err = io.ReadFull(reqStream, respBody)
	require.NoError(t, err)
	assert.Equal(t, "chunked-received-ok", string(respBody))
}

func TestH3_ServerChunkedResponse_MultiDataFrame(t *testing.T) {
	t.Parallel()

	numChunks := 6
	chunkSize := 32 * 1024 // 32 KB per chunk = 192 KB total
	fullPayload := make([]byte, numChunks*chunkSize)
	for i := range fullPayload {
		fullPayload[i] = byte((i * 23) % 251)
	}
	expectedHash := sha256.Sum256(fullPayload)

	serverTLS, clientTLS := generateTestTLSConfig(t)
	listener, err := quic.ListenAddr("127.0.0.1:0", serverTLS, quic.WithDatagrams(true))
	require.NoError(t, err)
	defer listener.Close()

	go func() {
		conn, aErr := listener.Accept(context.Background())
		if aErr != nil {
			return
		}

		ctrlOut, _ := conn.OpenUniStreamSync(context.Background())
		_, _ = ctrlOut.Write(varint.Append(nil, coreh3.StreamTypeControl))
		st := &coreh3.Settings{}
		_, _ = ctrlOut.Write(st.Encode())

		str, sErr := conn.AcceptStream(context.Background())
		if sErr != nil {
			return
		}
		defer str.Close()

		qr := varint.NewReader(str)
		fType, _ := varint.Read(qr)
		fLen, _ := varint.Read(qr)
		if fType == coreh3.FrameTypeHeaders && fLen > 0 {
			_, _ = io.CopyN(io.Discard, str, int64(fLen))
		}

		// 1. Response HEADERS
		respHeaders := []qpack.HeaderField{
			{Name: ":status", Value: "200"},
			{Name: "content-type", Value: "application/octet-stream"},
		}
		hBlock := qpack.NewEncoderWithDefaults(nil).EncodeHeaderList(0, respHeaders, nil)
		_, _ = str.Write(coreh3.AppendHeadersHeader(nil, uint64(len(hBlock))))
		_, _ = str.Write(hBlock)

		// 2. Response multiple DATA frames
		for c := range numChunks {
			chunk := fullPayload[c*chunkSize : (c+1)*chunkSize]
			_, _ = str.Write(coreh3.AppendDataHeader(nil, uint64(len(chunk))))
			_, _ = str.Write(chunk)
		}
	}()

	client, quicConn := dialH3Client(t, listener.Addr().String(), clientTLS)
	defer quicConn.CloseWithError(0, "")
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req := machhttp.AcquireRequest()
	resp := machhttp.AcquireResponse()
	defer machhttp.ReleaseRequest(req)
	defer machhttp.ReleaseResponse(resp)

	req.Header.SetMethod("GET")
	req.SetRequestURI("https://" + listener.Addr().String() + "/server-chunked")

	_, err = client.Do(ctx, req, resp, nil)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode())

	respHash := sha256.Sum256(resp.Body())
	assert.Equal(t, expectedHash, respHash)
}

// ============================================================================
// Suite 3: Stream Error Handling, Cancellations & Isolation
// ============================================================================

func TestH3_StreamReset_ClientCancellation(t *testing.T) {
	t.Parallel()

	var closeOnce sync.Once
	serverStartedProcessing := make(chan struct{})

	addr, clientTLS, cleanup := startH3Server(t, func(req *h3server.ServerRequest, res *h3server.ServerResponse) error {
		if req.Path == "/cancel-midflight" {
			closeOnce.Do(func() { close(serverStartedProcessing) })
			// Delay response to allow client to cancel context mid-flight
			time.Sleep(300 * time.Millisecond)
			res.StatusCode = status.OK
			res.Body = []byte("too-late")
			return nil
		}
		res.StatusCode = status.OK
		res.Body = []byte("ok")
		return nil
	})
	defer cleanup()

	client, quicConn := dialH3Client(t, addr, clientTLS)
	defer quicConn.CloseWithError(0, "")
	defer client.Close()

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		<-serverStartedProcessing
		cancel() // Cancel request context while server is processing
	}()

	req := machhttp.AcquireRequest()
	resp := machhttp.AcquireResponse()
	defer machhttp.ReleaseRequest(req)
	defer machhttp.ReleaseResponse(resp)

	req.Header.SetMethod("GET")
	req.SetRequestURI("https://" + addr + "/cancel-midflight")

	_, err := client.Do(ctx, req, resp, nil)
	assert.Error(t, err)

	// Invariant: The connection MUST remain healthy after a single stream cancellation.
	// A subsequent request on the same client connection MUST succeed!
	ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel2()

	req2 := machhttp.AcquireRequest()
	resp2 := machhttp.AcquireResponse()
	defer machhttp.ReleaseRequest(req2)
	defer machhttp.ReleaseResponse(resp2)

	req2.Header.SetMethod("GET")
	req2.SetRequestURI("https://" + addr + "/subsequent-success")

	_, err2 := client.Do(ctx2, req2, resp2, nil)
	require.NoError(t, err2)
	assert.Equal(t, 200, resp2.StatusCode())
}

func TestH3_StreamReset_ServerMessageError(t *testing.T) {
	t.Parallel()

	serverTLS, clientTLS := generateTestTLSConfig(t)
	listener, err := quic.ListenAddr("127.0.0.1:0", serverTLS, quic.WithDatagrams(true))
	require.NoError(t, err)
	defer listener.Close()

	go func() {
		conn, aErr := listener.Accept(context.Background())
		if aErr != nil {
			return
		}

		// Control stream
		ctrlOut, _ := conn.OpenUniStreamSync(context.Background())
		_, _ = ctrlOut.Write(varint.Append(nil, coreh3.StreamTypeControl))
		st := &coreh3.Settings{}
		_, _ = ctrlOut.Write(st.Encode())

		// First stream: abort with H3_MESSAGE_ERROR (0x010e)
		str1, sErr := conn.AcceptStream(context.Background())
		if sErr == nil {
			// Drain request headers
			qr := varint.NewReader(str1)
			_, _ = varint.Read(qr)
			fLen, _ := varint.Read(qr)
			_, _ = io.CopyN(io.Discard, str1, int64(fLen))

			// Reset stream with H3_MESSAGE_ERROR
			str1.CancelWrite(quic.StreamErrorCode(coreh3.ErrCodeH3MessageError))
			str1.CancelRead(quic.StreamErrorCode(coreh3.ErrCodeH3MessageError))
			_ = str1.Close()
		}

		// Second stream: respond normally with 200 OK
		str2, sErr2 := conn.AcceptStream(context.Background())
		if sErr2 == nil {
			qr2 := varint.NewReader(str2)
			_, _ = varint.Read(qr2)
			fLen2, _ := varint.Read(qr2)
			_, _ = io.CopyN(io.Discard, str2, int64(fLen2))

			respHeaders := []qpack.HeaderField{
				{Name: ":status", Value: "200"},
			}
			hBlock := qpack.NewEncoderWithDefaults(nil).EncodeHeaderList(0, respHeaders, nil)
			_, _ = str2.Write(coreh3.AppendHeadersHeader(nil, uint64(len(hBlock))))
			_, _ = str2.Write(hBlock)
			_, _ = str2.Write(coreh3.AppendDataHeader(nil, 9))
			_, _ = str2.Write([]byte("stream-ok"))
			_ = str2.Close()
		}
	}()

	client, quicConn := dialH3Client(t, listener.Addr().String(), clientTLS)
	defer quicConn.CloseWithError(0, "")
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Request 1: should receive stream reset error
	req1 := machhttp.AcquireRequest()
	resp1 := machhttp.AcquireResponse()
	defer machhttp.ReleaseRequest(req1)
	defer machhttp.ReleaseResponse(resp1)

	req1.Header.SetMethod("GET")
	req1.SetRequestURI("https://" + listener.Addr().String() + "/stream-fail")

	_, err1 := client.Do(ctx, req1, resp1, nil)
	assert.Error(t, err1, "first stream should error due to server reset")

	// Request 2: connection must remain open and handle subsequent request
	req2 := machhttp.AcquireRequest()
	resp2 := machhttp.AcquireResponse()
	defer machhttp.ReleaseRequest(req2)
	defer machhttp.ReleaseResponse(resp2)

	req2.Header.SetMethod("GET")
	req2.SetRequestURI("https://" + listener.Addr().String() + "/stream-ok")

	_, err2 := client.Do(ctx, req2, resp2, nil)
	require.NoError(t, err2, "subsequent stream must succeed")
	assert.Equal(t, 200, resp2.StatusCode())
	assert.Equal(t, "stream-ok", string(resp2.Body()))
}

func TestH3_ConcurrentStreams_MixedCancellations(t *testing.T) {
	t.Parallel()

	addr, clientTLS, cleanup := startH3Server(t, func(req *h3server.ServerRequest, res *h3server.ServerResponse) error {
		switch req.Path {
		case "/slow":
			time.Sleep(300 * time.Millisecond)
			res.StatusCode = status.OK
			res.Body = []byte("slow-done")
		default:
			res.StatusCode = status.OK
			res.Body = []byte("fast-done")
		}
		return nil
	})
	defer cleanup()

	client, quicConn := dialH3Client(t, addr, clientTLS)
	defer quicConn.CloseWithError(0, "")
	defer client.Close()

	totalRequests := 20
	var wg sync.WaitGroup
	fastSuccessCount := sync.WaitGroup{}

	var successCounter sync.Mutex
	successfulFastCount := 0

	for i := range totalRequests {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			req := machhttp.AcquireRequest()
			resp := machhttp.AcquireResponse()
			defer machhttp.ReleaseRequest(req)
			defer machhttp.ReleaseResponse(resp)

			if idx%2 == 0 {
				// Half the requests are slow with tight timeout -> cancelled
				ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
				defer cancel()

				req.Header.SetMethod("GET")
				req.SetRequestURI(fmt.Sprintf("https://%s/slow", addr))
				_, _ = client.Do(ctx, req, resp, nil)
			} else {
				// Half the requests are fast with generous timeout -> must succeed
				fastSuccessCount.Add(1)
				defer fastSuccessCount.Done()

				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()

				req.Header.SetMethod("GET")
				req.SetRequestURI(fmt.Sprintf("https://%s/fast/%d", addr, idx))

				_, err := client.Do(ctx, req, resp, nil)
				if err == nil && resp.StatusCode() == 200 && string(resp.Body()) == "fast-done" {
					successCounter.Lock()
					successfulFastCount++
					successCounter.Unlock()
				}
			}
		}(i)
	}

	wg.Wait()
	assert.Equal(t, totalRequests/2, successfulFastCount, "All fast requests must succeed despite concurrent cancellations")
}

// ============================================================================
// Suite 4: Graceful Connection Termination & Inflight Requests
// ============================================================================

func TestH3_GracefulTermination_ServerCloseWithInflight(t *testing.T) {
	t.Parallel()

	serverTLS, clientTLS := generateTestTLSConfig(t)
	listener, err := quic.ListenAddr("127.0.0.1:0", serverTLS, quic.WithDatagrams(true))
	require.NoError(t, err)
	defer listener.Close()

	handlerEntered := make(chan struct{}, 1)
	scCh := make(chan *h3server.ServerConn, 1)

	go func() {
		conn, aErr := listener.Accept(context.Background())
		if aErr != nil {
			return
		}

		handler := func(req *h3server.ServerRequest, res *h3server.ServerResponse) error {
			select {
			case handlerEntered <- struct{}{}:
			default:
			}
			time.Sleep(100 * time.Millisecond)
			res.StatusCode = status.OK
			res.Body = []byte("graceful-response")
			return nil
		}

		sc := h3server.NewServerConn(conn, handler)
		select {
		case scCh <- sc:
		default:
		}
		_ = sc.Serve()
	}()

	client, quicConn := dialH3Client(t, listener.Addr().String(), clientTLS)
	defer quicConn.CloseWithError(0, "")
	defer client.Close()

	// Start request in flight
	reqDone := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		req := machhttp.AcquireRequest()
		resp := machhttp.AcquireResponse()
		defer machhttp.ReleaseRequest(req)
		defer machhttp.ReleaseResponse(resp)

		req.Header.SetMethod("GET")
		req.SetRequestURI("https://" + listener.Addr().String() + "/inflight")

		_, err := client.Do(ctx, req, resp, nil)
		reqDone <- err
	}()

	// Wait for request to arrive at server handler, then gracefully close server
	select {
	case <-handlerEntered:
	case <-time.After(2 * time.Second):
	}

	select {
	case sc := <-scCh:
		_ = sc.Close()
	default:
	}

	// Request should complete or terminate without panic/deadlock
	select {
	case <-reqDone:
		// Completed cleanly
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for inflight request during server graceful close")
	}
}

func TestH3_GracefulTermination_ClientClose(t *testing.T) {
	t.Parallel()

	addr, clientTLS, cleanup := startH3Server(t, func(req *h3server.ServerRequest, res *h3server.ServerResponse) error {
		res.StatusCode = status.OK
		res.Body = []byte("before-close")
		return nil
	})
	defer cleanup()

	client, quicConn := dialH3Client(t, addr, clientTLS)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req := machhttp.AcquireRequest()
	resp := machhttp.AcquireResponse()
	defer machhttp.ReleaseRequest(req)
	defer machhttp.ReleaseResponse(resp)

	req.Header.SetMethod("GET")
	req.SetRequestURI("https://" + addr + "/pre-close")

	_, err := client.Do(ctx, req, resp, nil)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode())

	// Graceful client close
	err = client.Close()
	require.NoError(t, err)
	assert.True(t, client.IsClosed())

	_ = quicConn.CloseWithError(0, "")
}

// ============================================================================
// Suite 5: Control Stream & RFC 9114 Protocol Violations
// ============================================================================

func TestH3_ControlStream_RejectServerBidiStream(t *testing.T) {
	t.Parallel()

	serverTLS, clientTLS := generateTestTLSConfig(t)
	listener, err := quic.ListenAddr("127.0.0.1:0", serverTLS, quic.WithDatagrams(true))
	require.NoError(t, err)
	defer listener.Close()

	go func() {
		conn, aErr := listener.Accept(context.Background())
		if aErr != nil {
			return
		}

		// Server control stream
		ctrlOut, _ := conn.OpenUniStreamSync(context.Background())
		_, _ = ctrlOut.Write(varint.Append(nil, coreh3.StreamTypeControl))
		st := &coreh3.Settings{}
		_, _ = ctrlOut.Write(st.Encode())

		// RFC 9114 §6.1 violation: Server attempts to open a bidirectional stream to client
		bidiStr, bErr := conn.OpenStreamSync(context.Background())
		if bErr == nil {
			_, _ = bidiStr.Write([]byte("illegal-server-bidi-stream"))
			_ = bidiStr.Close()
		}
	}()

	client, quicConn := dialH3Client(t, listener.Addr().String(), clientTLS)
	defer quicConn.CloseWithError(0, "")
	defer client.Close()

	// Invariant: The client MUST terminate the connection with H3_STREAM_CREATION_ERROR (RFC 9114 §6.1).
	time.Sleep(100 * time.Millisecond)
	assert.True(t, client.IsClosed(), "Client must close connection upon receiving server-initiated bidi stream")
}

func TestH3_ControlStream_RejectClientPushStream(t *testing.T) {
	t.Parallel()

	serverTLS, clientTLS := generateTestTLSConfig(t)
	listener, err := quic.ListenAddr("127.0.0.1:0", serverTLS, quic.WithDatagrams(true))
	require.NoError(t, err)
	defer listener.Close()

	go func() {
		conn, aErr := listener.Accept(context.Background())
		if aErr != nil {
			return
		}

		sc := h3server.NewServerConn(conn, nil)
		_ = sc.Serve()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	quicConn, err := quic.DialAddr(ctx, listener.Addr().String(), clientTLS, quic.WithDatagrams(true))
	require.NoError(t, err)
	defer quicConn.CloseWithError(0, "")

	// RFC 9114 §6.2.2 violation: Client attempts to open push stream (type 0x01)
	pushStream, err := quicConn.OpenUniStreamSync(ctx)
	require.NoError(t, err)

	var typeBuf [8]byte
	n := varint.Append(typeBuf[:0], coreh3.StreamTypePush)
	_, err = pushStream.Write(n)
	require.NoError(t, err)

	// Invariant: Server MUST close connection with H3_STREAM_CREATION_ERROR (RFC 9114 §6.2.2)
	time.Sleep(100 * time.Millisecond)
	select {
	case <-quicConn.Context().Done():
		// Closed as expected
	case <-time.After(1 * time.Second):
		t.Fatal("server should have terminated connection on client push stream")
	}
}

func TestH3_ControlStream_RejectDuplicateControlStream(t *testing.T) {
	t.Parallel()

	serverTLS, clientTLS := generateTestTLSConfig(t)
	listener, err := quic.ListenAddr("127.0.0.1:0", serverTLS, quic.WithDatagrams(true))
	require.NoError(t, err)
	defer listener.Close()

	go func() {
		conn, aErr := listener.Accept(context.Background())
		if aErr != nil {
			return
		}

		sc := h3server.NewServerConn(conn, nil)
		_ = sc.Serve()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	quicConn, err := quic.DialAddr(ctx, listener.Addr().String(), clientTLS, quic.WithDatagrams(true))
	require.NoError(t, err)
	defer quicConn.CloseWithError(0, "")

	// 1. Send first valid control stream
	ctrl1, err := quicConn.OpenUniStreamSync(ctx)
	require.NoError(t, err)
	var typeBuf [8]byte
	n := varint.Append(typeBuf[:0], coreh3.StreamTypeControl)
	_, _ = ctrl1.Write(n)
	st := &coreh3.Settings{}
	_, _ = ctrl1.Write(st.Encode())

	// 2. RFC 9114 §6.2.1 violation: Send second duplicate control stream
	ctrl2, err := quicConn.OpenUniStreamSync(ctx)
	require.NoError(t, err)
	_, _ = ctrl2.Write(n)
	_, _ = ctrl2.Write(st.Encode())

	// Invariant: Server MUST terminate connection with H3_STREAM_CREATION_ERROR
	time.Sleep(100 * time.Millisecond)
	select {
	case <-quicConn.Context().Done():
		// Closed as expected
	case <-time.After(1 * time.Second):
		t.Fatal("server should have closed connection on duplicate control stream")
	}
}

func TestH3_CriticalStream_ClosedTriggersConnectionError(t *testing.T) {
	t.Parallel()

	serverTLS, clientTLS := generateTestTLSConfig(t)
	listener, err := quic.ListenAddr("127.0.0.1:0", serverTLS, quic.WithDatagrams(true))
	require.NoError(t, err)
	defer listener.Close()

	go func() {
		conn, aErr := listener.Accept(context.Background())
		if aErr != nil {
			return
		}

		sc := h3server.NewServerConn(conn, nil)
		_ = sc.Serve()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	quicConn, err := quic.DialAddr(ctx, listener.Addr().String(), clientTLS, quic.WithDatagrams(true))
	require.NoError(t, err)
	defer quicConn.CloseWithError(0, "")

	// Open control stream, send SETTINGS, then close it (RFC 9114 §6.2.1 violation: closing critical stream)
	ctrlStream, err := quicConn.OpenUniStreamSync(ctx)
	require.NoError(t, err)
	var typeBuf [8]byte
	n := varint.Append(typeBuf[:0], coreh3.StreamTypeControl)
	_, _ = ctrlStream.Write(n)
	st := &coreh3.Settings{}
	_, _ = ctrlStream.Write(st.Encode())

	// Abruptly close control stream
	_ = ctrlStream.Close()

	// Invariant: Server MUST close connection with H3_CLOSED_CRITICAL_STREAM
	time.Sleep(100 * time.Millisecond)
	select {
	case <-quicConn.Context().Done():
		// Successfully terminated
	case <-time.After(1 * time.Second):
		t.Fatal("server should have closed connection when critical stream closed")
	}
}

func TestH3_UnknownUnidirectionalStream_GracefullyIgnored(t *testing.T) {
	t.Parallel()

	addr, clientTLS, cleanup := startH3Server(t, func(req *h3server.ServerRequest, res *h3server.ServerResponse) error {
		res.StatusCode = status.OK
		res.Body = []byte("still-alive")
		return nil
	})
	defer cleanup()

	client, quicConn := dialH3Client(t, addr, clientTLS)
	defer quicConn.CloseWithError(0, "")
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// RFC 9114 §6.2: Unknown unidirectional stream type (e.g. 0x2b, GREASE or future extension)
	unknownStream, err := quicConn.OpenUniStreamSync(ctx)
	require.NoError(t, err)

	var typeBuf [8]byte
	n := varint.Append(typeBuf[:0], 0x2b)
	_, _ = unknownStream.Write(n)
	_, _ = unknownStream.Write(bytes.Repeat([]byte("unknown-stream-payload"), 50))
	_ = unknownStream.Close()

	// Invariant: The connection MUST remain intact and operational
	time.Sleep(50 * time.Millisecond)

	req := machhttp.AcquireRequest()
	resp := machhttp.AcquireResponse()
	defer machhttp.ReleaseRequest(req)
	defer machhttp.ReleaseResponse(resp)

	req.Header.SetMethod("GET")
	req.SetRequestURI("https://" + addr + "/ping")

	_, err = client.Do(ctx, req, resp, nil)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode())
	assert.Equal(t, "still-alive", string(resp.Body()))
}
