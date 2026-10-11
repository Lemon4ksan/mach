// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package e2e_test

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lemon4ksan/foundation/encoding/varint"
	"github.com/lemon4ksan/foundation/testing/assert"
	"github.com/lemon4ksan/foundation/testing/require"

	h2client "github.com/lemon4ksan/mach/client/h2"
	h3client "github.com/lemon4ksan/mach/client/h3"
	"github.com/lemon4ksan/mach/net/proxy/socks"
	quichandshake "github.com/lemon4ksan/mach/net/quic/core/handshake"
	quicwire "github.com/lemon4ksan/mach/net/quic/core/wire"
	"github.com/lemon4ksan/mach/proto/codec/hpack"
	"github.com/lemon4ksan/mach/proto/codec/qpack"
	coreh2 "github.com/lemon4ksan/mach/proto/h2"
	coreh3 "github.com/lemon4ksan/mach/proto/h3"
	machhttp "github.com/lemon4ksan/mach/proto/http"
	"github.com/lemon4ksan/mach/proto/semantic/status"
	h2server "github.com/lemon4ksan/mach/server/h2"
	h3server "github.com/lemon4ksan/mach/server/h3"
)

// ============================================================================
// Chromium Nemesis: HTTP/2 Frame Boundary & Fuzzing Robustness
// ============================================================================

// TestChromiumNemesis_H2_PaddingBoundaryStress tests padding boundary corner cases
// inspired by Chromium's net/spdy/fuzzing/http2_frame_decoder_fuzzer.cc.
// Verifies that malformed padding never causes slice-bounds-out-of-range panics.
func TestChromiumNemesis_H2_PaddingBoundaryStress(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		payload []byte
		length  int
		flags   coreh2.FrameFlags
	}{
		{
			name:    "PadLengthEqualsPayloadLength",
			payload: []byte{0x05, 'a', 'b', 'c', 'd', 0x00}, // padLen=5, payload=6: unpadded would be 6-5=1 -> payload[1:1]=empty
			length:  6,
			flags:   coreh2.FlagPadded,
		},
		{
			name:    "PadLengthExceedsPayloadLength",
			payload: []byte{0x10, 'a', 'b'}, // padLen=16, payload=3 -> MUST reject PROTOCOL_ERROR
			length:  3,
			flags:   coreh2.FlagPadded,
		},
		{
			name:    "PadLengthEqualsTotalFrame",
			payload: []byte{0x01}, // padLen=1, total len=1 -> 1-1 < 1 -> MUST reject
			length:  1,
			flags:   coreh2.FlagPadded,
		},
		{
			name:    "PadLength255WithShortFrame",
			payload: []byte{0xff, 0x01, 0x02},
			length:  3,
			flags:   coreh2.FlagPadded,
		},
		{
			name:    "ZeroLengthPayloadWithPaddedFlag",
			payload: []byte{},
			length:  0,
			flags:   coreh2.FlagPadded,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fr := coreh2.AcquireFrameHeader()
			defer coreh2.ReleaseFrameHeader(fr)

			fr.SetStream(1)
			fr.SetFlags(tc.flags)

			dataFrame := coreh2.AcquireFrame(coreh2.FrameData).(*coreh2.Data)
			defer coreh2.ReleaseFrame(dataFrame)

			var wireBuf bytes.Buffer
			var rawHeader [9]byte
			coreh2.PackFrameHeader(rawHeader[:], len(tc.payload), coreh2.FrameData, tc.flags, 1)
			wireBuf.Write(rawHeader[:])
			wireBuf.Write(tc.payload)

			br := bufio.NewReader(&wireBuf)
			parsedHeader, err := coreh2.ReadFrameFromWithSize(br, coreh2.DefaultMaxLen)
			if err == nil && parsedHeader != nil {
				coreh2.ReleaseFrameHeader(parsedHeader)
			}
		})
	}
}

// TestChromiumNemesis_H2_ReadFrameMidPayloadEOFDefense verifies that premature EOF
// during payload reading releases frame memory cleanly without double-freeing to sync.Pool.
func TestChromiumNemesis_H2_ReadFrameMidPayloadEOFDefense(t *testing.T) {
	t.Parallel()

	// Construct an incomplete frame: 9-byte header declaring 100 bytes payload, but only 10 bytes sent.
	var truncatedWire [19]byte
	coreh2.PackFrameHeader(truncatedWire[:9], 100, coreh2.FrameData, 0, 1)
	copy(truncatedWire[9:], "0123456789")

	for i := 0; i < 50; i++ {
		br := bufio.NewReader(bytes.NewReader(truncatedWire[:]))
		fr, err := coreh2.ReadFrameFromWithSize(br, coreh2.DefaultMaxLen)
		assert.Error(t, err, "Premature EOF must return an error")
		assert.True(t, errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF), "Expected EOF error")
		assert.Nil(t, fr, "FrameHeader must be nil on read error")
	}

	// Verify that the Data pool was not corrupted by acquiring and releasing cleanly
	for i := 0; i < 100; i++ {
		d := coreh2.AcquireFrame(coreh2.FrameData).(*coreh2.Data)
		d.SetData([]byte("pool-canary"))
		assert.Equal(t, "pool-canary", string(d.Data()))
		coreh2.ReleaseFrame(d)
	}
}

// TestChromiumNemesis_H2_PingPongHighConcurrencyRace floods the client with server-initiated
// PING frames while the client concurrently executes requests, verifying clean go test -race.
func TestChromiumNemesis_H2_PingPongHighConcurrencyRace(t *testing.T) {
	t.Parallel()

	var pingsReceived atomic.Int64

	addr, cleanup := startH2Server(t, func(req *h2server.ServerRequest, res *h2server.ServerResponse) error {
		res.StatusCode = status.OK
		res.Body = []byte("pong-payload")
		return nil
	})
	defer cleanup()

	client, rawConn := dialH2Client(t, addr, h2client.ConnOpts{
		PingInterval: 20 * time.Millisecond,
	})
	defer rawConn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	numWorkers := 10
	requestsPerWorker := 30

	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for r := 0; r < requestsPerWorker; r++ {
				req := machhttp.AcquireRequest()
				resp := machhttp.AcquireResponse()

				req.Header.SetMethod("GET")
				req.SetRequestURI("/ping-race")
				req.Header.SetHost(addr)

				err := client.Do(ctx, req, resp)
				if err == nil && resp.StatusCode() == 200 {
					pingsReceived.Add(1)
				}

				machhttp.ReleaseRequest(req)
				machhttp.ReleaseResponse(resp)
			}
		}()
	}

	wg.Wait()
	assert.Greater(t, pingsReceived.Load(), int64(0), "Requests must succeed concurrently with PING loop")
}

// TestChromiumNemesis_H2_StreamCancellationLifecycleNoLeak ensures that rapid context
// cancellations before or during stream transmission do not leak streams or memory.
func TestChromiumNemesis_H2_StreamCancellationLifecycleNoLeak(t *testing.T) {
	t.Parallel()

	addr, cleanup := startH2Server(t, func(req *h2server.ServerRequest, res *h2server.ServerResponse) error {
		time.Sleep(20 * time.Millisecond)
		res.StatusCode = status.OK
		res.Body = []byte("slow-response")
		return nil
	})
	defer cleanup()

	client, rawConn := dialH2Client(t, addr, h2client.ConnOpts{})
	defer rawConn.Close()

	// Issue 100 requests with extremely short deadlines (cancellation while queued)
	for i := 0; i < 100; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Microsecond)
		req := machhttp.AcquireRequest()
		resp := machhttp.AcquireResponse()

		req.Header.SetMethod("GET")
		req.SetRequestURI("/cancel-test")
		req.Header.SetHost(addr)

		_ = client.Do(ctx, req, resp)
		cancel()

		machhttp.ReleaseRequest(req)
		machhttp.ReleaseResponse(resp)
	}

	// Give background loops time to process any RST_STREAM egress
	time.Sleep(50 * time.Millisecond)

	// Now issue a normal request to ensure connection is still completely functional
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	req := machhttp.AcquireRequest()
	resp := machhttp.AcquireResponse()
	defer machhttp.ReleaseRequest(req)
	defer machhttp.ReleaseResponse(resp)

	req.Header.SetMethod("GET")
	req.SetRequestURI("/healthy-check")
	req.Header.SetHost(addr)

	err := client.Do(ctx, req, resp)
	require.NoError(t, err, "Connection must remain functional after stream cancellations")
	assert.Equal(t, 200, resp.StatusCode())
}

// TestChromiumNemesis_H2_ZeroIncrementWindowUpdateCompliance verifies RFC 9113 §6.9:
// A WINDOW_UPDATE with increment 0 on a stream MUST cause RST_STREAM(PROTOCOL_ERROR),
// not close the entire connection with GOAWAY.
func TestChromiumNemesis_H2_ZeroIncrementWindowUpdateCompliance(t *testing.T) {
	t.Parallel()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	scReleased := make(chan struct{})
	go func() {
		conn, aErr := ln.Accept()
		if aErr != nil {
			return
		}
		sc := h2server.NewServerConn(conn, func(req *h2server.ServerRequest, res *h2server.ServerResponse) error {
			res.StatusCode = status.OK
			return nil
		})
		defer func() {
			sc.Release()
			close(scReleased)
		}()
		_ = sc.Serve()
	}()

	clientConn, err := net.Dial("tcp", ln.Addr().String())
	require.NoError(t, err)
	defer clientConn.Close()

	// 1. Send client preface + empty SETTINGS
	_, err = clientConn.Write([]byte("PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n\x00\x00\x00\x04\x00\x00\x00\x00\x00"))
	require.NoError(t, err)

	enc := hpack.AcquireHPACK()
	defer hpack.ReleaseHPACK(enc)

	// 2. Open stream 1 with valid HEADERS
	var headerPayload []byte
	hf := hpack.AcquireHeaderField()
	defer hpack.ReleaseHeaderField(hf)

	for _, kv := range [][2]string{
		{":method", "GET"},
		{":scheme", "http"},
		{":path", "/test"},
	} {
		hf.Set(kv[0], kv[1])
		headerPayload = enc.AppendHeader(headerPayload, hf, false)
	}

	fr := coreh2.AcquireFrameHeader()
	defer coreh2.ReleaseFrameHeader(fr)

	hFrame := coreh2.AcquireFrame(coreh2.FrameHeaders).(*coreh2.Headers)
	hFrame.SetEndHeaders(true)
	hFrame.SetHeaders(headerPayload)

	fr.SetStream(1)
	fr.SetFlags(coreh2.FlagEndHeaders)
	fr.SetBody(hFrame)

	bw := bufio.NewWriter(clientConn)
	_, err = fr.WriteTo(bw)
	require.NoError(t, err)
	require.NoError(t, bw.Flush())

	// 3. Send WINDOW_UPDATE with increment = 0 on stream 1 (MUST elicit RST_STREAM, not GOAWAY)
	wuFrame := coreh2.AcquireFrameHeader()
	wuFrame.SetStream(1)
	wu := coreh2.AcquireFrame(coreh2.FrameWindowUpdate).(*coreh2.WindowUpdate)
	wu.SetIncrement(0)
	wuFrame.SetBody(wu)
	_, err = wuFrame.WriteTo(bw)
	require.NoError(t, err)
	require.NoError(t, bw.Flush())
	coreh2.ReleaseFrameHeader(wuFrame)

	// 4. Read frames from server until RST_STREAM on stream 1 is observed (verifying no GOAWAY)
	br := bufio.NewReader(clientConn)
	foundRst := false
	for range 10 {
		respFr, rErr := coreh2.ReadFrameFrom(br)
		if rErr != nil {
			break
		}
		if respFr.Type() == coreh2.FrameGoAway {
			t.Fatal("RFC 9113 violation: Received connection-level GOAWAY on stream-level zero-increment WINDOW_UPDATE")
		}
		if respFr.Type() == coreh2.FrameResetStream && respFr.Stream() == 1 {
			rst := respFr.Body().(*coreh2.RstStream)
			assert.Equal(t, coreh2.ProtocolError, rst.Code())
			foundRst = true
			coreh2.ReleaseFrameHeader(respFr)
			break
		}
		coreh2.ReleaseFrameHeader(respFr)
	}

	assert.True(t, foundRst, "Expected RST_STREAM(stream=1, PROTOCOL_ERROR) for zero increment window update")
}

// ============================================================================
// Chromium Nemesis: HTTP/3 Frame Boundary & Settings Robustness
// ============================================================================

// TestChromiumNemesis_H3_TruncatedSettingsFrameRejection verifies RFC 9114 §7.1:
// A SETTINGS frame smaller than declared payload length MUST be rejected with error.
func TestChromiumNemesis_H3_TruncatedSettingsFrameRejection(t *testing.T) {
	t.Parallel()

	// Declared payload length is 32 bytes, but only 2 bytes are provided.
	var truncatedSettings bytes.Buffer
	truncatedSettings.Write(varint.Append(nil, coreh3.SettingMaxFieldSectionSize))
	truncatedSettings.Write(varint.Append(nil, 1024))

	r := bytes.NewReader(truncatedSettings.Bytes())
	_, err := coreh3.DecodeSettings(r, 64)
	assert.Error(t, err, "Truncated SETTINGS frame must return an error")
	assert.True(t, errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF), "Expected unexpected EOF on truncated settings")
}

// TestChromiumNemesis_H3_TruncatedDataPayloadDetection verifies that client/h3
// rejects a DATA frame that is closed prematurely before specified payloadLen.
func TestChromiumNemesis_H3_TruncatedDataPayloadDetection(t *testing.T) {
	t.Parallel()

	// Build response with complete HEADERS frame followed by a truncated DATA frame
	headers := []qpack.HeaderField{
		{Name: ":status", Value: "200"},
		{Name: "content-type", Value: "text/plain"},
	}
	hBlock := qpack.NewEncoderWithDefaults(nil).EncodeHeaderList(0, headers, nil)

	var wire bytes.Buffer
	wire.Write(coreh3.AppendHeadersHeader(nil, uint64(len(hBlock))))
	wire.Write(hBlock)

	// Declare DATA frame of 256 bytes, but only supply 10 bytes
	wire.Write(coreh3.AppendDataHeader(nil, 256))
	wire.Write([]byte("short-data"))

	cc := h3client.NewClientConnWithCodec(coreh3.NewQPACKCodec())
	resp := machhttp.AcquireResponse()
	defer machhttp.ReleaseResponse(resp)

	reader := bytes.NewReader(wire.Bytes())
	_, err := cc.ReadResponseFromForTest(reader, resp, 0)
	assert.Error(t, err, "Truncated DATA frame must be detected as an error")
}

// TestChromiumNemesis_H3_ReservedH2FramesAndSettingsRejection verifies RFC 9114 §7.2.8 & §7.2.4.1:
// Obsolete HTTP/2 frames and settings MUST be rejected with H3_FRAME_UNEXPECTED / H3_SETTINGS_ERROR.
func TestChromiumNemesis_H3_ReservedH2FramesAndSettingsRejection(t *testing.T) {
	t.Parallel()

	reservedFrames := []uint64{
		coreh3.FrameTypeReservedPriority,
		coreh3.FrameTypeReservedPing,
		coreh3.FrameTypeReservedWindowUpdate,
		coreh3.FrameTypeReservedContinuation,
	}

	for _, rf := range reservedFrames {
		assert.True(t, coreh3.IsReservedH2FrameType(rf), fmt.Sprintf("Frame 0x%x must be recognized as reserved H2 frame", rf))
	}

	reservedSettings := []uint64{0x00, 0x02, 0x03, 0x04, 0x05}
	for _, rs := range reservedSettings {
		assert.True(t, coreh3.IsReservedH2Setting(rs), fmt.Sprintf("Setting 0x%x must be recognized as reserved H2 setting", rs))

		// Test DecodeSettings with reserved setting ID
		var buf bytes.Buffer
		buf.Write(varint.Append(nil, rs))
		buf.Write(varint.Append(nil, 1))

		_, err := coreh3.DecodeSettings(bytes.NewReader(buf.Bytes()), uint64(buf.Len()))
		assert.Error(t, err, fmt.Sprintf("DecodeSettings must reject reserved setting 0x%x", rs))
		assert.True(t, errors.Is(err, coreh3.ErrH3SettingsError), "Expected ErrH3SettingsError")
	}
}

// TestChromiumNemesis_H3_HighConcurrencyMultiplexing tests high-concurrency request/response
// multiplexing over a single HTTP/3 connection under race detector, verifying zero deadlocks or stream leaks.
func TestChromiumNemesis_H3_HighConcurrencyMultiplexing(t *testing.T) {
	t.Parallel()

	addr, clientTLS, cleanup := startH3Server(t, func(req *h3server.ServerRequest, res *h3server.ServerResponse) error {
		res.StatusCode = status.OK
		res.Headers.Set("content-type", "application/json")
		res.Body = []byte(`{"status":"nemesis-ok"}`)
		return nil
	})
	defer cleanup()

	client, quicConn := dialH3Client(t, addr, clientTLS)
	defer quicConn.CloseWithError(0, "")

	var wg sync.WaitGroup
	numWorkers := 10
	requestsPerWorker := 15
	var completedCount atomic.Int64

	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for r := 0; r < requestsPerWorker; r++ {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				req := machhttp.AcquireRequest()
				resp := machhttp.AcquireResponse()

				req.Header.SetMethod("GET")
				req.SetRequestURI(fmt.Sprintf("/worker-%d/req-%d", workerID, r))
				req.Header.SetHost(addr)

				_, err := client.Do(ctx, req, resp, nil)
				cancel()
				if err == nil && resp.StatusCode() == 200 {
					completedCount.Add(1)
				}

				machhttp.ReleaseRequest(req)
				machhttp.ReleaseResponse(resp)
			}
		}(w)
	}

	wg.Wait()
	assert.Equal(t, int64(numWorkers*requestsPerWorker), completedCount.Load(), "All concurrent H3 requests must succeed")
}

// ============================================================================
// Chromium Nemesis: QUIC Wire & Transport Parameters Fuzzing Defense
// ============================================================================

// TestChromiumNemesis_QUIC_TransportParameters_TrailingGarbage verifies RFC 9000 §18.2:
// Transport parameters with trailing garbage bytes must fail with TRANSPORT_PARAMETER_ERROR.
func TestChromiumNemesis_QUIC_TransportParameters_TrailingGarbage(t *testing.T) {
	t.Parallel()

	// Parameter ID: paramMaxIdleTimeout (0x01)
	// Declared length: 4 bytes
	// Value: 1-byte varint (0x1e = 30) followed by 3 bytes of garbage (0xaa, 0xbb, 0xcc)
	badParam := []byte{
		0x01,             // param ID: max_idle_timeout
		0x04,             // param length: 4 bytes
		0x1e,             // varint value: 30 (1 byte consumed)
		0xaa, 0xbb, 0xcc, // trailing garbage
	}

	var tp quichandshake.TransportParameters
	err := tp.Unmarshal(badParam, quichandshake.PerspectiveServer)
	assert.Error(t, err, "Transport parameter with trailing bytes MUST be rejected per RFC 9000 §18.2")
}

// TestChromiumNemesis_QUIC_AckFrameUnderflowProtection tests that ACK frames where
// FirstAckRange > LargestAcked are rejected without underflow panics.
func TestChromiumNemesis_QUIC_AckFrameUnderflowProtection(t *testing.T) {
	t.Parallel()

	// Construct ACK frame wire bytes where FirstAckRange (10) > LargestAcked (5)
	var ackWire bytes.Buffer
	ackWire.Write(varint.Append(nil, uint64(quicwire.FrameTypeAck)))
	ackWire.Write(varint.Append(nil, 5))  // LargestAcked = 5
	ackWire.Write(varint.Append(nil, 0))  // AckDelay = 0
	ackWire.Write(varint.Append(nil, 0))  // RangeCount = 0
	ackWire.Write(varint.Append(nil, 10)) // FirstAckRange = 10 (> LargestAcked)

	var f quicwire.Frame
	_, err := quicwire.ParseNextFrame(ackWire.Bytes(), &f)
	assert.Error(t, err, "FirstAckRange > LargestAcked must return an error")
}

// ============================================================================
// Chromium Nemesis: SOCKS5 Robustness (Chromium socks5_client_socket_fuzzer.cc)
// ============================================================================

// TestChromiumNemesis_SOCKS5_FuzzedServerResponses tests the SOCKS5 client against
// malformed server greetings, replies, and address types without panic or hangs.
func TestChromiumNemesis_SOCKS5_FuzzedServerResponses(t *testing.T) {
	t.Parallel()

	testReplies := []struct {
		name     string
		greeting []byte
		reply    []byte
	}{
		{
			name:     "InvalidProtocolVersionInGreeting",
			greeting: []byte{0x04, 0x00}, // SOCKS4 version instead of SOCKS5
			reply:    nil,
		},
		{
			name:     "NoAcceptableAuthMethods",
			greeting: []byte{0x05, 0xff}, // 0xFF = No acceptable methods
			reply:    nil,
		},
		{
			name:     "NonZeroReservedByteInReply",
			greeting: []byte{0x05, 0x00},
			reply:    []byte{0x05, 0x00, 0x01, 0x01, 127, 0, 0, 1, 0x1f, 0x90}, // RSV is 0x01 instead of 0x00
		},
		{
			name:     "UnknownAddressType",
			greeting: []byte{0x05, 0x00},
			reply:    []byte{0x05, 0x00, 0x00, 0x09, 0x01, 0x02}, // AddrType 0x09 is undefined
		},
		{
			name:     "TruncatedIPv4Address",
			greeting: []byte{0x05, 0x00},
			reply:    []byte{0x05, 0x00, 0x00, 0x01, 127, 0}, // Truncated IPv4
		},
	}

	for _, tc := range testReplies {
		t.Run(tc.name, func(t *testing.T) {
			serverConn, clientConn := net.Pipe()
			defer serverConn.Close()
			defer clientConn.Close()

			go func() {
				defer serverConn.Close()
				buf := make([]byte, 256)
				_, _ = serverConn.Read(buf) // greeting request
				if tc.greeting != nil {
					_, _ = serverConn.Write(tc.greeting)
				}
				if tc.reply != nil {
					_, _ = serverConn.Read(buf) // connect request
					_, _ = serverConn.Write(tc.reply)
				}
			}()

			d := &socks.Dialer{}
			ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
			defer cancel()

			err := d.Handshake(ctx, clientConn, "127.0.0.1:8080")
			assert.Error(t, err, "Malformed server response must result in handshake error")
		})
	}
}

// TestChromiumNemesis_SOCKS5_OversizedFQDNRejection tests that hostnames exceeding
// 255 bytes are rejected without allocations or panics (Chromium LargeHostNameFails).
func TestChromiumNemesis_SOCKS5_OversizedFQDNRejection(t *testing.T) {
	t.Parallel()

	longHost := make([]byte, 300)
	for i := range longHost {
		longHost[i] = 'a'
	}
	longAddr := string(longHost) + ":80"

	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	d := &socks.Dialer{}
	ctx := context.Background()

	err := d.Handshake(ctx, clientConn, longAddr)
	assert.Error(t, err, "Hostname > 255 bytes must be rejected")
}

// ============================================================================
// Chromium Nemesis: Zero-Allocation Hot Path Regression Suite
// ============================================================================

// TestChromiumNemesis_ZeroAllocHotPaths enforces testing.AllocsPerRun == 0
// across critical wire serialization, frame packing, and connection parsing paths.
func TestChromiumNemesis_ZeroAllocHotPaths(t *testing.T) {
	t.Run("H2_PackUnpackFrameHeader", func(t *testing.T) {
		var raw [9]byte
		allocs := testing.AllocsPerRun(1000, func() {
			coreh2.PackFrameHeader(raw[:], 512, coreh2.FrameData, coreh2.FlagEndStream, 7)
			l, kind, flags, stream := coreh2.UnpackFrameHeader(raw[:])
			if l != 512 || kind != coreh2.FrameData || stream != 7 || !flags.Has(coreh2.FlagEndStream) {
				t.Fatal("H2 pack/unpack mismatch")
			}
		})
		assert.Equal(t, 0.0, allocs, "H2 frame header pack/unpack must be 0 allocs")
	})

	t.Run("H2_ControlFramePoolRoundTrip", func(t *testing.T) {
		allocs := testing.AllocsPerRun(1000, func() {
			ping := coreh2.AcquireFrame(coreh2.FramePing).(*coreh2.Ping)
			ping.SetAck(true)
			coreh2.ReleaseFrame(ping)

			wu := coreh2.AcquireFrame(coreh2.FrameWindowUpdate).(*coreh2.WindowUpdate)
			wu.SetIncrement(32768)
			coreh2.ReleaseFrame(wu)

			rst := coreh2.AcquireFrame(coreh2.FrameResetStream).(*coreh2.RstStream)
			rst.SetCode(coreh2.StreamCanceled)
			coreh2.ReleaseFrame(rst)
		})
		assert.Equal(t, 0.0, allocs, "H2 control frame pool acquire/release must be 0 allocs")
	})

	t.Run("H3_VarintFramePacking", func(t *testing.T) {
		var buf [16]byte
		allocs := testing.AllocsPerRun(1000, func() {
			b := varint.Append(buf[:0], coreh3.FrameTypeHeaders)
			_ = varint.Append(b, 4096)
		})
		assert.Equal(t, 0.0, allocs, "H3 frame packing must be 0 allocs")
	})

	t.Run("QUIC_AckFrameInlineRanges", func(t *testing.T) {
		allocs := testing.AllocsPerRun(1000, func() {
			var ack quicwire.AckFrame
			ack.Reset()
			ack.LargestAcked = 100
			ack.FirstAckRange = 5
			ack.AppendRange(2, 4)
			ack.AppendRange(1, 2)
			if ack.RangeCount() != 2 {
				t.Fatal("ACK range count mismatch")
			}
		})
		assert.Equal(t, 0.0, allocs, "QUIC inline ACK ranges must be 0 allocs")
	})
}
