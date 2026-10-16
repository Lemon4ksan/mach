// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h2_test

import (
	"bufio"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lemon4ksan/foundation/testing/assert"
	"github.com/lemon4ksan/foundation/testing/require"

	"github.com/lemon4ksan/mach/proto/codec/hpack"
	coreh2 "github.com/lemon4ksan/mach/proto/h2"
	"github.com/lemon4ksan/mach/proto/semantic/status"
	"github.com/lemon4ksan/mach/server/h2"
)

// Helper to set up a test server and client socket pair
func setupServerClient(t *testing.T, handler h2.ServerHandlerFunc) (net.Conn, *h2.ServerConn, func()) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	serverDone := make(chan struct{})

	go func() {
		defer close(serverDone)
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		sc := h2.NewServerConn(conn, handler)
		defer sc.Release()
		_ = sc.Serve()
	}()

	clientConn, err := net.Dial("tcp", ln.Addr().String())
	require.NoError(t, err)
	_ = clientConn.SetDeadline(time.Now().Add(2 * time.Second))

	// Send HTTP/2 preface + empty SETTINGS
	_, err = clientConn.Write([]byte("PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n\x00\x00\x00\x04\x00\x00\x00\x00\x00"))
	require.NoError(t, err)

	cleanup := func() {
		_ = clientConn.Close()
		_ = ln.Close()
		<-serverDone
	}

	return clientConn, nil, cleanup
}

func encodeValidHeaders(enc *hpack.HPACK, path string) []byte {
	var payload []byte
	hf := hpack.AcquireHeaderField()
	defer hpack.ReleaseHeaderField(hf)

	for _, kv := range [][2]string{
		{":method", "GET"},
		{":scheme", "http"},
		{":path", path},
	} {
		hf.Set(kv[0], kv[1])
		payload = enc.AppendHeader(payload, hf, false)
	}
	return payload
}

// TestServer_RFC9113_StreamID_Monotonicity verifies RFC 9113 §5.1.1:
// "The identifier of a newly established stream MUST be numerically greater than all streams that
// the initiating endpoint has opened or reserved... An endpoint that receives an unexpected stream identifier
// MUST respond with a connection error (Section 5.4.1) of type PROTOCOL_ERROR."
func TestServer_RFC9113_StreamID_Monotonicity(t *testing.T) {
	clientConn, _, cleanup := setupServerClient(t, func(req *h2.ServerRequest, res *h2.ServerResponse) error {
		res.StatusCode = status.OK
		return nil
	})
	defer cleanup()

	enc := hpack.AcquireHPACK()
	defer hpack.ReleaseHPACK(enc)

	bw := bufio.NewWriter(clientConn)
	br := bufio.NewReader(clientConn)

	// Stream 5: Valid initial stream
	hdr5 := encodeValidHeaders(enc, "/stream5")
	fr5 := coreh2.AcquireFrameHeader()
	defer coreh2.ReleaseFrameHeader(fr5)
	h5 := coreh2.AcquireFrame(coreh2.FrameHeaders).(*coreh2.Headers)
	h5.SetEndHeaders(true)
	h5.SetEndStream(true)
	h5.SetHeaders(hdr5)
	fr5.SetStream(5)
	fr5.SetFlags(coreh2.FlagEndHeaders | coreh2.FlagEndStream)
	fr5.SetBody(h5)
	_, err := fr5.WriteTo(bw)
	require.NoError(t, err)
	require.NoError(t, bw.Flush())

	// Stream 3: Lower stream ID after Stream 5 was opened! MUST trigger PROTOCOL_ERROR!
	hdr3 := encodeValidHeaders(enc, "/stream3")
	fr3 := coreh2.AcquireFrameHeader()
	defer coreh2.ReleaseFrameHeader(fr3)
	h3 := coreh2.AcquireFrame(coreh2.FrameHeaders).(*coreh2.Headers)
	h3.SetEndHeaders(true)
	h3.SetEndStream(true)
	h3.SetHeaders(hdr3)
	fr3.SetStream(3)
	fr3.SetFlags(coreh2.FlagEndHeaders | coreh2.FlagEndStream)
	fr3.SetBody(h3)
	_, err = fr3.WriteTo(bw)
	require.NoError(t, err)
	require.NoError(t, bw.Flush())

	// Server MUST return GOAWAY with PROTOCOL_ERROR (RFC 9113 §5.1.1)
	gotProtocolError := false
	for range 15 {
		respFr, rErr := coreh2.ReadFrameFrom(br)
		if rErr != nil {
			break
		}
		if respFr.Type() == coreh2.FrameGoAway {
			ga := respFr.Body().(*coreh2.GoAway)
			if ga.Code() == coreh2.ProtocolError {
				gotProtocolError = true
			}
			coreh2.ReleaseFrameHeader(respFr)
			break
		}
		coreh2.ReleaseFrameHeader(respFr)
	}

	assert.True(
		t,
		gotProtocolError,
		"expected GOAWAY(PROTOCOL_ERROR) when opening lower stream ID 3 after 5 (RFC 9113 §5.1.1)",
	)
}

// TestServer_RFC9113_Continuation_Interleaving verifies RFC 9113 §6.2 & §6.10:
// "A HEADERS frame (without the END_HEADERS flag set) can be followed by a CONTINUATION frame for
// the same stream. A receiver MUST treat the receipt of any other type of frame or a frame on a different
// stream as a connection error (Section 5.4.1) of type PROTOCOL_ERROR."
func TestServer_RFC9113_Continuation_Interleaving(t *testing.T) {
	clientConn, _, cleanup := setupServerClient(t, func(req *h2.ServerRequest, res *h2.ServerResponse) error {
		res.StatusCode = status.OK
		return nil
	})
	defer cleanup()

	enc := hpack.AcquireHPACK()
	defer hpack.ReleaseHPACK(enc)

	bw := bufio.NewWriter(clientConn)
	br := bufio.NewReader(clientConn)

	// Stream 1: HEADERS without END_HEADERS
	hdr := encodeValidHeaders(enc, "/continuation-test")
	frHdr := coreh2.AcquireFrameHeader()
	defer coreh2.ReleaseFrameHeader(frHdr)
	hFrame := coreh2.AcquireFrame(coreh2.FrameHeaders).(*coreh2.Headers)
	hFrame.SetEndHeaders(false) // NO END_HEADERS!
	hFrame.SetEndStream(false)
	hFrame.SetHeaders(hdr)
	frHdr.SetStream(1)
	frHdr.SetFlags(0)
	frHdr.SetBody(hFrame)
	_, err := frHdr.WriteTo(bw)
	require.NoError(t, err)

	// INTERLEAVED FRAME: Send a DATA frame on stream 1 instead of CONTINUATION!
	frData := coreh2.AcquireFrameHeader()
	defer coreh2.ReleaseFrameHeader(frData)
	dFrame := coreh2.AcquireFrame(coreh2.FrameData).(*coreh2.Data)
	dFrame.SetData([]byte("interleaved data"))
	frData.SetStream(1)
	frData.SetFlags(0)
	frData.SetBody(dFrame)
	_, err = frData.WriteTo(bw)
	require.NoError(t, err)
	require.NoError(t, bw.Flush())

	// Server MUST return GOAWAY with PROTOCOL_ERROR (RFC 9113 §6.2)
	gotProtocolError := false
	for range 15 {
		respFr, rErr := coreh2.ReadFrameFrom(br)
		if rErr != nil {
			break
		}
		if respFr.Type() == coreh2.FrameGoAway {
			ga := respFr.Body().(*coreh2.GoAway)
			if ga.Code() == coreh2.ProtocolError {
				gotProtocolError = true
			}
			coreh2.ReleaseFrameHeader(respFr)
			break
		}
		coreh2.ReleaseFrameHeader(respFr)
	}

	assert.True(
		t,
		gotProtocolError,
		"expected GOAWAY(PROTOCOL_ERROR) when frame is interleaved during header block (RFC 9113 §6.2)",
	)
}

// TestServer_RFC9113_HalfClosedRemote_DataRejected verifies RFC 9113 §5.1:
// "half-closed (remote): If an endpoint receives additional frames other than WINDOW_UPDATE,
// PRIORITY, or RST_STREAM for a stream that is in this state, it MUST respond with a stream error
// (Section 5.4.2) of type STREAM_CLOSED."
// Also verifies that sending extra DATA does NOT cause a data race or duplicate dispatch.
func TestServer_RFC9113_HalfClosedRemote_DataRejected(t *testing.T) {
	var handlerInvocations atomic.Int32

	clientConn, _, cleanup := setupServerClient(t, func(req *h2.ServerRequest, res *h2.ServerResponse) error {
		handlerInvocations.Add(1)
		// Small delay to keep the stream in halfClosedRemote while extra frame is sent
		time.Sleep(50 * time.Millisecond)
		res.StatusCode = status.OK
		return nil
	})
	defer cleanup()

	enc := hpack.AcquireHPACK()
	defer hpack.ReleaseHPACK(enc)

	bw := bufio.NewWriter(clientConn)
	br := bufio.NewReader(clientConn)

	// Stream 1: HEADERS with END_HEADERS (endStream=false)
	hdr := encodeValidHeaders(enc, "/half-closed-test")
	frHdr := coreh2.AcquireFrameHeader()
	defer coreh2.ReleaseFrameHeader(frHdr)
	hFrame := coreh2.AcquireFrame(coreh2.FrameHeaders).(*coreh2.Headers)
	hFrame.SetEndHeaders(true)
	hFrame.SetEndStream(false)
	hFrame.SetHeaders(hdr)
	frHdr.SetStream(1)
	frHdr.SetFlags(coreh2.FlagEndHeaders)
	frHdr.SetBody(hFrame)
	_, err := frHdr.WriteTo(bw)
	require.NoError(t, err)

	// DATA frame with END_STREAM -> enters half-closed (remote) state
	frData1 := coreh2.AcquireFrameHeader()
	defer coreh2.ReleaseFrameHeader(frData1)
	dFrame1 := coreh2.AcquireFrame(coreh2.FrameData).(*coreh2.Data)
	dFrame1.SetData([]byte("initial payload"))
	dFrame1.SetEndStream(true)
	frData1.SetStream(1)
	frData1.SetFlags(coreh2.FlagEndStream)
	frData1.SetBody(dFrame1)
	_, err = frData1.WriteTo(bw)
	require.NoError(t, err)
	require.NoError(t, bw.Flush())

	// Allow server to transition stream to halfClosedRemote and start dispatchStream
	time.Sleep(10 * time.Millisecond)

	// Extra DATA frame on half-closed remote stream! MUST be rejected with STREAM_CLOSED!
	frData2 := coreh2.AcquireFrameHeader()
	defer coreh2.ReleaseFrameHeader(frData2)
	dFrame2 := coreh2.AcquireFrame(coreh2.FrameData).(*coreh2.Data)
	dFrame2.SetData([]byte("illegal extra data"))
	dFrame2.SetEndStream(true)
	frData2.SetStream(1)
	frData2.SetFlags(coreh2.FlagEndStream)
	frData2.SetBody(dFrame2)
	_, err = frData2.WriteTo(bw)
	require.NoError(t, err)
	require.NoError(t, bw.Flush())

	// Read frames: expect RST_STREAM(1, STREAM_CLOSED)
	gotStreamClosed := false
	for range 15 {
		respFr, rErr := coreh2.ReadFrameFrom(br)
		if rErr != nil {
			break
		}
		if respFr.Type() == coreh2.FrameResetStream && respFr.Stream() == 1 {
			rst := respFr.Body().(*coreh2.RstStream)
			if rst.Code() == coreh2.StreamClosedError {
				gotStreamClosed = true
			}
			coreh2.ReleaseFrameHeader(respFr)
			break
		}
		coreh2.ReleaseFrameHeader(respFr)
	}

	assert.True(
		t,
		gotStreamClosed,
		"expected RST_STREAM(STREAM_CLOSED) for extra DATA on half-closed stream (RFC 9113 §5.1)",
	)
	assert.Equal(t, int32(1), handlerInvocations.Load(), "handler must not be invoked more than once for the stream")
}
