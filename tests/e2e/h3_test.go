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

	"github.com/lemon4ksan/foundation/borrow"
	"github.com/lemon4ksan/foundation/encoding/varint"
	"github.com/lemon4ksan/foundation/net/http/status"
	"github.com/lemon4ksan/foundation/net/qpack"
	"github.com/lemon4ksan/foundation/net/quic"
	"github.com/lemon4ksan/foundation/testing/assert"
	"github.com/lemon4ksan/foundation/testing/require"

	h3client "github.com/lemon4ksan/mach/client/h3"
	coreh3 "github.com/lemon4ksan/mach/proto/h3"
	machhttp "github.com/lemon4ksan/mach/proto/http"
	h3server "github.com/lemon4ksan/mach/server/h3"
)

// ============================================================================
// Tier 1: Feature Coverage (RFC 9114 / RFC 9204)
// ============================================================================

func TestH3_Tier1_QPACKHeaderCompression(t *testing.T) {
	t.Parallel()

	addr, clientTLS, cleanup := startH3Server(t, func(req *h3server.ServerRequest, res *h3server.ServerResponse) error {
		assert.Equal(t, "GET", req.Method)
		assert.Equal(t, "/qpack-test", req.Path)
		assert.Equal(t, "trace-qpack-999", req.Headers.Get("X-Trace-ID"))

		res.StatusCode = status.OK
		res.Headers.Set("Content-Type", "application/json")
		res.Headers.Set("X-Powered-By", "mach-h3")
		res.Body = []byte(`{"h3":"qpack-success"}`)

		return nil
	})
	defer cleanup()

	client, quicConn := dialH3Client(t, addr, clientTLS)
	defer quicConn.CloseWithError(0, "")
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req := machhttp.AcquireRequest()
	resp := machhttp.AcquireResponse()
	defer machhttp.ReleaseRequest(req)
	defer machhttp.ReleaseResponse(resp)

	req.Header.SetMethod("GET")
	req.SetRequestURI("https://" + addr + "/qpack-test")
	req.Header.Set("X-Trace-ID", "trace-qpack-999")

	_, err := client.Do(ctx, req, resp, nil)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode())
	assert.Equal(t, `{"h3":"qpack-success"}`, string(resp.Body()))
	assert.Equal(t, "mach-h3", string(resp.Header.Peek("X-Powered-By")))
}

func TestH3_Tier1_DATAFrameStreaming(t *testing.T) {
	t.Parallel()

	payloadSize := 16 * 1024
	payload := make([]byte, payloadSize)
	for i := range payload {
		payload[i] = byte((i * 13) % 256)
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

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req := machhttp.AcquireRequest()
	resp := machhttp.AcquireResponse()
	defer machhttp.ReleaseRequest(req)
	defer machhttp.ReleaseResponse(resp)

	req.Header.SetMethod("POST")
	req.SetRequestURI("https://" + addr + "/data-stream")
	req.SetBody(payload)

	_, err := client.Do(ctx, req, resp, nil)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode())

	respHash := sha256.Sum256(resp.Body())
	assert.Equal(t, expectedHash, respHash)
}

func TestH3_Tier1_MultiStreamMultiplexing(t *testing.T) {
	t.Parallel()

	addr, clientTLS, cleanup := startH3Server(t, func(req *h3server.ServerRequest, res *h3server.ServerResponse) error {
		res.StatusCode = status.OK
		res.Body = []byte("h3-ack-" + req.Path)

		return nil
	})
	defer cleanup()

	client, quicConn := dialH3Client(t, addr, clientTLS)
	defer quicConn.CloseWithError(0, "")
	defer client.Close()

	numStreams := 15
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
			req.SetRequestURI("https://" + addr + path)

			_, err := client.Do(ctx, req, resp, nil)
			if err != nil {
				errs[idx] = err
				return
			}

			if resp.StatusCode() != 200 || string(resp.Body()) != "h3-ack-"+path {
				errs[idx] = fmt.Errorf("unexpected status %d or body", resp.StatusCode())
			}
		}(i)
	}

	wg.Wait()
	for i, err := range errs {
		require.NoErrorf(t, err, "h3 stream %d failed", i)
	}
}

func TestH3_Tier1_TrailingHeaders(t *testing.T) {
	t.Parallel()

	serverTLS, clientTLS := generateTestTLSConfig(t)
	listener, err := quic.ListenAddr("127.0.0.1:0", serverTLS, quic.WithDatagrams(true))
	require.NoError(t, err)
	defer listener.Close()

	// Custom server that returns HEADERS, DATA, and trailing HEADERS
	go func() {
		conn, aErr := listener.Accept(context.Background())
		if aErr != nil {
			return
		}

		// Read client control stream and send server control stream
		ctrlOut, _ := conn.OpenUniStreamSync(context.Background())
		_, _ = ctrlOut.Write(varint.Append(nil, coreh3.StreamTypeControl))
		st := &coreh3.Settings{}
		_, _ = ctrlOut.Write(st.Encode())

		// Accept bidirectional request stream
		str, sErr := conn.AcceptStream(context.Background())
		if sErr != nil {
			return
		}
		defer str.Close()

		// Read request frames
		qr := varint.NewReader(str)
		fType, _ := varint.Read(qr)
		fLen, _ := varint.Read(qr)
		if fType == coreh3.FrameTypeHeaders && fLen > 0 {
			_, _ = io.CopyN(io.Discard, str, int64(fLen))
		}

		// 1. Response HEADERS
		respHeaders := []qpack.HeaderField{
			{Name: ":status", Value: "200"},
			{Name: "content-type", Value: "application/grpc"},
		}
		hBlock := qpack.NewEncoderWithDefaults(nil).EncodeHeaderList(0, respHeaders, nil)
		_, _ = str.Write(coreh3.AppendHeadersHeader(nil, uint64(len(hBlock))))
		_, _ = str.Write(hBlock)

		// 2. Response DATA
		body := []byte("grpc-payload")
		_, _ = str.Write(coreh3.AppendDataHeader(nil, uint64(len(body))))
		_, _ = str.Write(body)

		// 3. Trailing HEADERS
		trailers := []qpack.HeaderField{
			{Name: "grpc-status", Value: "0"},
			{Name: "grpc-message", Value: "OK"},
		}
		tBlock := qpack.NewEncoderWithDefaults(nil).EncodeHeaderList(0, trailers, nil)
		_, _ = str.Write(coreh3.AppendHeadersHeader(nil, uint64(len(tBlock))))
		_, _ = str.Write(tBlock)
	}()

	client, quicConn := dialH3Client(t, listener.Addr().String(), clientTLS)
	defer quicConn.CloseWithError(0, "")
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req := machhttp.AcquireRequest()
	resp := machhttp.AcquireResponse()
	defer machhttp.ReleaseRequest(req)
	defer machhttp.ReleaseResponse(resp)

	req.Header.SetMethod("POST")
	req.SetRequestURI("https://" + listener.Addr().String() + "/grpc")

	trailers, err := client.Do(ctx, req, resp, nil)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode())
	assert.Equal(t, "grpc-payload", string(resp.Body()))
	require.NotNil(t, trailers)
	assert.Equal(t, []string{"0"}, trailers["grpc-status"])
	assert.Equal(t, []string{"OK"}, trailers["grpc-message"])
}

func TestH3_Tier1_ScopedMemoryBorrowing(t *testing.T) {
	t.Parallel()

	addr, clientTLS, cleanup := startH3Server(t, func(req *h3server.ServerRequest, res *h3server.ServerResponse) error {
		res.StatusCode = status.OK
		res.Body = []byte("scoped-borrow-payload")

		return nil
	})
	defer cleanup()

	client, quicConn := dialH3Client(t, addr, clientTLS)
	defer quicConn.CloseWithError(0, "")
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	scope := borrow.NewScope()
	defer scope.Release()

	req := machhttp.AcquireRequest()
	resp := machhttp.AcquireResponse()
	defer machhttp.ReleaseRequest(req)
	defer machhttp.ReleaseResponse(resp)

	req.Header.SetMethod("GET")
	req.SetRequestURI("https://" + addr + "/scoped")

	_, err := client.DoScoped(ctx, req, resp, nil, scope)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode())
	assert.Equal(t, "scoped-borrow-payload", string(resp.Body()))
}

func TestH3_Tier1_SettingsExchange(t *testing.T) {
	t.Parallel()

	addr, clientTLS, cleanup := startH3Server(t, func(req *h3server.ServerRequest, res *h3server.ServerResponse) error {
		res.StatusCode = status.OK
		return nil
	})
	defer cleanup()

	client, quicConn := dialH3Client(t, addr, clientTLS)
	defer quicConn.CloseWithError(0, "")
	defer client.Close()

	// Wait briefly for unidirectional settings exchange to settle
	time.Sleep(50 * time.Millisecond)
	assert.False(t, client.IsClosed())
}

// ============================================================================
// Tier 2: Boundary & Corner Cases (RFC 9114)
// ============================================================================

func TestH3_Tier2_ZeroLengthDATA(t *testing.T) {
	t.Parallel()

	addr, clientTLS, cleanup := startH3Server(t, func(req *h3server.ServerRequest, res *h3server.ServerResponse) error {
		assert.Equal(t, 0, len(req.Body))
		res.StatusCode = status.NoContent
		res.Body = nil

		return nil
	})
	defer cleanup()

	client, quicConn := dialH3Client(t, addr, clientTLS)
	defer quicConn.CloseWithError(0, "")
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req := machhttp.AcquireRequest()
	resp := machhttp.AcquireResponse()
	defer machhttp.ReleaseRequest(req)
	defer machhttp.ReleaseResponse(resp)

	req.Header.SetMethod("POST")
	req.SetRequestURI("https://" + addr + "/zero-data")

	_, err := client.Do(ctx, req, resp, nil)
	require.NoError(t, err)
	assert.Equal(t, 204, resp.StatusCode())
	assert.Equal(t, 0, len(resp.Body()))
}

func TestH3_Tier2_UnknownFrameTypesIgnored(t *testing.T) {
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
		}
		hBlock := qpack.NewEncoderWithDefaults(nil).EncodeHeaderList(0, respHeaders, nil)
		_, _ = str.Write(coreh3.AppendHeadersHeader(nil, uint64(len(hBlock))))
		_, _ = str.Write(hBlock)

		// 2. Unknown Frame (RFC 9114 §7.2.8): type 0x21 with 8-byte payload
		var unknownHdr [16]byte
		n := varint.Append(unknownHdr[:0], 0x21)
		n = varint.Append(n, 8)
		_, _ = str.Write(n)
		_, _ = str.Write([]byte("ignoreME"))

		// 3. Valid Response DATA
		body := []byte("survived-unknown-frame")
		_, _ = str.Write(coreh3.AppendDataHeader(nil, uint64(len(body))))
		_, _ = str.Write(body)
	}()

	client, quicConn := dialH3Client(t, listener.Addr().String(), clientTLS)
	defer quicConn.CloseWithError(0, "")
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req := machhttp.AcquireRequest()
	resp := machhttp.AcquireResponse()
	defer machhttp.ReleaseRequest(req)
	defer machhttp.ReleaseResponse(resp)

	req.Header.SetMethod("GET")
	req.SetRequestURI("https://" + listener.Addr().String() + "/unknown-frame")

	_, err = client.Do(ctx, req, resp, nil)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode())
	assert.Equal(t, "survived-unknown-frame", string(resp.Body()))
}

func TestH3_Tier2_ReservedH2SettingsRejected(t *testing.T) {
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

		// Server control stream sends reserved H2 setting ID 0x02 (SETTINGS_ENABLE_PUSH, RFC 9114 §7.2.4.1)
		ctrlOut, _ := conn.OpenUniStreamSync(context.Background())
		_, _ = ctrlOut.Write(varint.Append(nil, coreh3.StreamTypeControl))

		var payload [8]byte
		n := varint.Append(payload[:0], 0x02) // reserved H2 setting ID
		n = varint.Append(n, 1)

		var frameHdr [16]byte
		fHdr := varint.Append(frameHdr[:0], coreh3.FrameTypeSettings)
		fHdr = varint.Append(fHdr, uint64(len(n)))

		_, _ = ctrlOut.Write(fHdr)
		_, _ = ctrlOut.Write(n)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	quicConn, err := quic.DialAddr(ctx, listener.Addr().String(), clientTLS, quic.WithDatagrams(true))
	require.NoError(t, err)
	defer quicConn.CloseWithError(0, "")

	cc, err := h3client.NewClientConn(quicConn, nil)
	if err == nil {
		defer cc.Close()
		// Connection should close with H3_SETTINGS_ERROR shortly
		time.Sleep(100 * time.Millisecond)
		assert.True(t, cc.IsClosed())
	}
}

func TestH3_Tier2_ControlStreamMissingInitialSettings(t *testing.T) {
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

		// Control stream sends DATA frame instead of SETTINGS (RFC 9114 §7.2.4 violation: H3_MISSING_SETTINGS)
		ctrlOut, _ := conn.OpenUniStreamSync(context.Background())
		_, _ = ctrlOut.Write(varint.Append(nil, coreh3.StreamTypeControl))

		var frameHdr [16]byte
		fHdr := varint.Append(frameHdr[:0], coreh3.FrameTypeData)
		fHdr = varint.Append(fHdr, 4)
		_, _ = ctrlOut.Write(fHdr)
		_, _ = ctrlOut.Write([]byte("oops"))
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	quicConn, err := quic.DialAddr(ctx, listener.Addr().String(), clientTLS, quic.WithDatagrams(true))
	require.NoError(t, err)
	defer quicConn.CloseWithError(0, "")

	cc, err := h3client.NewClientConn(quicConn, nil)
	if err == nil {
		defer cc.Close()
		// Connection should close with H3_MISSING_SETTINGS shortly
		time.Sleep(100 * time.Millisecond)
		assert.True(t, cc.IsClosed())
	}
}

func TestH3_Tier2_OversizedHeaders(t *testing.T) {
	t.Parallel()

	addr, clientTLS, cleanup := startH3Server(t, func(req *h3server.ServerRequest, res *h3server.ServerResponse) error {
		res.StatusCode = status.OK
		res.Body = []byte("large-headers-received")

		return nil
	})
	defer cleanup()

	client, quicConn := dialH3Client(t, addr, clientTLS)
	defer quicConn.CloseWithError(0, "")
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req := machhttp.AcquireRequest()
	resp := machhttp.AcquireResponse()
	defer machhttp.ReleaseRequest(req)
	defer machhttp.ReleaseResponse(resp)

	req.Header.SetMethod("GET")
	req.SetRequestURI("https://" + addr + "/large-hdrs")

	// 15 headers each with 200 chars (~3KB)
	pad := bytes.Repeat([]byte("h"), 200)
	for i := 0; i < 15; i++ {
		req.Header.Set(fmt.Sprintf("X-H3-Pad-%02d", i), string(pad))
	}

	_, err := client.Do(ctx, req, resp, nil)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode())
	assert.Equal(t, "large-headers-received", string(resp.Body()))
}

func TestH3_Tier2_OversizedPayloads(t *testing.T) {
	t.Parallel()

	// 48KB payload exercises 32KB dataBufPool chunking
	payloadSize := 48 * 1024
	payload := make([]byte, payloadSize)
	for i := range payload {
		payload[i] = byte(i % 251)
	}
	expectedHash := sha256.Sum256(payload)

	addr, clientTLS, cleanup := startH3Server(t, func(req *h3server.ServerRequest, res *h3server.ServerResponse) error {
		reqHash := sha256.Sum256(req.Body)
		assert.Equal(t, expectedHash, reqHash)

		res.StatusCode = status.OK
		res.Body = req.Body

		return nil
	})
	defer cleanup()

	client, quicConn := dialH3Client(t, addr, clientTLS)
	defer quicConn.CloseWithError(0, "")
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req := machhttp.AcquireRequest()
	resp := machhttp.AcquireResponse()
	defer machhttp.ReleaseRequest(req)
	defer machhttp.ReleaseResponse(resp)

	req.Header.SetMethod("POST")
	req.SetRequestURI("https://" + addr + "/oversized-payload")
	req.SetBody(payload)

	_, err := client.Do(ctx, req, resp, nil)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode())

	respHash := sha256.Sum256(resp.Body())
	assert.Equal(t, expectedHash, respHash)
}

// ============================================================================
// Tier 3: Cross-Feature Combinations
// ============================================================================

func TestH3_Tier3_ConcurrentStreamsWithTrailers(t *testing.T) {
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

		ctrlOut, _ := conn.OpenUniStreamSync(context.Background())
		_, _ = ctrlOut.Write(varint.Append(nil, coreh3.StreamTypeControl))
		st := &coreh3.Settings{}
		_, _ = ctrlOut.Write(st.Encode())

		for {
			str, sErr := conn.AcceptStream(context.Background())
			if sErr != nil {
				return
			}

			go func(s *quic.Stream) {
				defer s.Close()

				qr := varint.NewReader(s)
				fType, _ := varint.Read(qr)
				fLen, _ := varint.Read(qr)
				if fType == coreh3.FrameTypeHeaders && fLen > 0 {
					_, _ = io.CopyN(io.Discard, s, int64(fLen))
				}

				// Response HEADERS
				respHeaders := []qpack.HeaderField{
					{Name: ":status", Value: "200"},
				}
				hBlock := qpack.NewEncoderWithDefaults(nil).EncodeHeaderList(0, respHeaders, nil)
				_, _ = s.Write(coreh3.AppendHeadersHeader(nil, uint64(len(hBlock))))
				_, _ = s.Write(hBlock)

				// Response DATA
				_, _ = s.Write(coreh3.AppendDataHeader(nil, 4))
				_, _ = s.Write([]byte("body"))

				// Trailing HEADERS
				streamID := s.StreamID()
				trailers := []qpack.HeaderField{
					{Name: "stream-id-trailer", Value: fmt.Sprintf("%d", streamID)},
				}
				tBlock := qpack.NewEncoderWithDefaults(nil).EncodeHeaderList(0, trailers, nil)
				_, _ = s.Write(coreh3.AppendHeadersHeader(nil, uint64(len(tBlock))))
				_, _ = s.Write(tBlock)
			}(str)
		}
	}()

	client, quicConn := dialH3Client(t, listener.Addr().String(), clientTLS)
	defer quicConn.CloseWithError(0, "")
	defer client.Close()

	concurrency := 5
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
			req.SetRequestURI(fmt.Sprintf("https://%s/trailer/%d", listener.Addr().String(), idx))

			trailers, err := client.Do(ctx, req, resp, nil)
			if err != nil {
				errs[idx] = err
				return
			}

			if resp.StatusCode() != 200 || string(resp.Body()) != "body" {
				errs[idx] = fmt.Errorf("unexpected status %d or body", resp.StatusCode())
				return
			}

			if len(trailers["stream-id-trailer"]) == 0 {
				errs[idx] = fmt.Errorf("missing trailer on worker %d", idx)
			}
		}(i)
	}

	wg.Wait()
	for i, err := range errs {
		require.NoErrorf(t, err, "trailer worker %d failed", i)
	}
}

func TestH3_Tier3_ScopedBorrowWithLargeMultiChunkPayload(t *testing.T) {
	t.Parallel()

	payloadSize := 64 * 1024
	payload := make([]byte, payloadSize)
	for i := range payload {
		payload[i] = byte(i % 249)
	}
	expectedHash := sha256.Sum256(payload)

	addr, clientTLS, cleanup := startH3Server(t, func(req *h3server.ServerRequest, res *h3server.ServerResponse) error {
		res.StatusCode = status.OK
		res.Body = req.Body

		return nil
	})
	defer cleanup()

	client, quicConn := dialH3Client(t, addr, clientTLS)
	defer quicConn.CloseWithError(0, "")
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	scope := borrow.NewScope()
	defer scope.Release()

	req := machhttp.AcquireRequest()
	resp := machhttp.AcquireResponse()
	defer machhttp.ReleaseRequest(req)
	defer machhttp.ReleaseResponse(resp)

	req.Header.SetMethod("POST")
	req.SetRequestURI("https://" + addr + "/scoped-large")
	req.SetBody(payload)

	_, err := client.DoScoped(ctx, req, resp, nil, scope)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode())

	respHash := sha256.Sum256(resp.Body())
	assert.Equal(t, expectedHash, respHash)
}

func TestH3_Tier3_Informational100ContinueThenFinalResponse(t *testing.T) {
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

		// 1. Informational 100 Continue HEADERS frame (RFC 9114 §4.1)
		hdrs100 := []qpack.HeaderField{
			{Name: ":status", Value: "100"},
		}
		b100 := qpack.NewEncoderWithDefaults(nil).EncodeHeaderList(0, hdrs100, nil)
		_, _ = str.Write(coreh3.AppendHeadersHeader(nil, uint64(len(b100))))
		_, _ = str.Write(b100)

		// 2. Final 200 OK HEADERS frame
		hdrs200 := []qpack.HeaderField{
			{Name: ":status", Value: "200"},
		}
		b200 := qpack.NewEncoderWithDefaults(nil).EncodeHeaderList(0, hdrs200, nil)
		_, _ = str.Write(coreh3.AppendHeadersHeader(nil, uint64(len(b200))))
		_, _ = str.Write(b200)

		// 3. Response DATA frame
		body := []byte("payload-after-continue")
		_, _ = str.Write(coreh3.AppendDataHeader(nil, uint64(len(body))))
		_, _ = str.Write(body)
	}()

	client, quicConn := dialH3Client(t, listener.Addr().String(), clientTLS)
	defer quicConn.CloseWithError(0, "")
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req := machhttp.AcquireRequest()
	resp := machhttp.AcquireResponse()
	defer machhttp.ReleaseRequest(req)
	defer machhttp.ReleaseResponse(resp)

	req.Header.SetMethod("GET")
	req.SetRequestURI("https://" + listener.Addr().String() + "/continue-flow")

	_, err = client.Do(ctx, req, resp, nil)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode())
	assert.Equal(t, "payload-after-continue", string(resp.Body()))
}

func TestH3_Tier3_ContextCancellationDuringActiveRead(t *testing.T) {
	t.Parallel()

	addr, clientTLS, cleanup := startH3Server(t, func(req *h3server.ServerRequest, res *h3server.ServerResponse) error {
		time.Sleep(500 * time.Millisecond)
		res.StatusCode = status.OK
		return nil
	})
	defer cleanup()

	client, quicConn := dialH3Client(t, addr, clientTLS)
	defer quicConn.CloseWithError(0, "")
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	req := machhttp.AcquireRequest()
	resp := machhttp.AcquireResponse()
	defer machhttp.ReleaseRequest(req)
	defer machhttp.ReleaseResponse(resp)

	req.Header.SetMethod("GET")
	req.SetRequestURI("https://" + addr + "/cancel")

	_, err := client.Do(ctx, req, resp, nil)
	assert.Error(t, err)
}

// ============================================================================
// Tier 4: Real-World Application Scenarios
// ============================================================================

func TestH3_Tier4_HighConcurrencyBurst(t *testing.T) {
	t.Parallel()

	addr, clientTLS, cleanup := startH3Server(t, func(req *h3server.ServerRequest, res *h3server.ServerResponse) error {
		res.StatusCode = status.OK
		res.Body = []byte("h3-burst-ok")

		return nil
	})
	defer cleanup()

	client, quicConn := dialH3Client(t, addr, clientTLS)
	defer quicConn.CloseWithError(0, "")
	defer client.Close()

	concurrency := 50
	var wg sync.WaitGroup
	errs := make([]error, concurrency)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()

			req := machhttp.AcquireRequest()
			resp := machhttp.AcquireResponse()
			defer machhttp.ReleaseRequest(req)
			defer machhttp.ReleaseResponse(resp)

			req.Header.SetMethod("GET")
			req.SetRequestURI(fmt.Sprintf("https://%s/burst/%d", addr, idx))

			_, err := client.Do(ctx, req, resp, nil)
			if err != nil {
				errs[idx] = err
				return
			}

			if resp.StatusCode() != 200 || string(resp.Body()) != "h3-burst-ok" {
				errs[idx] = fmt.Errorf("unexpected status %d or body", resp.StatusCode())
			}
		}(i)
	}

	wg.Wait()
	for i, err := range errs {
		require.NoErrorf(t, err, "h3 burst worker %d failed", i)
	}
}

func TestH3_Tier4_MultiChunkLargePayloadTransfer(t *testing.T) {
	t.Parallel()

	payloadSize := 128 * 1024
	payload := make([]byte, payloadSize)
	for i := range payload {
		payload[i] = byte((i * 17) % 251)
	}
	expectedHash := sha256.Sum256(payload)

	addr, clientTLS, cleanup := startH3Server(t, func(req *h3server.ServerRequest, res *h3server.ServerResponse) error {
		reqHash := sha256.Sum256(req.Body)
		assert.Equal(t, expectedHash, reqHash)

		res.StatusCode = status.OK
		res.Body = req.Body

		return nil
	})
	defer cleanup()

	client, quicConn := dialH3Client(t, addr, clientTLS)
	defer quicConn.CloseWithError(0, "")
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req := machhttp.AcquireRequest()
	resp := machhttp.AcquireResponse()
	defer machhttp.ReleaseRequest(req)
	defer machhttp.ReleaseResponse(resp)

	req.Header.SetMethod("POST")
	req.SetRequestURI("https://" + addr + "/large-transfer-128k")
	req.SetBody(payload)

	_, err := client.Do(ctx, req, resp, nil)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode())

	respHash := sha256.Sum256(resp.Body())
	assert.Equal(t, expectedHash, respHash)
}
