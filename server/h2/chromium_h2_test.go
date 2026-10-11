// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h2_test

import (
	"bufio"
	"testing"
	"time"

	"github.com/lemon4ksan/foundation/testing/assert"
	"github.com/lemon4ksan/foundation/testing/require"

	"github.com/lemon4ksan/mach/proto/codec/hpack"
	coreh2 "github.com/lemon4ksan/mach/proto/h2"
	"github.com/lemon4ksan/mach/proto/semantic/status"
	"github.com/lemon4ksan/mach/server/h2"
)

// Helper to read frames from connection until timeout or matching predicate
func readFrameWithTimeout(br *bufio.Reader, timeout time.Duration) (*coreh2.FrameHeader, error) {
	type readRes struct {
		fr  *coreh2.FrameHeader
		err error
	}
	ch := make(chan readRes, 1)
	go func() {
		fr, err := coreh2.ReadFrameFrom(br)
		ch <- readRes{fr: fr, err: err}
	}()

	select {
	case res := <-ch:
		return res.fr, res.err
	case <-time.After(timeout):
		return nil, contextDeadlineExceeded{}
	}
}

type contextDeadlineExceeded struct{}

func (contextDeadlineExceeded) Error() string   { return "deadline exceeded" }
func (contextDeadlineExceeded) Timeout() bool   { return true }
func (contextDeadlineExceeded) Temporary() bool { return true }

// Ported from Chromium net/spdy: SpdyStreamTest.IncreaseSendWindowSizeOverflow
// RFC 9113 §6.9.1:
// "A sender MUST NOT allow a flow-control window to exceed 2^31-1 octets.
// If a sender receives a WINDOW_UPDATE that causes a flow-control window to exceed this maximum,
// for a stream, the receiver MUST treat this as a stream error (Section 5.4.2) of type FLOW_CONTROL_ERROR.
// The receiver MUST NOT close the connection."
func TestChromium_SpdyStreamTest_IncreaseSendWindowSizeOverflow(t *testing.T) {
	clientConn, _, cleanup := setupServerClient(t, func(req *h2.ServerRequest, res *h2.ServerResponse) error {
		res.StatusCode = status.OK
		return nil
	})
	defer cleanup()

	enc := hpack.AcquireHPACK()
	defer hpack.ReleaseHPACK(enc)

	bw := bufio.NewWriter(clientConn)
	br := bufio.NewReader(clientConn)

	// Open stream 1
	hdr1 := encodeValidHeaders(enc, "/stream1")
	fr1 := coreh2.AcquireFrameHeader()
	h1 := coreh2.AcquireFrame(coreh2.FrameHeaders).(*coreh2.Headers)
	h1.SetEndHeaders(true)
	h1.SetEndStream(false)
	h1.SetHeaders(hdr1)
	fr1.SetStream(1)
	fr1.SetFlags(coreh2.FlagEndHeaders)
	fr1.SetBody(h1)
	_, err := fr1.WriteTo(bw)
	coreh2.ReleaseFrameHeader(fr1)
	require.NoError(t, err)
	require.NoError(t, bw.Flush())

	// Send WINDOW_UPDATE on stream 1 with delta that overflows int32 (> 2^31-1).
	// Initial stream window is 65535. Delta of 0x7fffffff causes 65535 + 0x7fffffff > 2^31-1.
	frWU := coreh2.AcquireFrameHeader()
	wu := coreh2.AcquireFrame(coreh2.FrameWindowUpdate).(*coreh2.WindowUpdate)
	wu.SetIncrement(0x7fffffff)
	frWU.SetStream(1)
	frWU.SetBody(wu)
	_, err = frWU.WriteTo(bw)
	coreh2.ReleaseFrameHeader(frWU)
	require.NoError(t, err)
	require.NoError(t, bw.Flush())

	// Chromium Expectation:
	// 1. Server MUST emit RST_STREAM on stream 1 with FLOW_CONTROL_ERROR.
	// 2. Connection MUST remain alive! Stream 3 can be opened successfully.
	gotRstStream := false
	gotGoAway := false
	for range 15 {
		respFr, rErr := readFrameWithTimeout(br, 500*time.Millisecond)
		if rErr != nil {
			break
		}
		if respFr.Type() == coreh2.FrameResetStream && respFr.Stream() == 1 {
			rst := respFr.Body().(*coreh2.RstStream)
			if rst.Code() == coreh2.FlowControlError {
				gotRstStream = true
			}
			coreh2.ReleaseFrameHeader(respFr)
			break
		}
		if respFr.Type() == coreh2.FrameGoAway {
			gotGoAway = true
			coreh2.ReleaseFrameHeader(respFr)
			break
		}
		coreh2.ReleaseFrameHeader(respFr)
	}

	assert.True(t, gotRstStream, "expected RST_STREAM(1, FLOW_CONTROL_ERROR)")
	assert.False(t, gotGoAway, "stream flow-control overflow MUST NOT terminate connection with GOAWAY")

	// Verify connection is still usable: open stream 3
	hdr3 := encodeValidHeaders(enc, "/stream3")
	fr3 := coreh2.AcquireFrameHeader()
	h3 := coreh2.AcquireFrame(coreh2.FrameHeaders).(*coreh2.Headers)
	h3.SetEndHeaders(true)
	h3.SetEndStream(true)
	h3.SetHeaders(hdr3)
	fr3.SetStream(3)
	fr3.SetFlags(coreh2.FlagEndHeaders | coreh2.FlagEndStream)
	fr3.SetBody(h3)
	_, err = fr3.WriteTo(bw)
	coreh2.ReleaseFrameHeader(fr3)
	require.NoError(t, err)
	require.NoError(t, bw.Flush())

	// Read response on stream 3
	gotStream3Resp := false
	for range 15 {
		respFr, rErr := readFrameWithTimeout(br, 500*time.Millisecond)
		if rErr != nil {
			break
		}
		if respFr.Type() == coreh2.FrameHeaders && respFr.Stream() == 3 {
			gotStream3Resp = true
			coreh2.ReleaseFrameHeader(respFr)
			break
		}
		coreh2.ReleaseFrameHeader(respFr)
	}
	assert.True(t, gotStream3Resp, "connection must remain alive to serve stream 3 after stream 1 reset")
}

// Ported from Chromium net/spdy: SpdySessionTest.GoAwayOnSessionFlowControlError
// RFC 9113 §6.9.1:
// "For flow-control frames on stream 0, a connection error (Section 5.4.1) of type FLOW_CONTROL_ERROR MUST be used."
func TestChromium_SpdySessionTest_GoAwayOnSessionFlowControlError(t *testing.T) {
	clientConn, _, cleanup := setupServerClient(t, func(req *h2.ServerRequest, res *h2.ServerResponse) error {
		res.StatusCode = status.OK
		return nil
	})
	defer cleanup()

	bw := bufio.NewWriter(clientConn)
	br := bufio.NewReader(clientConn)

	// Send WINDOW_UPDATE on stream 0 that overflows int32 (> 2^31-1)
	frWU := coreh2.AcquireFrameHeader()
	wu := coreh2.AcquireFrame(coreh2.FrameWindowUpdate).(*coreh2.WindowUpdate)
	wu.SetIncrement(0x7fffffff)
	frWU.SetStream(0)
	frWU.SetBody(wu)
	_, err := frWU.WriteTo(bw)
	coreh2.ReleaseFrameHeader(frWU)
	require.NoError(t, err)
	require.NoError(t, bw.Flush())

	// Chromium Expectation: Server MUST send GOAWAY with FLOW_CONTROL_ERROR
	gotGoAway := false
	for range 15 {
		respFr, rErr := readFrameWithTimeout(br, 500*time.Millisecond)
		if rErr != nil {
			break
		}
		if respFr.Type() == coreh2.FrameGoAway {
			ga := respFr.Body().(*coreh2.GoAway)
			if ga.Code() == coreh2.FlowControlError {
				gotGoAway = true
			}
			coreh2.ReleaseFrameHeader(respFr)
			break
		}
		coreh2.ReleaseFrameHeader(respFr)
	}

	assert.True(t, gotGoAway, "expected GOAWAY(FLOW_CONTROL_ERROR) on connection flow control overflow")
}

// RFC 9113 §6.9:
// "A receiver MUST treat the receipt of a WINDOW_UPDATE frame with an flow-control window increment of 0
// as a stream error (Section 5.4.2) of type PROTOCOL_ERROR; for the connection flow-control window,
// a connection error (Section 5.4.1) of type PROTOCOL_ERROR MUST be used instead."
func TestChromium_SpdyStreamTest_WindowUpdateZeroIncrement_StreamError(t *testing.T) {
	clientConn, _, cleanup := setupServerClient(t, func(req *h2.ServerRequest, res *h2.ServerResponse) error {
		res.StatusCode = status.OK
		return nil
	})
	defer cleanup()

	enc := hpack.AcquireHPACK()
	defer hpack.ReleaseHPACK(enc)

	bw := bufio.NewWriter(clientConn)
	br := bufio.NewReader(clientConn)

	// Open stream 1
	hdr1 := encodeValidHeaders(enc, "/stream1")
	fr1 := coreh2.AcquireFrameHeader()
	h1 := coreh2.AcquireFrame(coreh2.FrameHeaders).(*coreh2.Headers)
	h1.SetEndHeaders(true)
	h1.SetEndStream(false)
	h1.SetHeaders(hdr1)
	fr1.SetStream(1)
	fr1.SetFlags(coreh2.FlagEndHeaders)
	fr1.SetBody(h1)
	_, err := fr1.WriteTo(bw)
	coreh2.ReleaseFrameHeader(fr1)
	require.NoError(t, err)
	require.NoError(t, bw.Flush())

	// Send WINDOW_UPDATE on stream 1 with increment = 0
	frWU := coreh2.AcquireFrameHeader()
	wu := coreh2.AcquireFrame(coreh2.FrameWindowUpdate).(*coreh2.WindowUpdate)
	wu.SetIncrement(0)
	frWU.SetStream(1)
	frWU.SetBody(wu)
	_, err = frWU.WriteTo(bw)
	coreh2.ReleaseFrameHeader(frWU)
	require.NoError(t, err)
	require.NoError(t, bw.Flush())

	// MUST be stream error RST_STREAM(1, PROTOCOL_ERROR), NOT GOAWAY
	gotRstStream := false
	gotGoAway := false
	for range 15 {
		respFr, rErr := readFrameWithTimeout(br, 500*time.Millisecond)
		if rErr != nil {
			break
		}
		if respFr.Type() == coreh2.FrameResetStream && respFr.Stream() == 1 {
			rst := respFr.Body().(*coreh2.RstStream)
			if rst.Code() == coreh2.ProtocolError {
				gotRstStream = true
			}
			coreh2.ReleaseFrameHeader(respFr)
			break
		}
		if respFr.Type() == coreh2.FrameGoAway {
			gotGoAway = true
			coreh2.ReleaseFrameHeader(respFr)
			break
		}
		coreh2.ReleaseFrameHeader(respFr)
	}

	assert.True(t, gotRstStream, "expected RST_STREAM(1, PROTOCOL_ERROR) for 0 increment on stream")
	assert.False(t, gotGoAway, "increment 0 on stream MUST NOT close connection with GOAWAY")
}

// Ported from Chromium net/spdy: SpdySessionTest.RejectInvalidUnknownFrames
// RFC 9113 §6.10:
// "A HEADERS frame (without the END_HEADERS flag set) can be followed by a CONTINUATION frame for
// the same stream. A receiver MUST treat the receipt of any other type of frame or a frame on a different
// stream as a connection error (Section 5.4.1) of type PROTOCOL_ERROR."
// Tests interleaved PRIORITY (0x02) and unknown extension frame (0x0a) between HEADERS and CONTINUATION.
func TestChromium_SpdySessionTest_InterleavedFramesBetweenHeadersAndContinuation(t *testing.T) {
	runTest := func(t *testing.T, interleaveFrame []byte, desc string) {
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
		hdr := encodeValidHeaders(enc, "/test")
		frHdr := coreh2.AcquireFrameHeader()
		hFrame := coreh2.AcquireFrame(coreh2.FrameHeaders).(*coreh2.Headers)
		hFrame.SetEndHeaders(false)
		hFrame.SetEndStream(false)
		hFrame.SetHeaders(hdr)
		frHdr.SetStream(1)
		frHdr.SetFlags(0)
		frHdr.SetBody(hFrame)
		_, err := frHdr.WriteTo(bw)
		coreh2.ReleaseFrameHeader(frHdr)
		require.NoError(t, err)

		// Send forbidden interleaved frame
		_, err = bw.Write(interleaveFrame)
		require.NoError(t, err)
		require.NoError(t, bw.Flush())

		// Server MUST return GOAWAY with PROTOCOL_ERROR
		gotProtocolError := false
		for range 15 {
			respFr, rErr := readFrameWithTimeout(br, 500*time.Millisecond)
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

		assert.True(t, gotProtocolError, "expected GOAWAY(PROTOCOL_ERROR) when "+desc+" is interleaved (RFC 9113 §6.10)")
	}

	t.Run("InterleavedPriorityFrame", func(t *testing.T) {
		// PRIORITY frame (type 0x02), length 5, stream 1
		priorityFrame := []byte{
			0x00, 0x00, 0x05, // length: 5
			0x02,             // type: PRIORITY
			0x00,             // flags: 0
			0x00, 0x00, 0x00, 0x01, // stream: 1
			0x00, 0x00, 0x00, 0x00, 0x10, // payload
		}
		runTest(t, priorityFrame, "PRIORITY frame")
	})

	t.Run("InterleavedUnknownExtensionFrame", func(t *testing.T) {
		// Unknown frame type 0x0a, length 4, stream 1
		unknownFrame := []byte{
			0x00, 0x00, 0x04, // length: 4
			0x0a,             // type: 0x0a (unknown)
			0x00,             // flags: 0
			0x00, 0x00, 0x00, 0x01, // stream: 1
			0x01, 0x02, 0x03, 0x04, // payload
		}
		runTest(t, unknownFrame, "unknown frame type 0x0a")
	})

	t.Run("InterleavedPingOnStream0", func(t *testing.T) {
		// PING frame (type 0x06), length 8, stream 0
		pingFrame := []byte{
			0x00, 0x00, 0x08, // length: 8
			0x06,             // type: PING
			0x00,             // flags: 0
			0x00, 0x00, 0x00, 0x00, // stream: 0
			0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, // payload
		}
		runTest(t, pingFrame, "PING frame on stream 0")
	})
}

// Ported from Chromium net/spdy: SpdySessionTest.SessionFlowControlNoReceiveLeaks
// RFC 9113 §5.4.2:
// "To avoid connection-level flow-control imbalances, a receiver MUST process the flow-control
// window increment for DATA frames that are received on a reset stream, even if the stream has
// already been closed. Specifically, the receiver MUST update the connection flow-control window
// for DATA frames received after sending or receiving a RST_STREAM."
func TestChromium_SpdySessionTest_SessionFlowControlNoReceiveLeaks(t *testing.T) {
	clientConn, _, cleanup := setupServerClient(t, func(req *h2.ServerRequest, res *h2.ServerResponse) error {
		res.StatusCode = status.OK
		return nil
	})
	defer cleanup()

	enc := hpack.AcquireHPACK()
	defer hpack.ReleaseHPACK(enc)

	bw := bufio.NewWriter(clientConn)
	br := bufio.NewReader(clientConn)

	// Stream 1: Request with END_STREAM=true (stream closes once response is sent)
	hdr1 := encodeValidHeaders(enc, "/stream1")
	fr1 := coreh2.AcquireFrameHeader()
	h1 := coreh2.AcquireFrame(coreh2.FrameHeaders).(*coreh2.Headers)
	h1.SetEndHeaders(true)
	h1.SetEndStream(true)
	h1.SetHeaders(hdr1)
	fr1.SetStream(1)
	fr1.SetFlags(coreh2.FlagEndHeaders | coreh2.FlagEndStream)
	fr1.SetBody(h1)
	_, err := fr1.WriteTo(bw)
	coreh2.ReleaseFrameHeader(fr1)
	require.NoError(t, err)
	require.NoError(t, bw.Flush())

	// Read response on stream 1
	for range 15 {
		respFr, rErr := readFrameWithTimeout(br, 500*time.Millisecond)
		if rErr != nil {
			break
		}
		if respFr.Type() == coreh2.FrameHeaders && respFr.Stream() == 1 {
			coreh2.ReleaseFrameHeader(respFr)
			break
		}
		coreh2.ReleaseFrameHeader(respFr)
	}

	// Now stream 1 is closed. Client sends an in-flight DATA frame (e.g. 500 bytes) on stream 1.
	dataPayload := make([]byte, 500)
	frData := coreh2.AcquireFrameHeader()
	dFrame := coreh2.AcquireFrame(coreh2.FrameData).(*coreh2.Data)
	dFrame.SetData(dataPayload)
	frData.SetStream(1)
	frData.SetBody(dFrame)
	_, err = frData.WriteTo(bw)
	coreh2.ReleaseFrameHeader(frData)
	require.NoError(t, err)
	require.NoError(t, bw.Flush())

	// Chromium & RFC 9113: Server MUST replenish connection flow-control window
	// by sending a WINDOW_UPDATE frame on stream 0!
	gotConnWindowUpdate := false
	for range 15 {
		respFr, rErr := readFrameWithTimeout(br, 500*time.Millisecond)
		if rErr != nil {
			break
		}
		if respFr.Type() == coreh2.FrameWindowUpdate && respFr.Stream() == 0 {
			gotConnWindowUpdate = true
			coreh2.ReleaseFrameHeader(respFr)
			break
		}
		coreh2.ReleaseFrameHeader(respFr)
	}

	assert.True(t, gotConnWindowUpdate, "server MUST send WINDOW_UPDATE on stream 0 for DATA received on closed stream to prevent receive window leak")
}

// RFC 9113 §6.5 & §6.5.2: Settings validation
func TestChromium_SpdySessionTest_Settings_InvalidValues(t *testing.T) {
	testCases := []struct {
		name          string
		frameBytes    []byte
		expectedError coreh2.ErrorCode
	}{
		{
			name: "SettingsAckWithPayload_FrameSizeError",
			// SETTINGS with ACK flag (0x1) and length 6 (> 0)
			frameBytes: []byte{
				0x00, 0x00, 0x06, // len 6
				0x04,                   // type SETTINGS
				0x01,                   // ACK flag
				0x00, 0x00, 0x00, 0x00, // stream 0
				0x00, 0x05, 0x00, 0x00, 0x40, 0x00,
			},
			expectedError: coreh2.FrameSizeError,
		},
		{
			name: "SettingsLengthNotMultipleOf6_FrameSizeError",
			// SETTINGS with length 5 (not multiple of 6)
			frameBytes: []byte{
				0x00, 0x00, 0x05, // len 5
				0x04,                   // type SETTINGS
				0x00,                   // flags 0
				0x00, 0x00, 0x00, 0x00, // stream 0
				0x00, 0x05, 0x00, 0x00, 0x40,
			},
			expectedError: coreh2.FrameSizeError,
		},
		{
			name: "SettingsMaxFrameSizeBelow16384_ProtocolError",
			// SETTINGS_MAX_FRAME_SIZE = 1000 (< 16384)
			frameBytes: []byte{
				0x00, 0x00, 0x06,
				0x04,
				0x00,
				0x00, 0x00, 0x00, 0x00,
				0x00, 0x05, 0x00, 0x00, 0x03, 0xe8, // id 5, val 1000
			},
			expectedError: coreh2.ProtocolError,
		},
		{
			name: "SettingsEnablePushInvalidValue_ProtocolError",
			// SETTINGS_ENABLE_PUSH = 2 (must be 0 or 1)
			frameBytes: []byte{
				0x00, 0x00, 0x06,
				0x04,
				0x00,
				0x00, 0x00, 0x00, 0x00,
				0x00, 0x02, 0x00, 0x00, 0x00, 0x02, // id 2, val 2
			},
			expectedError: coreh2.ProtocolError,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			clientConn, _, cleanup := setupServerClient(t, func(req *h2.ServerRequest, res *h2.ServerResponse) error {
				res.StatusCode = status.OK
				return nil
			})
			defer cleanup()

			bw := bufio.NewWriter(clientConn)
			br := bufio.NewReader(clientConn)

			_, err := bw.Write(tc.frameBytes)
			require.NoError(t, err)
			require.NoError(t, bw.Flush())

			gotExpectedGoAway := false
			for range 15 {
				respFr, rErr := readFrameWithTimeout(br, 500*time.Millisecond)
				if rErr != nil {
					break
				}
				if respFr.Type() == coreh2.FrameGoAway {
					ga := respFr.Body().(*coreh2.GoAway)
					if ga.Code() == tc.expectedError {
						gotExpectedGoAway = true
					}
					coreh2.ReleaseFrameHeader(respFr)
					break
				}
				coreh2.ReleaseFrameHeader(respFr)
			}

			assert.True(t, gotExpectedGoAway, "expected GOAWAY("+tc.expectedError.String()+") for "+tc.name)
		})
	}
}
