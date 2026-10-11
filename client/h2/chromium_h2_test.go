// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h2_test

import (
	"bufio"
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/lemon4ksan/foundation/testing/assert"
	"github.com/lemon4ksan/foundation/testing/require"

	h2client "github.com/lemon4ksan/mach/client/h2"
	"github.com/lemon4ksan/mach/proto/codec/hpack"
	coreh2 "github.com/lemon4ksan/mach/proto/h2"
	h1 "github.com/lemon4ksan/mach/proto/http"
)

func handshakeServer(t *testing.T, serverConn net.Conn) (*bufio.Reader, *bufio.Writer) {
	br := bufio.NewReader(serverConn)
	bw := bufio.NewWriter(serverConn)

	if !coreh2.ReadPreface(br) {
		t.Fatalf("server: invalid preface")
	}

	// Read client SETTINGS & WINDOW_UPDATE
	_, _ = coreh2.ReadFrameFrom(br)
	_, _ = coreh2.ReadFrameFrom(br)

	// Send server SETTINGS
	st := &coreh2.Settings{}
	st.Reset()
	stFrame := coreh2.AcquireFrameHeader()
	stFrame.SetBody(st)
	_, _ = stFrame.WriteTo(bw)
	_ = bw.Flush()
	coreh2.ReleaseFrameHeader(stFrame)

	// Read client SETTINGS ACK
	_, _ = coreh2.ReadFrameFrom(br)

	// Send server SETTINGS ACK
	ackFr := coreh2.AcquireFrameHeader()
	ackSt := coreh2.AcquireFrame(coreh2.FrameSettings).(*coreh2.Settings)
	ackSt.SetAck(true)
	ackFr.SetBody(ackSt)
	_, _ = ackFr.WriteTo(bw)
	_ = bw.Flush()
	coreh2.ReleaseFrameHeader(ackFr)

	return br, bw
}

// Ported from Chromium net/spdy: SpdySessionTest / BufferedSpdyFramerTest
// Tests receiving a response header block split across HEADERS (without END_HEADERS)
// and CONTINUATION (with END_HEADERS) frames.
func TestChromium_Client_HeadersWithContinuation(t *testing.T) {
	clientConn, serverConn, cleanup := setupClientServer(t)
	defer cleanup()

	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		br, bw := handshakeServer(t, serverConn)

		// Read client request HEADERS
		reqFr, err := coreh2.ReadFrameFrom(br)
		if err != nil {
			return
		}
		streamID := reqFr.Stream()
		coreh2.ReleaseFrameHeader(reqFr)

		enc := hpack.AcquireHPACK()
		defer hpack.ReleaseHPACK(enc)

		hf := hpack.AcquireHeaderField()
		defer hpack.ReleaseHeaderField(hf)

		// Build response headers: :status=200, content-type=text/plain, x-foo=bar
		hf.Set(":status", "200")
		frag1 := enc.AppendHeader(nil, hf, false)

		hf.Set("content-type", "text/plain")
		frag2 := enc.AppendHeader(nil, hf, false)
		hf.Set("x-foo", "bar")
		frag2 = enc.AppendHeader(frag2, hf, false)

		// Frame 1: HEADERS without END_HEADERS
		frHdr := coreh2.AcquireFrameHeader()
		hFrame := coreh2.AcquireFrame(coreh2.FrameHeaders).(*coreh2.Headers)
		hFrame.SetEndHeaders(false) // NO END_HEADERS!
		hFrame.SetEndStream(false)
		hFrame.SetHeaders(frag1)
		frHdr.SetStream(streamID)
		frHdr.SetFlags(0)
		frHdr.SetBody(hFrame)
		_, _ = frHdr.WriteTo(bw)
		coreh2.ReleaseFrameHeader(frHdr)

		// Frame 2: CONTINUATION with END_HEADERS
		frCont := coreh2.AcquireFrameHeader()
		cFrame := coreh2.AcquireFrame(coreh2.FrameContinuation).(*coreh2.Continuation)
		cFrame.SetEndHeaders(true)
		cFrame.SetHeader(frag2)
		frCont.SetStream(streamID)
		frCont.SetFlags(coreh2.FlagEndHeaders)
		frCont.SetBody(cFrame)
		_, _ = frCont.WriteTo(bw)
		coreh2.ReleaseFrameHeader(frCont)

		// Frame 3: DATA with END_STREAM
		frData := coreh2.AcquireFrameHeader()
		dFrame := coreh2.AcquireFrame(coreh2.FrameData).(*coreh2.Data)
		dFrame.SetEndStream(true)
		dFrame.SetData([]byte("hello continuation"))
		frData.SetStream(streamID)
		frData.SetFlags(coreh2.FlagEndStream)
		frData.SetBody(dFrame)
		_, _ = frData.WriteTo(bw)
		coreh2.ReleaseFrameHeader(frData)
		_ = bw.Flush()
	}()

	c := h2client.NewConn(clientConn, h2client.ConnOpts{})
	require.NoError(t, c.Handshake())
	defer func() { _ = c.Close() }()

	req := h1.AcquireRequest()
	defer h1.ReleaseRequest(req)
	req.SetRequestURI("http://127.0.0.1/test")
	req.Header.SetMethod("GET")

	res := h1.AcquireResponse()
	defer h1.ReleaseResponse(res)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := c.Do(ctx, req, res)
	require.NoError(t, err, "client should successfully receive response split across HEADERS + CONTINUATION")
	assert.Equal(t, 200, res.StatusCode())
	assert.Equal(t, "hello continuation", string(res.Body()))
	<-serverDone
}

// Ported from Chromium net/spdy: SpdyStreamTest.TrailersMustNotFollowTrailers
// RFC 9113 §8.1:
// "An endpoint that receives a trailer section after another trailer section MUST treat this as
// a stream error (Section 5.4.2) of type PROTOCOL_ERROR."
func TestChromium_Client_TrailersMustNotFollowTrailers(t *testing.T) {
	clientConn, serverConn, cleanup := setupClientServer(t)
	defer cleanup()

	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		br, bw := handshakeServer(t, serverConn)

		reqFr, err := coreh2.ReadFrameFrom(br)
		if err != nil {
			return
		}
		streamID := reqFr.Stream()
		coreh2.ReleaseFrameHeader(reqFr)

		enc := hpack.AcquireHPACK()
		defer hpack.ReleaseHPACK(enc)

		hf := hpack.AcquireHeaderField()
		defer hpack.ReleaseHeaderField(hf)

		// Response headers
		hf.Set(":status", "200")
		hdrPayload := enc.AppendHeader(nil, hf, false)

		frHdr := coreh2.AcquireFrameHeader()
		hFrame := coreh2.AcquireFrame(coreh2.FrameHeaders).(*coreh2.Headers)
		hFrame.SetEndHeaders(true)
		hFrame.SetEndStream(false)
		hFrame.SetHeaders(hdrPayload)
		frHdr.SetStream(streamID)
		frHdr.SetFlags(coreh2.FlagEndHeaders)
		frHdr.SetBody(hFrame)
		_, _ = frHdr.WriteTo(bw)
		coreh2.ReleaseFrameHeader(frHdr)

		// First trailers block
		hf.Set("first-trailer", "val1")
		tr1Payload := enc.AppendHeader(nil, hf, false)
		frTr1 := coreh2.AcquireFrameHeader()
		tr1Frame := coreh2.AcquireFrame(coreh2.FrameHeaders).(*coreh2.Headers)
		tr1Frame.SetEndHeaders(true)
		tr1Frame.SetEndStream(false)
		tr1Frame.SetHeaders(tr1Payload)
		frTr1.SetStream(streamID)
		frTr1.SetFlags(coreh2.FlagEndHeaders)
		frTr1.SetBody(tr1Frame)
		_, _ = frTr1.WriteTo(bw)
		coreh2.ReleaseFrameHeader(frTr1)

		// Second trailers block (FORBIDDEN per RFC 9113 §8.1 / Chromium)
		hf.Set("second-trailer", "val2")
		tr2Payload := enc.AppendHeader(nil, hf, false)
		frTr2 := coreh2.AcquireFrameHeader()
		tr2Frame := coreh2.AcquireFrame(coreh2.FrameHeaders).(*coreh2.Headers)
		tr2Frame.SetEndHeaders(true)
		tr2Frame.SetEndStream(true)
		tr2Frame.SetHeaders(tr2Payload)
		frTr2.SetStream(streamID)
		frTr2.SetFlags(coreh2.FlagEndHeaders | coreh2.FlagEndStream)
		frTr2.SetBody(tr2Frame)
		_, _ = frTr2.WriteTo(bw)
		coreh2.ReleaseFrameHeader(frTr2)
		_ = bw.Flush()
	}()

	c := h2client.NewConn(clientConn, h2client.ConnOpts{})
	require.NoError(t, c.Handshake())
	defer func() { _ = c.Close() }()

	req := h1.AcquireRequest()
	defer h1.ReleaseRequest(req)
	req.SetRequestURI("http://127.0.0.1/test")
	req.Header.SetMethod("GET")

	res := h1.AcquireResponse()
	defer h1.ReleaseResponse(res)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := c.Do(ctx, req, res)
	require.Error(t, err, "client MUST reject trailers following trailers with PROTOCOL_ERROR")
	assert.True(t, errors.Is(err, coreh2.ProtocolError), "error must be PROTOCOL_ERROR")
	<-serverDone
}

// Ported from Chromium net/spdy: SpdyStreamTest.DataMustNotFollowTrailers
// RFC 9113 §8.1:
// "An endpoint that receives a DATA frame after a trailer section MUST treat the receipt as
// a stream error (Section 5.4.2) of type PROTOCOL_ERROR."
func TestChromium_Client_DataMustNotFollowTrailers(t *testing.T) {
	clientConn, serverConn, cleanup := setupClientServer(t)
	defer cleanup()

	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		br, bw := handshakeServer(t, serverConn)

		reqFr, err := coreh2.ReadFrameFrom(br)
		if err != nil {
			return
		}
		streamID := reqFr.Stream()
		coreh2.ReleaseFrameHeader(reqFr)

		enc := hpack.AcquireHPACK()
		defer hpack.ReleaseHPACK(enc)

		hf := hpack.AcquireHeaderField()
		defer hpack.ReleaseHeaderField(hf)

		// Response headers
		hf.Set(":status", "200")
		hdrPayload := enc.AppendHeader(nil, hf, false)

		frHdr := coreh2.AcquireFrameHeader()
		hFrame := coreh2.AcquireFrame(coreh2.FrameHeaders).(*coreh2.Headers)
		hFrame.SetEndHeaders(true)
		hFrame.SetEndStream(false)
		hFrame.SetHeaders(hdrPayload)
		frHdr.SetStream(streamID)
		frHdr.SetFlags(coreh2.FlagEndHeaders)
		frHdr.SetBody(hFrame)
		_, _ = frHdr.WriteTo(bw)
		coreh2.ReleaseFrameHeader(frHdr)

		// Trailers block
		hf.Set("my-trailer", "val1")
		trPayload := enc.AppendHeader(nil, hf, false)
		frTr := coreh2.AcquireFrameHeader()
		trFrame := coreh2.AcquireFrame(coreh2.FrameHeaders).(*coreh2.Headers)
		trFrame.SetEndHeaders(true)
		trFrame.SetEndStream(false)
		trFrame.SetHeaders(trPayload)
		frTr.SetStream(streamID)
		frTr.SetFlags(coreh2.FlagEndHeaders)
		frTr.SetBody(trFrame)
		_, _ = frTr.WriteTo(bw)
		coreh2.ReleaseFrameHeader(frTr)

		// DATA frame following trailers (FORBIDDEN per RFC 9113 §8.1 / Chromium)
		frData := coreh2.AcquireFrameHeader()
		dFrame := coreh2.AcquireFrame(coreh2.FrameData).(*coreh2.Data)
		dFrame.SetEndStream(true)
		dFrame.SetData([]byte("forbidden data after trailers"))
		frData.SetStream(streamID)
		frData.SetFlags(coreh2.FlagEndStream)
		frData.SetBody(dFrame)
		_, _ = frData.WriteTo(bw)
		coreh2.ReleaseFrameHeader(frData)
		_ = bw.Flush()
	}()

	c := h2client.NewConn(clientConn, h2client.ConnOpts{})
	require.NoError(t, c.Handshake())
	defer func() { _ = c.Close() }()

	req := h1.AcquireRequest()
	defer h1.ReleaseRequest(req)
	req.SetRequestURI("http://127.0.0.1/test")
	req.Header.SetMethod("GET")

	res := h1.AcquireResponse()
	defer h1.ReleaseResponse(res)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := c.Do(ctx, req, res)
	require.Error(t, err, "client MUST reject DATA following trailers with PROTOCOL_ERROR")
	assert.True(t, errors.Is(err, coreh2.ProtocolError), "error must be PROTOCOL_ERROR")
	<-serverDone
}

// RFC 9113 §6.10: Unexpected CONTINUATION frame without preceding HEADERS
func TestChromium_Client_UnexpectedContinuationFrame(t *testing.T) {
	clientConn, serverConn, cleanup := setupClientServer(t)
	defer cleanup()

	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		br, bw := handshakeServer(t, serverConn)

		reqFr, err := coreh2.ReadFrameFrom(br)
		if err != nil {
			return
		}
		streamID := reqFr.Stream()
		coreh2.ReleaseFrameHeader(reqFr)

		// Send CONTINUATION frame on streamID without sending any HEADERS first!
		frCont := coreh2.AcquireFrameHeader()
		cFrame := coreh2.AcquireFrame(coreh2.FrameContinuation).(*coreh2.Continuation)
		cFrame.SetEndHeaders(true)
		cFrame.SetHeader([]byte{0x82}) // :method: GET
		frCont.SetStream(streamID)
		frCont.SetFlags(coreh2.FlagEndHeaders)
		frCont.SetBody(cFrame)
		_, _ = frCont.WriteTo(bw)
		coreh2.ReleaseFrameHeader(frCont)
		_ = bw.Flush()
	}()

	c := h2client.NewConn(clientConn, h2client.ConnOpts{})
	require.NoError(t, c.Handshake())
	defer func() { _ = c.Close() }()

	req := h1.AcquireRequest()
	defer h1.ReleaseRequest(req)
	req.SetRequestURI("http://127.0.0.1/test")
	req.Header.SetMethod("GET")

	res := h1.AcquireResponse()
	defer h1.ReleaseResponse(res)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := c.Do(ctx, req, res)
	require.Error(t, err, "unexpected CONTINUATION must be rejected with PROTOCOL_ERROR")
	assert.True(t, errors.Is(err, coreh2.ProtocolError), "error must be PROTOCOL_ERROR")
	<-serverDone
}
