// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h2_test

import (
	"bufio"
	"context"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lemon4ksan/foundation/testing/assert"
	"github.com/lemon4ksan/foundation/testing/require"

	h2client "github.com/lemon4ksan/mach/client/h2"
	"github.com/lemon4ksan/mach/proto/codec/hpack"
	coreh2 "github.com/lemon4ksan/mach/proto/h2"
	h1 "github.com/lemon4ksan/mach/proto/http"
)

func setupClientServer(t *testing.T) (net.Conn, net.Conn, func()) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	type acceptRes struct {
		conn net.Conn
		err  error
	}
	ch := make(chan acceptRes, 1)
	go func() {
		c, aErr := ln.Accept()
		ch <- acceptRes{conn: c, err: aErr}
	}()

	clientConn, err := net.Dial("tcp", ln.Addr().String())
	require.NoError(t, err)

	res := <-ch
	require.NoError(t, res.err)
	serverConn := res.conn

	cleanup := func() {
		_ = clientConn.Close()
		_ = serverConn.Close()
		_ = ln.Close()
	}

	return clientConn, serverConn, cleanup
}

// TestClient_RST_STREAM_Race_WindowWaitDeadlock tests the race condition where
// client writeLoop is blocked waiting for flow-control window (waitForWindowUpdate)
// and the server resets the stream with RST_STREAM.
// On HEAD: c.finish does NOT wake up c.windowCond, so writeLoop hangs permanently.
func TestClient_RST_STREAM_Race_WindowWaitDeadlock(t *testing.T) {
	clientConn, serverConn, cleanup := setupClientServer(t)
	defer cleanup()

	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		br := bufio.NewReader(serverConn)
		bw := bufio.NewWriter(serverConn)

		// 1. Read preface
		if !coreh2.ReadPreface(br) {
			t.Errorf("server: invalid preface")
			return
		}

		// 2. Read client SETTINGS & WINDOW_UPDATE
		_, _ = coreh2.ReadFrameFrom(br) // SETTINGS
		_, _ = coreh2.ReadFrameFrom(br) // WINDOW_UPDATE

		// 3. Send server SETTINGS with INITIAL_WINDOW_SIZE = 50 bytes
		st := &coreh2.Settings{}
		st.Reset()
		st.SetMaxWindowSize(50) // only 50 bytes stream window!
		stFrame := coreh2.AcquireFrameHeader()
		stFrame.SetBody(st)
		_, _ = stFrame.WriteTo(bw)
		_ = bw.Flush()
		coreh2.ReleaseFrameHeader(stFrame)

		// 4. Read client SETTINGS ACK
		_, _ = coreh2.ReadFrameFrom(br)

		// 5. Send SETTINGS ACK
		ackFr := coreh2.AcquireFrameHeader()
		ackSt := coreh2.AcquireFrame(coreh2.FrameSettings).(*coreh2.Settings)
		ackSt.SetAck(true)
		ackFr.SetBody(ackSt)
		_, _ = ackFr.WriteTo(bw)
		_ = bw.Flush()
		coreh2.ReleaseFrameHeader(ackFr)

		// 6. Read client request HEADERS
		reqHdrFr, err := coreh2.ReadFrameFrom(br)
		if err != nil {
			return
		}
		streamID := reqHdrFr.Stream()
		coreh2.ReleaseFrameHeader(reqHdrFr)

		// 7. Read first DATA frame (up to 50 bytes)
		reqDataFr, err := coreh2.ReadFrameFrom(br)
		if err != nil {
			return
		}
		coreh2.ReleaseFrameHeader(reqDataFr)

		// Stream flow control window is now exhausted (50 - 50 = 0).
		// Client writeLoop will block in waitForWindowUpdate!
		time.Sleep(30 * time.Millisecond)

		// 8. Server transmits RST_STREAM(streamID, CANCEL)
		rstFr := coreh2.AcquireFrameHeader()
		rstFr.SetStream(streamID)
		rst := coreh2.AcquireFrame(coreh2.FrameResetStream).(*coreh2.RstStream)
		rst.SetCode(coreh2.StreamCanceled)
		rstFr.SetBody(rst)
		_, _ = rstFr.WriteTo(bw)
		_ = bw.Flush()
		coreh2.ReleaseFrameHeader(rstFr)

		// 9. Read req2 HEADERS with timeout
		_ = serverConn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		req2Hdr, rErr := coreh2.ReadFrameFrom(br)
		if rErr != nil {
			return
		}
		s2 := req2Hdr.Stream()
		coreh2.ReleaseFrameHeader(req2Hdr)

		// Respond 200 OK to req2
		enc := hpack.AcquireHPACK()
		defer hpack.ReleaseHPACK(enc)
		hf := hpack.AcquireHeaderField()
		defer hpack.ReleaseHeaderField(hf)
		hf.Set(":status", "200")
		h2Payload := enc.AppendHeader(nil, hf, false)

		r2FH := coreh2.AcquireFrameHeader()
		r2FH.SetStream(s2)
		r2FH.SetFlags(coreh2.FlagEndHeaders | coreh2.FlagEndStream)
		r2H := coreh2.AcquireFrame(coreh2.FrameHeaders).(*coreh2.Headers)
		r2H.SetEndHeaders(true)
		r2H.SetEndStream(true)
		r2H.SetHeaders(h2Payload)
		r2FH.SetBody(r2H)
		_, _ = r2FH.WriteTo(bw)
		_ = bw.Flush()
		coreh2.ReleaseFrameHeader(r2FH)
	}()

	client := h2client.NewConn(clientConn, h2client.ConnOpts{})
	require.NoError(t, client.Handshake())
	defer client.Close()

	req := h1.AcquireRequest()
	defer h1.ReleaseRequest(req)
	resp := h1.AcquireResponse()
	defer h1.ReleaseResponse(resp)

	req.Header.SetMethod("POST")
	req.SetRequestURI("http://example.com/upload")
	body := make([]byte, 500)
	req.SetBody(body)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- client.Do(ctx, req, resp)
	}()

	select {
	case err := <-done:
		assert.Error(t, err)
	case <-time.After(800 * time.Millisecond):
		t.Fatal("DEADLOCK: client.Do timed out on RST_STREAM race!")
	}

	// Try sending a second request on the same connection.
	// If writeLoop was deadlocked in waitForWindowUpdate, it can NEVER process req2!
	req2 := h1.AcquireRequest()
	defer h1.ReleaseRequest(req2)
	resp2 := h1.AcquireResponse()
	defer h1.ReleaseResponse(resp2)
	req2.Header.SetMethod("GET")
	req2.SetRequestURI("http://example.com/check")

	ctx2, cancel2 := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel2()
	err2 := client.Do(ctx2, req2, resp2)
	assert.NoError(t, err2, "subsequent request must not deadlock writeLoop")

	<-serverDone
}

// TestClient_ClosedStream_DATA_ReplenishesConnectionWindow verifies RFC 9113 §5.2.2:
// "DATA frames received on a closed stream count toward the connection flow-control window,
// even if the stream has been closed or reset. Even though the data is discarded, the recipient
// MUST update the connection flow-control window."
func TestClient_ClosedStream_DATA_ReplenishesConnectionWindow(t *testing.T) {
	clientConn, serverConn, cleanup := setupClientServer(t)
	defer cleanup()

	var gotConnWindowUpdate atomic.Bool

	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		br := bufio.NewReader(serverConn)
		bw := bufio.NewWriter(serverConn)

		_ = coreh2.ReadPreface(br)
		_, _ = coreh2.ReadFrameFrom(br) // SETTINGS
		_, _ = coreh2.ReadFrameFrom(br) // WINDOW_UPDATE

		// Server SETTINGS
		st := &coreh2.Settings{}
		st.Reset()
		stFrame := coreh2.AcquireFrameHeader()
		stFrame.SetBody(st)
		_, _ = stFrame.WriteTo(bw)
		_ = bw.Flush()
		coreh2.ReleaseFrameHeader(stFrame)

		_, _ = coreh2.ReadFrameFrom(br) // SETTINGS ACK

		// Send SETTINGS ACK
		ackFr := coreh2.AcquireFrameHeader()
		ackSt := coreh2.AcquireFrame(coreh2.FrameSettings).(*coreh2.Settings)
		ackSt.SetAck(true)
		ackFr.SetBody(ackSt)
		_, _ = ackFr.WriteTo(bw)
		_ = bw.Flush()
		coreh2.ReleaseFrameHeader(ackFr)

		// Read client request HEADERS
		reqHdrFr, err := coreh2.ReadFrameFrom(br)
		if err != nil {
			return
		}
		streamID := reqHdrFr.Stream()
		coreh2.ReleaseFrameHeader(reqHdrFr)

		// Reset stream immediately
		rstFr := coreh2.AcquireFrameHeader()
		rstFr.SetStream(streamID)
		rst := coreh2.AcquireFrame(coreh2.FrameResetStream).(*coreh2.RstStream)
		rst.SetCode(coreh2.StreamCanceled)
		rstFr.SetBody(rst)
		_, _ = rstFr.WriteTo(bw)
		_ = bw.Flush()
		coreh2.ReleaseFrameHeader(rstFr)

		// Stream is now closed on client!
		// Now server sends a large batch of in-flight DATA frames (8MB > 7.8MB threshold)
		// on the closed stream to exhaust receive window!
		largeData := make([]byte, 16384)
		for range 500 { // 500 * 16384 = ~8.1MB
			dFr := coreh2.AcquireFrameHeader()
			dFr.SetStream(streamID)
			d := coreh2.AcquireFrame(coreh2.FrameData).(*coreh2.Data)
			d.SetData(largeData)
			dFr.SetBody(d)
			_, _ = dFr.WriteTo(bw)
			coreh2.ReleaseFrameHeader(dFr)
		}
		_ = bw.Flush()

		// Read frames from client to check if WINDOW_UPDATE on stream 0 is emitted
		_ = serverConn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		for range 50 {
			fr, rErr := coreh2.ReadFrameFrom(br)
			if rErr != nil {
				break
			}
			if fr.Type() == coreh2.FrameWindowUpdate && fr.Stream() == 0 {
				gotConnWindowUpdate.Store(true)
				coreh2.ReleaseFrameHeader(fr)
				break
			}
			coreh2.ReleaseFrameHeader(fr)
		}
	}()

	client := h2client.NewConn(clientConn, h2client.ConnOpts{})
	require.NoError(t, client.Handshake())
	defer client.Close()

	req := h1.AcquireRequest()
	defer h1.ReleaseRequest(req)
	resp := h1.AcquireResponse()
	defer h1.ReleaseResponse(resp)

	req.Header.SetMethod("GET")
	req.SetRequestURI("http://example.com/test")

	_ = client.Do(context.Background(), req, resp)

	<-serverDone

	assert.True(t, gotConnWindowUpdate.Load(), "client MUST emit WINDOW_UPDATE(0) for DATA on closed stream (RFC 9113 §5.2.2)")
}

// TestClient_GOAWAY_GracefulShutdown_InFlightStreams verifies RFC 9113 §6.8:
// GOAWAY allows finishing processing of previously established in-flight streams.
// Client must NOT immediately abort streams with ID <= lastStreamID.
func TestClient_GOAWAY_GracefulShutdown_InFlightStreams(t *testing.T) {
	clientConn, serverConn, cleanup := setupClientServer(t)
	defer cleanup()

	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		br := bufio.NewReader(serverConn)
		bw := bufio.NewWriter(serverConn)

		_ = coreh2.ReadPreface(br)
		_, _ = coreh2.ReadFrameFrom(br) // SETTINGS
		_, _ = coreh2.ReadFrameFrom(br) // WINDOW_UPDATE

		// Server SETTINGS
		st := &coreh2.Settings{}
		st.Reset()
		stFrame := coreh2.AcquireFrameHeader()
		stFrame.SetBody(st)
		_, _ = stFrame.WriteTo(bw)
		_ = bw.Flush()
		coreh2.ReleaseFrameHeader(stFrame)

		_, _ = coreh2.ReadFrameFrom(br) // SETTINGS ACK

		ackFr := coreh2.AcquireFrameHeader()
		ackSt := coreh2.AcquireFrame(coreh2.FrameSettings).(*coreh2.Settings)
		ackSt.SetAck(true)
		ackFr.SetBody(ackSt)
		_, _ = ackFr.WriteTo(bw)
		_ = bw.Flush()
		coreh2.ReleaseFrameHeader(ackFr)

		// Read client request HEADERS for stream 1
		reqHdrFr, err := coreh2.ReadFrameFrom(br)
		if err != nil {
			return
		}
		streamID := reqHdrFr.Stream()
		coreh2.ReleaseFrameHeader(reqHdrFr)

		// Server sends GOAWAY(lastStreamID=1, NO_ERROR)
		gaFr := coreh2.AcquireFrameHeader()
		ga := coreh2.AcquireFrame(coreh2.FrameGoAway).(*coreh2.GoAway)
		ga.SetStream(streamID)
		ga.SetCode(coreh2.NoError)
		gaFr.SetBody(ga)
		_, _ = gaFr.WriteTo(bw)
		_ = bw.Flush()
		coreh2.ReleaseFrameHeader(gaFr)

		// Small delay to let client read GOAWAY
		time.Sleep(30 * time.Millisecond)

		// Server transmits response for stream 1
		enc := hpack.AcquireHPACK()
		defer hpack.ReleaseHPACK(enc)

		var hdrPayload []byte
		hf := hpack.AcquireHeaderField()
		defer hpack.ReleaseHeaderField(hf)
		hf.Set(":status", "200")
		hdrPayload = enc.AppendHeader(hdrPayload, hf, false)

		respHdrFr := coreh2.AcquireFrameHeader()
		respHdrFr.SetStream(streamID)
		respHdrFr.SetFlags(coreh2.FlagEndHeaders)
		respH := coreh2.AcquireFrame(coreh2.FrameHeaders).(*coreh2.Headers)
		respH.SetEndHeaders(true)
		respH.SetHeaders(hdrPayload)
		respHdrFr.SetBody(respH)
		_, _ = respHdrFr.WriteTo(bw)
		coreh2.ReleaseFrameHeader(respHdrFr)

		dataFr := coreh2.AcquireFrameHeader()
		dataFr.SetStream(streamID)
		dataFr.SetFlags(coreh2.FlagEndStream)
		data := coreh2.AcquireFrame(coreh2.FrameData).(*coreh2.Data)
		data.SetEndStream(true)
		data.SetData([]byte("graceful payload"))
		dataFr.SetBody(data)
		_, _ = dataFr.WriteTo(bw)
		_ = bw.Flush()
		coreh2.ReleaseFrameHeader(dataFr)
	}()

	client := h2client.NewConn(clientConn, h2client.ConnOpts{})
	require.NoError(t, client.Handshake())
	defer client.Close()

	req := h1.AcquireRequest()
	defer h1.ReleaseRequest(req)
	resp := h1.AcquireResponse()
	defer h1.ReleaseResponse(resp)

	req.Header.SetMethod("GET")
	req.SetRequestURI("http://example.com/in-flight")

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	err := client.Do(ctx, req, resp)
	require.NoError(t, err, "in-flight stream <= lastStreamID must complete successfully on GOAWAY")
	assert.Equal(t, 200, resp.StatusCode())
	assert.Equal(t, "graceful payload", string(resp.Body()))

	<-serverDone
}

// TestClient_HPACK_DynamicTableSizeUpdate_SubsequentStreams verifies RFC 7541 §4.2:
// A dynamic table size update MUST occur at the beginning of the header block.
// When stream 3 contains a dynamic table size update, the client must accept it.
// On HEAD: client fails with ErrTableSizeUpdateNotAllowed because BeginHeaderBlock was never called.
func TestClient_HPACK_DynamicTableSizeUpdate_SubsequentStreams(t *testing.T) {
	clientConn, serverConn, cleanup := setupClientServer(t)
	defer cleanup()

	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		br := bufio.NewReader(serverConn)
		bw := bufio.NewWriter(serverConn)

		_ = coreh2.ReadPreface(br)
		_, _ = coreh2.ReadFrameFrom(br) // SETTINGS
		_, _ = coreh2.ReadFrameFrom(br) // WINDOW_UPDATE

		st := &coreh2.Settings{}
		st.Reset()
		stFrame := coreh2.AcquireFrameHeader()
		stFrame.SetBody(st)
		_, _ = stFrame.WriteTo(bw)
		_ = bw.Flush()
		coreh2.ReleaseFrameHeader(stFrame)

		_, _ = coreh2.ReadFrameFrom(br) // SETTINGS ACK

		ackFr := coreh2.AcquireFrameHeader()
		ackSt := coreh2.AcquireFrame(coreh2.FrameSettings).(*coreh2.Settings)
		ackSt.SetAck(true)
		ackFr.SetBody(ackSt)
		_, _ = ackFr.WriteTo(bw)
		_ = bw.Flush()
		coreh2.ReleaseFrameHeader(ackFr)

		enc := hpack.AcquireHPACK()
		defer hpack.ReleaseHPACK(enc)

		// Handle Stream 1: Simple 200 OK
		req1, _ := coreh2.ReadFrameFrom(br)
		s1 := req1.Stream()
		coreh2.ReleaseFrameHeader(req1)

		var h1Payload []byte
		hf := hpack.AcquireHeaderField()
		defer hpack.ReleaseHeaderField(hf)
		hf.Set(":status", "200")
		h1Payload = enc.AppendHeader(h1Payload, hf, false)

		r1Fr := coreh2.AcquireFrameHeader()
		r1Fr.SetStream(s1)
		r1Fr.SetFlags(coreh2.FlagEndHeaders | coreh2.FlagEndStream)
		r1H := coreh2.AcquireFrame(coreh2.FrameHeaders).(*coreh2.Headers)
		r1H.SetEndHeaders(true)
		r1H.SetEndStream(true)
		r1H.SetHeaders(h1Payload)
		r1Fr.SetBody(r1H)
		_, _ = r1Fr.WriteTo(bw)
		_ = bw.Flush()
		coreh2.ReleaseFrameHeader(r1Fr)

		// Handle Stream 3: Response begins with dynamic table size update
		req3, _ := coreh2.ReadFrameFrom(br)
		s3 := req3.Stream()
		coreh2.ReleaseFrameHeader(req3)

		tableUpdate := []byte{0x3F, 0xE1, 0x03}
		var h3Payload []byte
		h3Payload = append(h3Payload, tableUpdate...)
		hf.Set(":status", "200")
		h3Payload = enc.AppendHeader(h3Payload, hf, false)

		r3Fr := coreh2.AcquireFrameHeader()
		r3Fr.SetStream(s3)
		r3Fr.SetFlags(coreh2.FlagEndHeaders | coreh2.FlagEndStream)
		r3H := coreh2.AcquireFrame(coreh2.FrameHeaders).(*coreh2.Headers)
		r3H.SetEndHeaders(true)
		r3H.SetEndStream(true)
		r3H.SetHeaders(h3Payload)
		r3Fr.SetBody(r3H)
		_, _ = r3Fr.WriteTo(bw)
		_ = bw.Flush()
		coreh2.ReleaseFrameHeader(r3Fr)
	}()

	client := h2client.NewConn(clientConn, h2client.ConnOpts{})
	require.NoError(t, client.Handshake())
	defer client.Close()

	// Request 1: Succeeds
	req1 := h1.AcquireRequest()
	defer h1.ReleaseRequest(req1)
	resp1 := h1.AcquireResponse()
	defer h1.ReleaseResponse(resp1)
	req1.Header.SetMethod("GET")
	req1.SetRequestURI("http://example.com/req1")
	require.NoError(t, client.Do(context.Background(), req1, resp1))
	assert.Equal(t, 200, resp1.StatusCode())

	// Request 2 (Stream 3): Has dynamic table size update at start of response
	req2 := h1.AcquireRequest()
	defer h1.ReleaseRequest(req2)
	resp2 := h1.AcquireResponse()
	defer h1.ReleaseResponse(resp2)
	req2.Header.SetMethod("GET")
	req2.SetRequestURI("http://example.com/req2")
	err := client.Do(context.Background(), req2, resp2)
	require.NoError(t, err, "subsequent stream with dynamic table size update MUST be accepted (RFC 7541 §4.2)")
	assert.Equal(t, 200, resp2.StatusCode())

	<-serverDone
}
