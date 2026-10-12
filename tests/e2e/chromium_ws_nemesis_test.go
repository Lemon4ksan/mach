// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package e2e_test

import (
	"bufio"
	"encoding/binary"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/lemon4ksan/foundation/testing/assert"
	"github.com/lemon4ksan/foundation/testing/require"
	"github.com/lemon4ksan/mach/proto/ws"
)

// setupWSServer starts a loopback TCP listener and accepts a connection,
// completing the RFC 6455 HTTP/1.1 handshake before returning the connected ws.Conn.
func setupWSServer(t *testing.T) (net.Listener, string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	return ln, ln.Addr().String()
}

// performHandshake performs an RFC 6455 opening handshake over rawConn.
func performServerHandshake(rawConn net.Conn) (*ws.Conn, error) {
	reader := bufio.NewReader(rawConn)
	// Read request line and headers
	reqLine, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}
	_ = reqLine

	var wsKey string
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		if line == "\r\n" || line == "\n" {
			break
		}
		if len(line) > 19 && line[:19] == "Sec-WebSocket-Key: " {
			wsKey = line[19 : len(line)-2]
		}
	}

	if wsKey == "" {
		return nil, errors.New("missing Sec-WebSocket-Key")
	}

	// Send 101 Switching Protocols
	acceptVal := ws.ComputeAccept(wsKey)
	resp := "HTTP/1.1 101 Switching Protocols\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Accept: " + acceptVal + "\r\n\r\n"
	if _, err := rawConn.Write([]byte(resp)); err != nil {
		return nil, err
	}

	return ws.NewConnWithReader(rawConn, reader, true), nil
}

// ============================================================================
// Chromium WebSocket Nemesis: Wire Framing & Protocol Error Attacks
// ============================================================================

// TestChromiumWSNemesis_UnmaskedFrameInjection verifies that an unmasked frame
// injected by a malicious client is rejected with 1002 Protocol Error and closed.
func TestChromiumWSNemesis_UnmaskedFrameInjection(t *testing.T) {
	t.Parallel()

	ln, addr := setupWSServer(t)
	defer ln.Close()

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		serverWS, err := performServerHandshake(conn)
		if err != nil {
			_ = conn.Close()
			return
		}
		defer serverWS.Close()

		_, _, _ = serverWS.ReadMessage()
	}()

	rawClient, err := net.Dial("tcp", addr)
	require.NoError(t, err)
	defer rawClient.Close()

	// 1. Send opening handshake
	req := "GET /ws HTTP/1.1\r\n" +
		"Host: " + addr + "\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\n" +
		"Sec-WebSocket-Version: 13\r\n\r\n"
	_, err = rawClient.Write([]byte(req))
	require.NoError(t, err)

	// Read 101 response
	reader := bufio.NewReader(rawClient)
	statusLine, err := reader.ReadString('\n')
	require.NoError(t, err)
	assert.Contains(t, statusLine, "101")

	// Read remaining headers until empty line
	for {
		line, err := reader.ReadString('\n')
		require.NoError(t, err)
		if line == "\r\n" || line == "\n" {
			break
		}
	}

	// 2. Inject unmasked text frame (RFC 6455 §5.1 violation)
	unmaskedFrame := []byte{0x81, 0x05, 'H', 'e', 'l', 'l', 'o'}
	_, err = rawClient.Write(unmaskedFrame)
	require.NoError(t, err)

	// 3. Server MUST reply with Close frame code 1002 (Protocol Error)
	var respBuf [128]byte
	_ = rawClient.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err := reader.Read(respBuf[:])
	require.NoError(t, err)
	require.True(t, n >= 4, "server must send Close frame")

	h, hlen, err := ws.ParseHeader(respBuf[:n])
	require.NoError(t, err)
	assert.Equal(t, ws.OpcodeClose, h.Opcode)

	closeCode, _, err := ws.ParseClosePayload(respBuf[hlen : hlen+int(h.PayloadLen)])
	require.NoError(t, err)
	assert.Equal(t, ws.StatusProtocolError, closeCode, "server must send 1002 Protocol Error")

	wg.Wait()
}

// TestChromiumWSNemesis_OversizedControlFrames verifies that Ping, Pong, and Close
// frames with payload length > 125 bytes are strictly rejected with 1002 Protocol Error.
func TestChromiumWSNemesis_OversizedControlFrames(t *testing.T) {
	t.Parallel()

	testOpcodes := []ws.Opcode{ws.OpcodePing, ws.OpcodePong, ws.OpcodeClose}

	for _, op := range testOpcodes {
		t.Run(op.String(), func(t *testing.T) {
			ln, addr := setupWSServer(t)
			defer ln.Close()

			var wg sync.WaitGroup
			wg.Add(1)
			go func() {
				defer wg.Done()
				conn, err := ln.Accept()
				if err != nil {
					return
				}
				serverWS, err := performServerHandshake(conn)
				if err != nil {
					_ = conn.Close()
					return
				}
				defer serverWS.Close()
				_, _, _ = serverWS.ReadMessage()
			}()

			rawClient, err := net.Dial("tcp", addr)
			require.NoError(t, err)
			defer rawClient.Close()

			req := "GET /ws HTTP/1.1\r\n" +
				"Host: " + addr + "\r\n" +
				"Upgrade: websocket\r\n" +
				"Connection: Upgrade\r\n" +
				"Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\n" +
				"Sec-WebSocket-Version: 13\r\n\r\n"
			_, _ = rawClient.Write([]byte(req))

			reader := bufio.NewReader(rawClient)
			for {
				line, _ := reader.ReadString('\n')
				if line == "\r\n" || line == "\n" {
					break
				}
			}

			// Construct 126-byte masked control frame
			payload126 := make([]byte, 126)
			maskKey := [4]byte{0xDE, 0xAD, 0xBE, 0xEF}
			ws.Mask(payload126, maskKey)

			var frame []byte
			frame = append(frame, 0x80|byte(op), 0x80|126)
			var lenBytes [2]byte
			binary.BigEndian.PutUint16(lenBytes[:], 126)
			frame = append(frame, lenBytes[:]...)
			frame = append(frame, maskKey[:]...)
			frame = append(frame, payload126...)

			_, _ = rawClient.Write(frame)

			var resp [128]byte
			_ = rawClient.SetReadDeadline(time.Now().Add(2 * time.Second))
			n, err := reader.Read(resp[:])
			require.NoError(t, err)

			h, hlen, err := ws.ParseHeader(resp[:n])
			require.NoError(t, err)
			assert.Equal(t, ws.OpcodeClose, h.Opcode)

			closeCode, _, _ := ws.ParseClosePayload(resp[hlen : hlen+int(h.PayloadLen)])
			assert.Equal(t, ws.StatusProtocolError, closeCode)

			wg.Wait()
		})
	}
}

// TestChromiumWSNemesis_FragmentedControlFrameInjection verifies that FIN=0
// on a control frame is rejected with 1002 Protocol Error.
func TestChromiumWSNemesis_FragmentedControlFrameInjection(t *testing.T) {
	t.Parallel()

	ln, addr := setupWSServer(t)
	defer ln.Close()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		serverWS, err := performServerHandshake(conn)
		if err != nil {
			_ = conn.Close()
			return
		}
		defer serverWS.Close()
		_, _, _ = serverWS.ReadMessage()
	}()

	rawClient, err := net.Dial("tcp", addr)
	require.NoError(t, err)
	defer rawClient.Close()

	req := "GET /ws HTTP/1.1\r\n" +
		"Host: " + addr + "\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\n" +
		"Sec-WebSocket-Version: 13\r\n\r\n"
	_, _ = rawClient.Write([]byte(req))

	reader := bufio.NewReader(rawClient)
	for {
		line, _ := reader.ReadString('\n')
		if line == "\r\n" || line == "\n" {
			break
		}
	}

	// Send Ping with FIN=0 (0x09) and mask key
	maskKey := [4]byte{1, 2, 3, 4}
	fragPing := []byte{0x09, 0x84, maskKey[0], maskKey[1], maskKey[2], maskKey[3], 'p', 'i', 'n', 'g'}
	_, _ = rawClient.Write(fragPing)

	var resp [128]byte
	_ = rawClient.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err := reader.Read(resp[:])
	require.NoError(t, err)

	h, hlen, _ := ws.ParseHeader(resp[:n])
	assert.Equal(t, ws.OpcodeClose, h.Opcode)
	code, _, _ := ws.ParseClosePayload(resp[hlen : hlen+int(h.PayloadLen)])
	assert.Equal(t, ws.StatusProtocolError, code)

	wg.Wait()
}

// TestChromiumWSNemesis_ReservedBitsSmuggling verifies that setting RSV1, RSV2, or RSV3
// bits without negotiated extension causes 1002 Protocol Error.
func TestChromiumWSNemesis_ReservedBitsSmuggling(t *testing.T) {
	t.Parallel()

	ln, addr := setupWSServer(t)
	defer ln.Close()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		serverWS, err := performServerHandshake(conn)
		if err != nil {
			_ = conn.Close()
			return
		}
		defer serverWS.Close()
		_, _, _ = serverWS.ReadMessage()
	}()

	rawClient, err := net.Dial("tcp", addr)
	require.NoError(t, err)
	defer rawClient.Close()

	req := "GET /ws HTTP/1.1\r\n" +
		"Host: " + addr + "\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\n" +
		"Sec-WebSocket-Version: 13\r\n\r\n"
	_, _ = rawClient.Write([]byte(req))

	reader := bufio.NewReader(rawClient)
	for {
		line, _ := reader.ReadString('\n')
		if line == "\r\n" || line == "\n" {
			break
		}
	}

	// RSV2 set (0xA1 = 1010 0001)
	maskKey := [4]byte{5, 6, 7, 8}
	payload := []byte("evil")
	ws.Mask(payload, maskKey)
	rsvFrame := []byte{0xA1, 0x84, maskKey[0], maskKey[1], maskKey[2], maskKey[3]}
	rsvFrame = append(rsvFrame, payload...)
	_, _ = rawClient.Write(rsvFrame)

	var resp [128]byte
	_ = rawClient.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err := reader.Read(resp[:])
	require.NoError(t, err)

	h, hlen, _ := ws.ParseHeader(resp[:n])
	assert.Equal(t, ws.OpcodeClose, h.Opcode)
	code, _, _ := ws.ParseClosePayload(resp[hlen : hlen+int(h.PayloadLen)])
	assert.Equal(t, ws.StatusProtocolError, code)

	wg.Wait()
}

// TestChromiumWSNemesis_UTF8FuzzExploits verifies that invalid UTF-8 sequences
// in Text frames trigger StatusInvalidFramePayloadData (1007).
func TestChromiumWSNemesis_UTF8FuzzExploits(t *testing.T) {
	t.Parallel()

	ln, addr := setupWSServer(t)
	defer ln.Close()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		serverWS, err := performServerHandshake(conn)
		if err != nil {
			_ = conn.Close()
			return
		}
		defer serverWS.Close()
		_, _, _ = serverWS.ReadMessage()
	}()

	rawClient, err := net.Dial("tcp", addr)
	require.NoError(t, err)
	defer rawClient.Close()

	req := "GET /ws HTTP/1.1\r\n" +
		"Host: " + addr + "\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\n" +
		"Sec-WebSocket-Version: 13\r\n\r\n"
	_, _ = rawClient.Write([]byte(req))

	reader := bufio.NewReader(rawClient)
	for {
		line, _ := reader.ReadString('\n')
		if line == "\r\n" || line == "\n" {
			break
		}
	}

	// Text frame with invalid UTF-8: 0xFF, 0xFE
	badUTF8 := []byte{0xFF, 0xFE}
	maskKey := [4]byte{1, 1, 1, 1}
	ws.Mask(badUTF8, maskKey)
	badFrame := []byte{0x81, 0x82, maskKey[0], maskKey[1], maskKey[2], maskKey[3], badUTF8[0], badUTF8[1]}
	_, _ = rawClient.Write(badFrame)

	var resp [128]byte
	_ = rawClient.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err := reader.Read(resp[:])
	require.NoError(t, err)

	h, hlen, _ := ws.ParseHeader(resp[:n])
	assert.Equal(t, ws.OpcodeClose, h.Opcode)
	code, _, _ := ws.ParseClosePayload(resp[hlen : hlen+int(h.PayloadLen)])
	assert.Equal(t, ws.StatusInvalidFramePayloadData, code, "server must reject invalid UTF-8 with 1007")

	wg.Wait()
}

// TestChromiumWSNemesis_ForbiddenCloseCodes verifies that forbidden close codes
// on wire (e.g. 1005, 1006, 1015, <1000) are rejected with 1002 Protocol Error.
func TestChromiumWSNemesis_ForbiddenCloseCodes(t *testing.T) {
	t.Parallel()

	ln, addr := setupWSServer(t)
	defer ln.Close()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		serverWS, err := performServerHandshake(conn)
		if err != nil {
			_ = conn.Close()
			return
		}
		defer serverWS.Close()
		_, _, _ = serverWS.ReadMessage()
	}()

	rawClient, err := net.Dial("tcp", addr)
	require.NoError(t, err)
	defer rawClient.Close()

	req := "GET /ws HTTP/1.1\r\n" +
		"Host: " + addr + "\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\n" +
		"Sec-WebSocket-Version: 13\r\n\r\n"
	_, _ = rawClient.Write([]byte(req))

	reader := bufio.NewReader(rawClient)
	for {
		line, _ := reader.ReadString('\n')
		if line == "\r\n" || line == "\n" {
			break
		}
	}

	// Close frame sending 1005 (StatusNoStatusReceived) on wire (RFC 6455 §7.4.2 violation)
	payload := []byte{0x03, 0xED} // 1005
	maskKey := [4]byte{9, 9, 9, 9}
	ws.Mask(payload, maskKey)
	badClose := []byte{0x88, 0x82, maskKey[0], maskKey[1], maskKey[2], maskKey[3], payload[0], payload[1]}
	_, _ = rawClient.Write(badClose)

	var resp [128]byte
	_ = rawClient.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err := reader.Read(resp[:])
	require.NoError(t, err)

	h, hlen, _ := ws.ParseHeader(resp[:n])
	assert.Equal(t, ws.OpcodeClose, h.Opcode)
	code, _, _ := ws.ParseClosePayload(resp[hlen : hlen+int(h.PayloadLen)])
	assert.Equal(t, ws.StatusProtocolError, code, "code 1005 on wire must be rejected with 1002")

	wg.Wait()
}

// TestChromiumWSNemesis_SlowlorisFrameStarvation verifies that frames streamed
// 1 byte at a time through TCP buffers are correctly assembled without hang.
func TestChromiumWSNemesis_SlowlorisFrameStarvation(t *testing.T) {
	t.Parallel()

	ln, addr := setupWSServer(t)
	defer ln.Close()

	var wg sync.WaitGroup
	wg.Add(1)

	expected := "Slowloris payload streamed byte-by-byte"

	go func() {
		defer wg.Done()
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		serverWS, err := performServerHandshake(conn)
		if err != nil {
			_ = conn.Close()
			return
		}
		defer serverWS.Close()

		op, msg, err := serverWS.ReadMessage()
		if err == nil {
			assert.Equal(t, ws.OpcodeText, op)
			assert.Equal(t, expected, string(msg))
		}
	}()

	rawClient, err := net.Dial("tcp", addr)
	require.NoError(t, err)
	defer rawClient.Close()

	req := "GET /ws HTTP/1.1\r\n" +
		"Host: " + addr + "\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\n" +
		"Sec-WebSocket-Version: 13\r\n\r\n"
	_, _ = rawClient.Write([]byte(req))

	reader := bufio.NewReader(rawClient)
	for {
		line, _ := reader.ReadString('\n')
		if line == "\r\n" || line == "\n" {
			break
		}
	}

	// Construct masked Text frame
	h := ws.Header{
		Fin:        true,
		Opcode:     ws.OpcodeText,
		Masked:     true,
		MaskKey:    [4]byte{0x11, 0x22, 0x33, 0x44},
		PayloadLen: int64(len(expected)),
	}
	var wireFrame []byte
	wireFrame = ws.AppendFrame(wireFrame, h, []byte(expected))

	// Stream 1 byte at a time
	for _, b := range wireFrame {
		_, err := rawClient.Write([]byte{b})
		if err != nil {
			break
		}
		time.Sleep(20 * time.Microsecond)
	}

	wg.Wait()
}
