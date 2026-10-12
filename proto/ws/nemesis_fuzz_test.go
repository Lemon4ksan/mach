// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ws_test

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/lemon4ksan/foundation/testing/assert"
	"github.com/lemon4ksan/foundation/testing/require"

	"github.com/lemon4ksan/mach/proto/ws"
)

// tcpPair establishes a bidirectional loopback TCP socket connection pair.
// Unlike unbuffered net.Pipe(), real TCP sockets provide kernel buffering and
// accurately model real-world full-duplex WebSocket network transport.
func tcpPair(t *testing.T) (client, server net.Conn) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	var sConn net.Conn
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		var acceptErr error
		sConn, acceptErr = ln.Accept()
		if acceptErr != nil {
			t.Errorf("accept error: %v", acceptErr)
		}
	}()

	cConn, err := net.Dial("tcp", ln.Addr().String())
	require.NoError(t, err)
	wg.Wait()
	return cConn, sConn
}

// ============================================================================
// 1. RFC 6455 Wire & Framing Violations: Masking Enforcement
// ============================================================================

// TestNemesis_UnmaskedClientFrame_Rejected verifies that an unmasked frame
// sent from a client to a server MUST be rejected with StatusProtocolError (1002)
// and fail the connection (RFC 6455 §5.1).
func TestNemesis_UnmaskedClientFrame_Rejected(t *testing.T) {
	t.Parallel()

	clientConnRaw, serverConnRaw := tcpPair(t)
	serverConn := ws.NewServerConn(serverConnRaw)
	defer serverConn.Close()
	defer clientConnRaw.Close()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		// Send raw unmasked text frame: FIN=1, Opcode=1, Mask=0, Len=5, "Hello"
		rawUnmasked := []byte{0x81, 0x05, 'H', 'e', 'l', 'l', 'o'}
		_, _ = clientConnRaw.Write(rawUnmasked)

		// Read server's Close frame response
		buf := make([]byte, 128)
		_ = clientConnRaw.SetReadDeadline(time.Now().Add(2 * time.Second))
		n, _ := clientConnRaw.Read(buf)
		if n >= 2 {
			h, hlen, err := ws.ParseHeader(buf[:n])
			if err == nil && n >= hlen+int(h.PayloadLen) {
				payload := make([]byte, h.PayloadLen)
				copy(payload, buf[hlen:hlen+int(h.PayloadLen)])
				if h.Masked {
					ws.Mask(payload, h.MaskKey)
				}
				closeCode, _, _ := ws.ParseClosePayload(payload)
				assert.Equal(t, ws.StatusProtocolError, closeCode, "server must reply with 1002 Protocol Error")
			}
		}
	}()

	_, _, err := serverConn.ReadMessage()
	wg.Wait()
	require.Error(t, err, "server MUST reject unmasked client frame")
	assert.True(t, errors.Is(err, ws.ErrMaskRequired) || errors.Is(err, ws.ErrProtocolViolation),
		fmt.Sprintf("expected ErrMaskRequired or ErrProtocolViolation, got: %v", err))
	assert.Equal(t, ws.StateClosed, serverConn.State(), "server connection must transition to closed")
}

// TestNemesis_MaskedServerFrame_Rejected verifies that a masked frame
// sent from a server to a client MUST be rejected with StatusProtocolError (1002)
// and fail the connection (RFC 6455 §5.1).
func TestNemesis_MaskedServerFrame_Rejected(t *testing.T) {
	t.Parallel()

	clientConnRaw, serverConnRaw := tcpPair(t)
	clientConn := ws.NewClientConn(clientConnRaw)
	defer clientConn.Close()
	defer serverConnRaw.Close()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		maskKey := [4]byte{0x12, 0x34, 0x56, 0x78}
		payload := []byte("Hello")
		ws.Mask(payload, maskKey)

		rawMasked := []byte{0x81, 0x85, maskKey[0], maskKey[1], maskKey[2], maskKey[3]}
		rawMasked = append(rawMasked, payload...)
		_, _ = serverConnRaw.Write(rawMasked)

		buf := make([]byte, 128)
		_ = serverConnRaw.SetReadDeadline(time.Now().Add(2 * time.Second))
		_, _ = serverConnRaw.Read(buf)
	}()

	_, _, err := clientConn.ReadMessage()
	wg.Wait()
	require.Error(t, err, "client MUST reject masked frame from server")
	assert.True(t, errors.Is(err, ws.ErrMaskForbidden) || errors.Is(err, ws.ErrProtocolViolation),
		fmt.Sprintf("expected ErrMaskForbidden or ErrProtocolViolation, got: %v", err))
	assert.Equal(t, ws.StateClosed, clientConn.State(), "client connection must transition to closed")
}

// ============================================================================
// 2. Control Frame Rules: Size, Fragmentation, and Reserved Opcodes
// ============================================================================

// TestNemesis_ControlFrame_PayloadTooLarge verifies that control frames with
// payload length > 125 bytes MUST be rejected with StatusProtocolError (1002)
// (RFC 6455 §5.5, Chromium 126BytePong test).
func TestNemesis_ControlFrame_PayloadTooLarge(t *testing.T) {
	t.Parallel()

	raw126BytePong := make([]byte, 4+126)
	raw126BytePong[0] = 0x8A // FIN=1, Opcode=Pong (0xA)
	raw126BytePong[1] = 126  // 2-byte extended length
	binary.BigEndian.PutUint16(raw126BytePong[2:4], 126)

	_, _, err := ws.ParseHeader(raw126BytePong[:4])
	assert.True(t, errors.Is(err, ws.ErrControlFrameTooLarge), "ParseHeader must reject control frame > 125 bytes")

	clientConnRaw, serverConnRaw := tcpPair(t)
	clientConn := ws.NewClientConn(clientConnRaw)
	defer clientConn.Close()
	defer serverConnRaw.Close()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _ = serverConnRaw.Write(raw126BytePong)
		buf := make([]byte, 128)
		_ = serverConnRaw.SetReadDeadline(time.Now().Add(2 * time.Second))
		_, _ = serverConnRaw.Read(buf)
	}()

	_, _, err = clientConn.ReadMessage()
	wg.Wait()
	require.Error(t, err, "ReadMessage must reject 126-byte control frame")
	assert.True(t, errors.Is(err, ws.ErrControlFrameTooLarge) || errors.Is(err, ws.ErrProtocolViolation),
		fmt.Sprintf("expected ErrControlFrameTooLarge, got: %v", err))
}

// TestNemesis_ControlFrame_Fragmented_Rejected verifies that fragmented control frames
// (FIN=0 with Opcode >= 0x8) MUST fail connection with StatusProtocolError (1002)
// (RFC 6455 §5.5, Chromium PingFrameWithoutFin test).
func TestNemesis_ControlFrame_Fragmented_Rejected(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		header []byte
	}{
		{"PingWithoutFin", []byte{0x09, 0x00}},
		{"PongWithoutFin", []byte{0x0A, 0x00}},
		{"CloseWithoutFin", []byte{0x08, 0x00}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := ws.ParseHeader(tc.header)
			assert.True(
				t,
				errors.Is(err, ws.ErrControlFrameFragmented),
				"ParseHeader must reject control frame without FIN",
			)

			clientConnRaw, serverConnRaw := tcpPair(t)
			clientConn := ws.NewClientConn(clientConnRaw)
			defer clientConn.Close()
			defer serverConnRaw.Close()

			var wg sync.WaitGroup
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, _ = serverConnRaw.Write(tc.header)
				buf := make([]byte, 128)
				_ = serverConnRaw.SetReadDeadline(time.Now().Add(2 * time.Second))
				_, _ = serverConnRaw.Read(buf)
			}()

			_, _, err = clientConn.ReadMessage()
			wg.Wait()
			require.Error(t, err, "ReadMessage must reject fragmented control frame")
			assert.True(
				t,
				errors.Is(err, ws.ErrControlFrameFragmented) || errors.Is(err, ws.ErrProtocolViolation),
				"expected error",
			)
		})
	}
}

// TestNemesis_ReservedOpcodes_Rejected verifies that reserved opcodes (0x3-0x7, 0xB-0xF)
// MUST fail the connection with StatusProtocolError (1002) (RFC 6455 §5.2).
func TestNemesis_ReservedOpcodes_Rejected(t *testing.T) {
	t.Parallel()

	reservedOpcodes := []ws.Opcode{
		0x3, 0x4, 0x5, 0x6, 0x7, // Reserved data opcodes
		0xB, 0xC, 0xD, 0xE, 0xF, // Reserved control opcodes
	}

	for _, op := range reservedOpcodes {
		assert.True(t, op.IsReserved(), fmt.Sprintf("opcode 0x%X must report IsReserved() == true", byte(op)))
		assert.False(t, op.IsValid(), fmt.Sprintf("opcode 0x%X must report IsValid() == false", byte(op)))

		clientConnRaw, serverConnRaw := tcpPair(t)
		clientConn := ws.NewClientConn(clientConnRaw)

		var wg sync.WaitGroup
		wg.Add(1)
		go func(opByte byte) {
			defer wg.Done()
			rawFrame := []byte{0x80 | opByte, 0x00}
			_, _ = serverConnRaw.Write(rawFrame)
			buf := make([]byte, 128)
			_ = serverConnRaw.SetReadDeadline(time.Now().Add(2 * time.Second))
			_, _ = serverConnRaw.Read(buf)
			_ = serverConnRaw.Close()
		}(byte(op))

		_, _, err := clientConn.ReadMessage()
		wg.Wait()
		_ = clientConn.Close()

		require.Error(t, err, fmt.Sprintf("reserved opcode 0x%X must cause ReadMessage to fail", byte(op)))
		assert.True(t, errors.Is(err, ws.ErrInvalidOpcode) || errors.Is(err, ws.ErrProtocolViolation),
			fmt.Sprintf("expected ErrInvalidOpcode for 0x%X, got: %v", byte(op), err))
	}
}

// ============================================================================
// 3. Reserved Bits (RSV1, RSV2, RSV3) Violations
// ============================================================================

// TestNemesis_ReservedBits_NonZero_Rejected verifies that RSV1, RSV2, or RSV3
// set when no extension was negotiated MUST fail with StatusProtocolError (1002)
// (RFC 6455 §5.2).
func TestNemesis_ReservedBits_NonZero_Rejected(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		b0   byte
	}{
		{"RSV1_Set", 0xC1},      // 1100 0001 (FIN=1, RSV1=1, Opcode=Text)
		{"RSV2_Set", 0xA1},      // 1010 0001 (FIN=1, RSV2=1, Opcode=Text)
		{"RSV3_Set", 0x91},      // 1001 0001 (FIN=1, RSV3=1, Opcode=Text)
		{"RSV1_RSV2_Set", 0xE1}, // 1110 0001
		{"All_RSV_Set", 0xF1},   // 1111 0001
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			clientConnRaw, serverConnRaw := tcpPair(t)
			clientConn := ws.NewClientConn(clientConnRaw)
			defer clientConn.Close()
			defer serverConnRaw.Close()

			var wg sync.WaitGroup
			wg.Add(1)
			go func() {
				defer wg.Done()
				rawFrame := []byte{tc.b0, 0x00}
				_, _ = serverConnRaw.Write(rawFrame)
				buf := make([]byte, 128)
				_ = serverConnRaw.SetReadDeadline(time.Now().Add(2 * time.Second))
				_, _ = serverConnRaw.Read(buf)
			}()

			_, _, err := clientConn.ReadMessage()
			wg.Wait()
			require.Error(t, err, "non-zero RSV bit must fail connection")
			assert.True(t, errors.Is(err, ws.ErrRSVSet) || errors.Is(err, ws.ErrProtocolViolation),
				fmt.Sprintf("expected ErrRSVSet, got: %v", err))
		})
	}
}

// TestNemesis_RSV1_OnControlFrame_Rejected verifies that even when permessage-deflate
// is enabled, RSV1 MUST NOT be set on control frames (RFC 6455 §5.5, RFC 7692 §5.1).
func TestNemesis_RSV1_OnControlFrame_Rejected(t *testing.T) {
	t.Parallel()

	clientConnRaw, serverConnRaw := tcpPair(t)
	clientConn := ws.NewClientConn(clientConnRaw)
	clientConn.EnableCompression(true)
	defer clientConn.Close()
	defer serverConnRaw.Close()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		// Ping with RSV1=1 (0xC9 = 1100 1001)
		rawFrame := []byte{0xC9, 0x00}
		_, _ = serverConnRaw.Write(rawFrame)
		buf := make([]byte, 128)
		_ = serverConnRaw.SetReadDeadline(time.Now().Add(2 * time.Second))
		_, _ = serverConnRaw.Read(buf)
	}()

	_, _, err := clientConn.ReadMessage()
	wg.Wait()
	require.Error(t, err, "RSV1 on control frame must be rejected")
	assert.True(t, errors.Is(err, ws.ErrRSVSet) || errors.Is(err, ws.ErrProtocolViolation), "expected error")
}

// ============================================================================
// 4. Fragmentation State Machine Violations
// ============================================================================

// TestNemesis_ContinuationWithoutStart_Rejected verifies that a Continuation frame
// received when no fragmented message is in progress MUST fail with StatusProtocolError (1002)
// (RFC 6455 §5.4).
func TestNemesis_ContinuationWithoutStart_Rejected(t *testing.T) {
	t.Parallel()

	clientConnRaw, serverConnRaw := tcpPair(t)
	clientConn := ws.NewClientConn(clientConnRaw)
	defer clientConn.Close()
	defer serverConnRaw.Close()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		rawContinuation := []byte{0x80, 0x05, 'c', 'h', 'u', 'n', 'k'}
		_, _ = serverConnRaw.Write(rawContinuation)
		buf := make([]byte, 128)
		_ = serverConnRaw.SetReadDeadline(time.Now().Add(2 * time.Second))
		_, _ = serverConnRaw.Read(buf)
	}()

	_, _, err := clientConn.ReadMessage()
	wg.Wait()
	require.Error(t, err, "continuation frame without starting frame must be rejected")
	assert.True(
		t,
		errors.Is(err, ws.ErrUnexpectedContinuation) || errors.Is(err, ws.ErrProtocolViolation),
		"expected error",
	)
}

// TestNemesis_NewDataFrameBeforeCompletion_Rejected verifies that receiving a new
// Text or Binary frame while a fragmented message is incomplete MUST fail with StatusProtocolError (1002)
// (RFC 6455 §5.4).
func TestNemesis_NewDataFrameBeforeCompletion_Rejected(t *testing.T) {
	t.Parallel()

	clientConnRaw, serverConnRaw := tcpPair(t)
	clientConn := ws.NewClientConn(clientConnRaw)
	defer clientConn.Close()
	defer serverConnRaw.Close()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		// 1. Text frame with FIN=0 (fragment 1)
		frag1 := []byte{0x01, 0x04, 'p', 'a', 'r', 't'}
		_, _ = serverConnRaw.Write(frag1)

		// 2. Instead of Continuation frame, send a new Binary frame (Opcode=2, FIN=1)
		illegalNewFrame := []byte{0x82, 0x04, 0x01, 0x02, 0x03, 0x04}
		_, _ = serverConnRaw.Write(illegalNewFrame)

		buf := make([]byte, 128)
		_ = serverConnRaw.SetReadDeadline(time.Now().Add(2 * time.Second))
		_, _ = serverConnRaw.Read(buf)
	}()

	_, _, err := clientConn.ReadMessage()
	wg.Wait()
	require.Error(t, err, "new data frame before completion of fragmented message must be rejected")
	assert.True(
		t,
		errors.Is(err, ws.ErrExpectedContinuation) || errors.Is(err, ws.ErrProtocolViolation),
		"expected error",
	)
}

// ============================================================================
// 5. Payload Validation: UTF-8 Enforcements
// ============================================================================

// TestNemesis_InvalidUTF8_TextFrame_Rejected verifies that invalid UTF-8 sequences
// in Text frames (Opcode 0x1) MUST fail the connection with StatusInvalidFramePayloadData (1007)
// (RFC 6455 §5.6, §8.1).
func TestNemesis_InvalidUTF8_TextFrame_Rejected(t *testing.T) {
	t.Parallel()

	invalidSequences := [][]byte{
		{0xFF, 0xFE},                    // Invalid UTF-8 BOM/surrogate
		{0xC0, 0x80},                    // Overlong NUL
		{0xED, 0xA0, 0x80},              // UTF-16 surrogate half (U+D800)
		{0xF4, 0x90, 0x80, 0x80},        // Beyond Unicode max (U+110000)
		{'t', 'e', 'x', 't', 0x80, 'a'}, // Stray continuation byte
	}

	for _, invalidSeq := range invalidSequences {
		clientConnRaw, serverConnRaw := tcpPair(t)
		clientConn := ws.NewClientConn(clientConnRaw)

		var wg sync.WaitGroup
		wg.Add(1)
		go func(badPayload []byte) {
			defer wg.Done()
			rawFrame := []byte{0x81, byte(len(badPayload))}
			rawFrame = append(rawFrame, badPayload...)
			_, _ = serverConnRaw.Write(rawFrame)

			var respBuf [128]byte
			_ = serverConnRaw.SetReadDeadline(time.Now().Add(2 * time.Second))
			n, _ := serverConnRaw.Read(respBuf[:])
			if n >= 2 {
				h, hlen, err := ws.ParseHeader(respBuf[:n])
				if err == nil && n >= hlen+int(h.PayloadLen) {
					closePayload := make([]byte, h.PayloadLen)
					copy(closePayload, respBuf[hlen:hlen+int(h.PayloadLen)])
					if h.Masked {
						ws.Mask(closePayload, h.MaskKey)
					}
					closeCode, _, _ := ws.ParseClosePayload(closePayload)
					assert.Equal(t, ws.StatusInvalidFramePayloadData, closeCode,
						"close status must be 1007 (Invalid Frame Payload Data)")
				}
			}
			_ = serverConnRaw.Close()
		}(invalidSeq)

		_, _, err := clientConn.ReadMessage()
		wg.Wait()
		_ = clientConn.Close()

		require.Error(t, err, "invalid UTF-8 in Text frame must be rejected")
		assert.True(t, errors.Is(err, ws.ErrInvalidUTF8) || errors.Is(err, ws.ErrInvalidPayload), "expected error")
	}
}

// TestNemesis_InvalidUTF8_CloseReason_Rejected verifies that an invalid UTF-8 sequence
// in a Close frame's reason payload MUST fail with StatusInvalidFramePayloadData (1007)
// (RFC 6455 §7.4, §8.1).
func TestNemesis_InvalidUTF8_CloseReason_Rejected(t *testing.T) {
	t.Parallel()

	// Close frame: Code=1000 (0x03E8), Reason=invalid UTF-8 [0xFF, 0xFE]
	badClosePayload := []byte{0x03, 0xE8, 0xFF, 0xFE}
	_, _, err := ws.ParseClosePayload(badClosePayload)
	require.Error(t, err, "ParseClosePayload must reject invalid UTF-8 reason")
	assert.True(t, errors.Is(err, ws.ErrInvalidUTF8), "expected error")

	clientConnRaw, serverConnRaw := tcpPair(t)
	clientConn := ws.NewClientConn(clientConnRaw)
	defer clientConn.Close()
	defer serverConnRaw.Close()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		rawCloseFrame := []byte{0x88, byte(len(badClosePayload))}
		rawCloseFrame = append(rawCloseFrame, badClosePayload...)
		_, _ = serverConnRaw.Write(rawCloseFrame)

		var resp [128]byte
		_ = serverConnRaw.SetReadDeadline(time.Now().Add(2 * time.Second))
		n, _ := serverConnRaw.Read(resp[:])
		if n >= 2 {
			h, hlen, err := ws.ParseHeader(resp[:n])
			if err == nil && n >= hlen+int(h.PayloadLen) {
				closePayload := make([]byte, h.PayloadLen)
				copy(closePayload, resp[hlen:hlen+int(h.PayloadLen)])
				if h.Masked {
					ws.Mask(closePayload, h.MaskKey)
				}
				respCode, _, _ := ws.ParseClosePayload(closePayload)
				assert.Equal(t, ws.StatusInvalidFramePayloadData, respCode)
			}
		}
	}()

	_, _, err = clientConn.ReadMessage()
	wg.Wait()
	require.Error(t, err, "ReadMessage must reject Close frame with invalid UTF-8 reason")
	assert.True(t, errors.Is(err, ws.ErrInvalidUTF8), "expected error")
}

// ============================================================================
// 6. Close Status Codes: Forbidden and Reserved Codes on Wire
// ============================================================================

// TestNemesis_InvalidCloseCodes_Rejected verifies that forbidden codes on the wire
// (0-999, 1004, 1005, 1006, 1014, 1015, 1016-2999, >= 5000) MUST fail with 1002 Protocol Error
// (RFC 6455 §7.4.2).
func TestNemesis_InvalidCloseCodes_Rejected(t *testing.T) {
	t.Parallel()

	forbiddenCodes := []uint16{
		0, 1, 999, // 0-999: Not used
		1004,             // Reserved
		1005,             // No Status Received (forbidden on wire)
		1006,             // Abnormal Closure (forbidden on wire)
		1014,             // Bad Gateway (forbidden on wire in RFC 6455)
		1015,             // TLS Handshake (forbidden on wire)
		1016, 2000, 2999, // 1016-2999: Unassigned protocol reserved
		5000, 6000, 65535, // >= 5000: Out of range
	}

	for _, code := range forbiddenCodes {
		cc := ws.CloseCode(code)
		assert.False(t, cc.IsValidForWire(), fmt.Sprintf("code %d must NOT be valid for wire transmission", code))

		var payload [2]byte
		binary.BigEndian.PutUint16(payload[:], code)
		_, _, err := ws.ParseClosePayload(payload[:])
		require.Error(t, err, fmt.Sprintf("ParseClosePayload must reject forbidden code %d", code))
		assert.True(t, errors.Is(err, ws.ErrInvalidCloseCode), "expected error")

		clientConnRaw, serverConnRaw := tcpPair(t)
		clientConn := ws.NewClientConn(clientConnRaw)

		var wg sync.WaitGroup
		wg.Add(1)
		go func(c uint16) {
			defer wg.Done()
			rawFrame := []byte{0x88, 0x02, byte(c >> 8), byte(c)}
			_, _ = serverConnRaw.Write(rawFrame)
			buf := make([]byte, 128)
			_ = serverConnRaw.SetReadDeadline(time.Now().Add(2 * time.Second))
			_, _ = serverConnRaw.Read(buf)
			_ = serverConnRaw.Close()
		}(code)

		_, _, err = clientConn.ReadMessage()
		wg.Wait()
		_ = clientConn.Close()

		require.Error(t, err, fmt.Sprintf("connection must fail on forbidden close code %d", code))
		assert.True(
			t,
			errors.Is(err, ws.ErrInvalidCloseCode) || errors.Is(err, ws.ErrProtocolViolation),
			"expected error",
		)
	}
}

// TestNemesis_OneByteClosePayload_Rejected verifies that a Close frame with
// a 1-byte payload is invalid and MUST fail with 1002 (RFC 6455 §7.4).
func TestNemesis_OneByteClosePayload_Rejected(t *testing.T) {
	t.Parallel()

	oneBytePayload := []byte{0x03}
	_, _, err := ws.ParseClosePayload(oneBytePayload)
	require.Error(t, err, "1-byte close payload must be rejected")
	assert.True(
		t,
		errors.Is(err, ws.ErrInvalidClosePayload) || errors.Is(err, ws.ErrProtocolViolation),
		"expected error",
	)
}

// ============================================================================
// 7. Non-Minimal Extended Payload Length Enforcement
// ============================================================================

// TestNemesis_NonMinimalLengthEncoding_Rejected verifies that non-minimal extended
// payload length encodings are rejected with StatusProtocolError (1002) (RFC 6455 §5.2).
func TestNemesis_NonMinimalLengthEncoding_Rejected(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		header []byte
	}{
		// 16-bit extended length used for length 0 (must use 7-bit)
		{"16Bit_Length0", []byte{0x81, 126, 0x00, 0x00}},
		// 16-bit extended length used for length 125 (must use 7-bit)
		{"16Bit_Length125", []byte{0x81, 126, 0x00, 0x7D}},
		// 64-bit extended length used for length 0 (must use 7-bit)
		{"64Bit_Length0", []byte{0x81, 127, 0, 0, 0, 0, 0, 0, 0, 0}},
		// 64-bit extended length used for length 65535 (must use 16-bit)
		{"64Bit_Length65535", []byte{0x81, 127, 0, 0, 0, 0, 0, 0, 0xFF, 0xFF}},
		// 64-bit extended length with MSB set (most significant bit must be 0)
		{"64Bit_MSBSet", []byte{0x81, 127, 0x80, 0, 0, 0, 0, 0, 0, 0}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := ws.ParseHeader(tc.header)
			require.Error(t, err, "ParseHeader must reject non-minimal length encoding")
			assert.True(
				t,
				errors.Is(err, ws.ErrInvalidLength) || errors.Is(err, ws.ErrPayloadTooLarge) ||
					errors.Is(err, ws.ErrProtocolViolation),
				"expected error",
			)
		})
	}
}

// ============================================================================
// 8. Memory Exhaustion / Allocation Bomb Denial-of-Service Defense
// ============================================================================

// TestNemesis_MemoryExhaustion_DoS_Defense verifies that an adversary sending a
// frame header claiming a 2GB payload does NOT cause the server to allocate 2GB
// of heap memory before verifying readLimit (RFC 6455 §10.4).
func TestNemesis_MemoryExhaustion_DoS_Defense(t *testing.T) {
	t.Parallel()

	clientConnRaw, serverConnRaw := tcpPair(t)
	serverConn := ws.NewServerConn(serverConnRaw)
	serverConn.SetReadLimit(64 * 1024) // 64KB read limit
	defer serverConn.Close()
	defer clientConnRaw.Close()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		rawHeader := make([]byte, 14)
		rawHeader[0] = 0x82       // Binary, FIN=1
		rawHeader[1] = 0x80 | 127 // Masked=1, len=127 (64-bit)
		binary.BigEndian.PutUint64(rawHeader[2:10], 2_000_000_000)
		_, _ = clientConnRaw.Write(rawHeader)

		buf := make([]byte, 128)
		_ = clientConnRaw.SetReadDeadline(time.Now().Add(2 * time.Second))
		_, _ = clientConnRaw.Read(buf)
	}()

	_, _, err := serverConn.ReadMessage()
	wg.Wait()
	require.Error(t, err, "server must reject payload exceeding read limit immediately")
	assert.True(t, errors.Is(err, ws.ErrMessageTooBig) || errors.Is(err, ws.ErrPayloadTooLarge),
		fmt.Sprintf("expected ErrMessageTooBig, got: %v", err))
	assert.Equal(t, ws.StateClosed, serverConn.State())
}

// ============================================================================
// 9. Slowloris Partial Frame Streaming & Reassembly
// ============================================================================

// TestNemesis_Slowloris_ByteByByte_Streaming verifies that frames delivered
// 1 byte at a time across network boundaries are completely and correctly reassembled.
func TestNemesis_Slowloris_ByteByByte_Streaming(t *testing.T) {
	t.Parallel()

	clientConnRaw, serverConnRaw := tcpPair(t)
	clientConn := ws.NewClientConn(clientConnRaw)
	defer clientConn.Close()
	defer serverConnRaw.Close()

	expectedMessage := "The quick brown fox jumps over the lazy dog."

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		var frameBuf []byte
		h := ws.Header{
			Fin:        true,
			Opcode:     ws.OpcodeText,
			PayloadLen: int64(len(expectedMessage)),
		}
		frameBuf = ws.AppendFrame(frameBuf, h, []byte(expectedMessage))

		for _, b := range frameBuf {
			_, err := serverConnRaw.Write([]byte{b})
			if err != nil {
				break
			}
			time.Sleep(50 * time.Microsecond)
		}
	}()

	op, payload, err := clientConn.ReadMessage()
	wg.Wait()
	require.NoError(t, err)
	assert.Equal(t, ws.OpcodeText, op)
	assert.Equal(t, expectedMessage, string(payload))
}

// ============================================================================
// 10. Chromium Regressions: Interleaved Control Frames During Fragmentation
// ============================================================================

// TestNemesis_Chromium_InterleavedPingPong_FragmentedText verifies that control
// frames (Ping, Pong) injected in the middle of fragmented messages are processed
// without corrupting message reassembly (crbug.com/393000981, RFC 6455 §5.5).
func TestNemesis_Chromium_InterleavedPingPong_FragmentedText(t *testing.T) {
	t.Parallel()

	clientConnRaw, serverConnRaw := tcpPair(t)
	clientConn := ws.NewClientConn(clientConnRaw)
	defer clientConn.Close()
	defer serverConnRaw.Close()

	var pingPayloadReceived []byte
	var pingMu sync.Mutex
	clientConn.SetPingHandler(func(appData []byte) error {
		pingMu.Lock()
		pingPayloadReceived = append([]byte(nil), appData...)
		pingMu.Unlock()
		return nil
	})

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		// 1. Text fragment 1: FIN=0, payload="part1_"
		f1 := []byte{0x01, 0x06, 'p', 'a', 'r', 't', '1', '_'}
		_, _ = serverConnRaw.Write(f1)

		// 2. Interleaved Ping control frame: FIN=1, Opcode=9, payload="heartbeat"
		ping := []byte{0x89, 0x09, 'h', 'e', 'a', 'r', 't', 'b', 'e', 'a', 't'}
		_, _ = serverConnRaw.Write(ping)

		// 3. Text fragment 2: FIN=1, Continuation (Opcode=0), payload="part2"
		f2 := []byte{0x80, 0x05, 'p', 'a', 'r', 't', '2'}
		_, _ = serverConnRaw.Write(f2)
	}()

	op, msg, err := clientConn.ReadMessage()
	wg.Wait()
	require.NoError(t, err)
	assert.Equal(t, ws.OpcodeText, op)
	assert.Equal(
		t,
		"part1_part2",
		string(msg),
		"fragmented message must be correctly assembled despite interleaved Ping",
	)

	pingMu.Lock()
	assert.Equal(t, "heartbeat", string(pingPayloadReceived), "interleaved ping must be handled")
	pingMu.Unlock()
}

// ============================================================================
// 11. Zero-Allocation Mandate on Hot Paths
// ============================================================================

// TestNemesis_ZeroAllocation_HotPaths verifies that RFC 6455 parsing, masking,
// header packing, and handshake accept calculation allocate 0 heap bytes.
func TestNemesis_ZeroAllocation_HotPaths(t *testing.T) {
	rawFrameHeader := []byte{0x81, 0x85, 0xDE, 0xAD, 0xBE, 0xEF}
	allocs := testing.AllocsPerRun(100, func() {
		h, _, err := ws.ParseHeader(rawFrameHeader)
		if err != nil || !h.Fin {
			t.Fail()
		}
	})
	assert.Equal(t, float64(0), allocs, "ParseHeader must achieve 0 heap allocations")

	var writeBuf [14]byte
	h := ws.Header{
		Fin:        true,
		Opcode:     ws.OpcodeBinary,
		Masked:     true,
		MaskKey:    [4]byte{1, 2, 3, 4},
		PayloadLen: 100,
	}
	allocs = testing.AllocsPerRun(100, func() {
		n, err := ws.WriteHeader(writeBuf[:], h)
		if err != nil || n == 0 {
			t.Fail()
		}
	})
	assert.Equal(t, float64(0), allocs, "WriteHeader must achieve 0 heap allocations")

	payload := make([]byte, 1024)
	key := [4]byte{0xAA, 0xBB, 0xCC, 0xDD}
	allocs = testing.AllocsPerRun(100, func() {
		ws.Mask(payload, key)
	})
	assert.Equal(t, float64(0), allocs, "SWAR Vector Mask must achieve 0 heap allocations")

	wsKey := "dGhlIHNhbXBsZSBub25jZQ=="
	var acceptBuf [32]byte
	allocs = testing.AllocsPerRun(100, func() {
		dst := ws.AppendAccept(acceptBuf[:0], wsKey)
		if len(dst) != 28 {
			t.Fail()
		}
	})
	assert.Equal(t, float64(0), allocs, "AppendAccept must achieve 0 heap allocations")
}

// ============================================================================
// 12. High-Concurrency Race & Stress Fuzzing
// ============================================================================

// TestNemesis_Concurrent_ReadWrite_Stress tests concurrent bidirectional messaging,
// interleaved ping/pong heartbeats, and graceful closing under concurrent goroutines.
func TestNemesis_Concurrent_ReadWrite_Stress(t *testing.T) {
	clientConnRaw, serverConnRaw := tcpPair(t)
	clientConn := ws.NewClientConn(clientConnRaw)
	serverConn := ws.NewServerConn(serverConnRaw)

	defer clientConn.Close()
	defer serverConn.Close()

	const numMessages = 50
	var wg sync.WaitGroup
	wg.Add(4)

	// Goroutine 1: Client writes messages and pings
	go func() {
		defer wg.Done()
		for i := 0; i < numMessages; i++ {
			_ = clientConn.WriteMessage(ws.OpcodeBinary, []byte("client-data"))
			if i%10 == 0 {
				_ = clientConn.WritePing([]byte("ping"))
			}
		}
	}()

	// Goroutine 2: Server writes messages and pongs
	go func() {
		defer wg.Done()
		for i := 0; i < numMessages; i++ {
			_ = serverConn.WriteMessage(ws.OpcodeText, []byte("server-data"))
			if i%15 == 0 {
				_ = serverConn.WritePong([]byte("pong"))
			}
		}
	}()

	// Goroutine 3: Client reads messages
	go func() {
		defer wg.Done()
		received := 0
		for received < numMessages {
			_, _, err := clientConn.ReadMessage()
			if err != nil {
				break
			}
			received++
		}
	}()

	// Goroutine 4: Server reads messages
	go func() {
		defer wg.Done()
		received := 0
		for received < numMessages {
			_, _, err := serverConn.ReadMessage()
			if err != nil {
				break
			}
			received++
		}
	}()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Completed successfully
	case <-time.After(5 * time.Second):
		t.Fatal("concurrent stress test timed out (potential deadlock)")
	}
}
