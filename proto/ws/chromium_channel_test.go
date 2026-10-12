package ws

import (
	"bytes"
	"encoding/binary"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

// Chromium test vectors ported from:
// - net/websockets/websocket_channel_test.cc
// - net/websockets/websocket_basic_stream_test.cc

// Ported from: TEST_F(WebSocketChannelEventInterfaceTest, SingleFrameMessage)
func TestChromium_SingleFrameMessage(t *testing.T) {
	client, server := tcpPair(t)
	defer client.Close()
	defer server.Close()

	go func() {
		_ = client.WriteMessage(OpText, []byte("Hello"))
	}()

	op, msg, err := server.ReadMessage()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if op != OpText || string(msg) != "Hello" {
		t.Fatalf("unexpected message: op=%v, msg=%s", op, string(msg))
	}
}

// Ported from: TEST_F(WebSocketChannelEventInterfaceTest, EmptyMessage)
func TestChromium_EmptyMessage(t *testing.T) {
	client, server := tcpPair(t)
	defer client.Close()
	defer server.Close()

	go func() {
		_ = client.WriteMessage(OpText, []byte(""))
	}()

	op, msg, err := server.ReadMessage()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if op != OpText || len(msg) != 0 {
		t.Fatalf("expected empty text message, got op=%v, len=%d", op, len(msg))
	}
}

// Ported from: TEST_F(WebSocketChannelEventInterfaceTest, FragmentedMessage)
func TestChromium_FragmentedMessage(t *testing.T) {
	client, server := tcpPair(t)
	defer client.Close()
	defer server.Close()

	go func() {
		_ = client.writeDataFrame(false, false, OpText, []byte("SPLIT "))
		_ = client.writeDataFrame(false, false, OpContinuation, []byte("FRAGMENTED "))
		_ = client.writeDataFrame(true, false, OpContinuation, []byte("MESSAGE"))
	}()

	op, msg, err := server.ReadMessage()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if op != OpText || string(msg) != "SPLIT FRAGMENTED MESSAGE" {
		t.Fatalf("unexpected message: op=%v, msg=%s", op, string(msg))
	}
}

// Ported from: TEST_F(WebSocketChannelEventInterfaceTest, ControlFrameInDataMessage)
func TestChromium_ControlFrameInDataMessage(t *testing.T) {
	client, server := tcpPair(t)
	defer client.Close()
	defer server.Close()

	pongReceived := make(chan struct{}, 1)
	server.SetPongHandler(func(appData []byte) error {
		pongReceived <- struct{}{}
		return nil
	})

	serverResult := make(chan string, 1)
	go func() {
		op, msg, err := server.ReadMessage()
		if err != nil {
			t.Errorf("server read error: %v", err)
			return
		}
		if op != OpText {
			t.Errorf("expected OpText, got %v", op)
			return
		}
		serverResult <- string(msg)
	}()

	// 1. Text fragment (FIN=false): "SPLIT "
	if err := client.writeDataFrame(false, false, OpText, []byte("SPLIT ")); err != nil {
		t.Fatalf("fragment 1 error: %v", err)
	}

	// 2. Control frame Pong in the middle (FIN=true)
	if err := client.WritePong([]byte("")); err != nil {
		t.Fatalf("pong write error: %v", err)
	}

	// 3. Continuation fragment (FIN=true): "MESSAGE"
	if err := client.writeDataFrame(true, false, OpContinuation, []byte("MESSAGE")); err != nil {
		t.Fatalf("fragment 2 error: %v", err)
	}

	select {
	case <-pongReceived:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for interleaved pong")
	}

	select {
	case msg := <-serverResult:
		if msg != "SPLIT MESSAGE" {
			t.Fatalf("expected 'SPLIT MESSAGE', got %q", msg)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for reassembled message")
	}
}

// Ported from: TEST_F(WebSocketChannelEventInterfaceTest, PongWithNullData)
func TestChromium_PongWithNullData(t *testing.T) {
	client, server := tcpPair(t)
	defer client.Close()
	defer server.Close()

	nullPongReceived := make(chan bool, 1)
	server.SetPongHandler(func(appData []byte) error {
		nullPongReceived <- (len(appData) == 0)
		return nil
	})

	go func() {
		// Server reads message to trigger pong handler
		_, _, _ = server.ReadMessage()
	}()

	if err := client.WritePong(nil); err != nil {
		t.Fatalf("failed to write null pong: %v", err)
	}

	// Send text message so server ReadMessage completes
	_ = client.WriteMessage(OpText, []byte("done"))

	select {
	case wasNull := <-nullPongReceived:
		if !wasNull {
			t.Fatal("expected empty payload in pong")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for null pong")
	}
}

// Ported from: TEST_F(WebSocketChannelStreamTest, PingRepliedWithPong)
func TestChromium_PingRepliedWithPong(t *testing.T) {
	client, server := tcpPair(t)
	defer client.Close()
	defer server.Close()

	pongReceived := make(chan []byte, 1)
	client.SetPongHandler(func(appData []byte) error {
		pongReceived <- appData
		return nil
	})

	go func() {
		_, _, _ = server.ReadMessage()
	}()
	go func() {
		_, _, _ = client.ReadMessage()
	}()

	pingData := []byte("Application data")
	if err := client.WritePing(pingData); err != nil {
		t.Fatalf("failed to write ping: %v", err)
	}

	select {
	case pongData := <-pongReceived:
		if !bytes.Equal(pongData, pingData) {
			t.Fatalf("pong data mismatch: got %q, want %q", pongData, pingData)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for pong")
	}
}

// Ported from: TEST_F(WebSocketChannelStreamTest, NullPingRepliedWithNullPong)
func TestChromium_NullPingRepliedWithNullPong(t *testing.T) {
	client, server := tcpPair(t)
	defer client.Close()
	defer server.Close()

	pongReceived := make(chan []byte, 1)
	client.SetPongHandler(func(appData []byte) error {
		pongReceived <- appData
		return nil
	})

	go func() {
		_, _, _ = server.ReadMessage()
	}()
	go func() {
		_, _, _ = client.ReadMessage()
	}()

	if err := client.WritePing(nil); err != nil {
		t.Fatalf("failed to write null ping: %v", err)
	}

	select {
	case pongData := <-pongReceived:
		if len(pongData) != 0 {
			t.Fatalf("expected 0-length pong data, got len=%d", len(pongData))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for null pong")
	}
}

// Ported from: TEST_F(WebSocketChannelStreamTest, PongInTheMiddleOfDataMessage)
func TestChromium_PongInTheMiddleOfDataMessage(t *testing.T) {
	client, server := tcpPair(t)
	defer client.Close()
	defer server.Close()

	pongReceived := make(chan []byte, 1)
	server.SetPongHandler(func(appData []byte) error {
		pongReceived <- appData
		return nil
	})

	serverDone := make(chan string, 1)
	go func() {
		op, msg, err := server.ReadMessage()
		if err != nil {
			t.Errorf("read error: %v", err)
			return
		}
		if op != OpText {
			t.Errorf("expected OpText, got %v", op)
			return
		}
		serverDone <- string(msg)
	}()

	// Fragment 1: "Hello "
	if err := client.writeDataFrame(false, false, OpText, []byte("Hello ")); err != nil {
		t.Fatalf("write fragment 1 error: %v", err)
	}

	// Interleaved Pong: "Application data"
	if err := client.WritePong([]byte("Application data")); err != nil {
		t.Fatalf("write pong error: %v", err)
	}

	// Fragment 2: "World"
	if err := client.writeDataFrame(true, false, OpContinuation, []byte("World")); err != nil {
		t.Fatalf("write fragment 2 error: %v", err)
	}

	select {
	case pongData := <-pongReceived:
		if string(pongData) != "Application data" {
			t.Fatalf("pong payload mismatch: %s", string(pongData))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for pong")
	}

	select {
	case msg := <-serverDone:
		if msg != "Hello World" {
			t.Fatalf("reassembled message mismatch: got %q, want 'Hello World'", msg)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for complete message")
	}
}

// Ported from: TEST_F(WebSocketChannelStreamTest, CloseIsEchoedBack)
func TestChromium_CloseIsEchoedBack(t *testing.T) {
	client, server := tcpPair(t)
	defer client.Close()
	defer server.Close()

	clientErrCh := make(chan error, 1)
	go func() {
		_, _, err := client.ReadMessage()
		clientErrCh <- err
	}()

	serverErrCh := make(chan error, 1)
	go func() {
		_, _, err := server.ReadMessage()
		serverErrCh <- err
	}()

	// Server sends Close
	if err := server.WriteClose(StatusNormalClosure, "Close"); err != nil {
		t.Fatalf("server write close error: %v", err)
	}

	select {
	case err := <-clientErrCh:
		var ce *CloseError
		if !errors.As(err, &ce) {
			t.Fatalf("client expected CloseError, got %v", err)
		}
		if ce.Code != StatusNormalClosure || ce.Reason != "Close" {
			t.Fatalf("client close mismatch: %v, %s", ce.Code, ce.Reason)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("client timed out waiting for close")
	}

	select {
	case err := <-serverErrCh:
		var ce *CloseError
		if !errors.As(err, &ce) {
			t.Fatalf("server expected CloseError, got %v", err)
		}
		if ce.Code != StatusNormalClosure {
			t.Fatalf("server close code mismatch: %v", ce.Code)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server timed out waiting for echoed close")
	}
}

// Ported from: TEST_F(WebSocketChannelStreamTest, CloseOnlySentOnce)
func TestChromium_CloseOnlySentOnce(t *testing.T) {
	client, server := tcpPair(t)
	defer client.Close()
	defer server.Close()

	if err := client.WriteClose(StatusNormalClosure, "Close"); err != nil {
		t.Fatalf("client first close failed: %v", err)
	}

	// Calling WriteClose a second time must return ErrClosed
	if err := client.WriteClose(StatusNormalClosure, "Second Close"); !errors.Is(err, ErrClosed) {
		t.Fatalf("expected ErrClosed on second WriteClose, got %v", err)
	}
}

// Ported from: TEST_F(WebSocketChannelStreamTest, InvalidCloseStatusCodeNotSent)
func TestChromium_InvalidCloseStatusCodeNotSent(t *testing.T) {
	client, _ := tcpPair(t)
	defer client.Close()

	// Codes < 1000 or reserved codes 1004, 1005, 1006, 1014, 1015, >= 5000 must be rejected
	invalidCodes := []CloseCode{0, 999, 1004, 1005, 1006, 1014, 1015, 2999, 5000, 6000}
	for _, code := range invalidCodes {
		err := client.WriteClose(code, "test")
		if !errors.Is(err, ErrInvalidCloseCode) {
			t.Fatalf("code %d: expected ErrInvalidCloseCode, got %v", code, err)
		}
	}
}

// Ported from: TEST_F(WebSocketChannelStreamTest, LongCloseReasonNotSent)
func TestChromium_LongCloseReasonNotSent(t *testing.T) {
	client, _ := tcpPair(t)
	defer client.Close()

	// Reason > 123 bytes must be rejected
	longReason := strings.Repeat("A", 124)
	err := client.WriteClose(StatusNormalClosure, longReason)
	if !errors.Is(err, ErrCloseReasonTooLong) {
		t.Fatalf("expected ErrCloseReasonTooLong, got %v", err)
	}

	// Reason <= 123 bytes should pass validation
	validReason := strings.Repeat("A", 123)
	payload, err := AppendClosePayload(StatusNormalClosure, validReason)
	if err != nil || len(payload) != 125 {
		t.Fatalf("expected valid 125-byte close payload, got len=%d, err=%v", len(payload), err)
	}
}

// Ported from: TEST_F(WebSocketChannelStreamTest, Code1005IsNotEchoed)
func TestChromium_Code1005IsNotEchoed(t *testing.T) {
	client, server := tcpPair(t)
	defer client.Close()
	defer server.Close()

	serverErrCh := make(chan error, 1)
	go func() {
		_, _, err := server.ReadMessage()
		serverErrCh <- err
	}()

	clientErrCh := make(chan error, 1)
	go func() {
		_, _, err := client.ReadMessage()
		clientErrCh <- err
	}()

	// Client sends empty close frame (0 payload, representing 1005 internally)
	_ = client.writeControlFrame(OpClose, nil)

	select {
	case err := <-serverErrCh:
		var ce *CloseError
		if !errors.As(err, &ce) {
			t.Fatalf("server expected CloseError, got %v", err)
		}
		// Server internally records code 1005
		if ce.Code != StatusNoStatusRcvd {
			t.Fatalf("expected StatusNoStatusRcvd (1005), got %v", ce.Code)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server timed out waiting for empty close")
	}

	select {
	case err := <-clientErrCh:
		var ce *CloseError
		if !errors.As(err, &ce) {
			t.Fatalf("client expected CloseError, got %v", err)
		}
		// Echoed back frame also had 0 payload, so client also sees StatusNoStatusRcvd
		if ce.Code != StatusNoStatusRcvd {
			t.Fatalf("client expected StatusNoStatusRcvd (1005), got %v", ce.Code)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("client timed out waiting for echoed empty close")
	}
}

// Ported from: TEST_F(WebSocketChannelEventInterfaceTest, OneByteClosePayloadMessage)
func TestChromium_OneByteClosePayloadMessage(t *testing.T) {
	client, server := tcpPair(t)
	defer client.Close()
	defer server.Close()

	go func() {
		// Send Close frame with invalid 1-byte payload
		_ = client.writeControlFrame(OpClose, []byte{0x03})
	}()

	_, _, err := server.ReadMessage()
	if !errors.Is(err, ErrProtocolViolation) {
		t.Fatalf("expected ErrProtocolViolation for 1-byte close payload, got %v", err)
	}
}

// Ported from: TEST_F(WebSocketChannelEventInterfaceTest, ClosePayloadReservedStatusMessage)
func TestChromium_ClosePayloadReservedStatusMessage(t *testing.T) {
	client, server := tcpPair(t)
	defer client.Close()
	defer server.Close()

	go func() {
		// Code 1006 (0x03EE) on wire is forbidden
		var buf [2 + 17]byte
		binary.BigEndian.PutUint16(buf[:2], uint16(StatusAbnormalClosure))
		copy(buf[2:], "Not valid on wire")
		_ = client.writeControlFrame(OpClose, buf[:])
	}()

	_, _, err := server.ReadMessage()
	if !errors.Is(err, ErrInvalidCloseCode) {
		t.Fatalf("expected ErrInvalidCloseCode for 1006 on wire, got %v", err)
	}
}

// Ported from: TEST_F(WebSocketChannelStreamTest, CloseFrameInvalidUtf8)
func TestChromium_CloseFrameInvalidUtf8(t *testing.T) {
	client, server := tcpPair(t)
	defer client.Close()
	defer server.Close()

	go func() {
		// Valid code 1000 + invalid UTF-8 byte \xFF
		payload := []byte{0x03, 0xE8, 0xFF}
		_ = client.writeControlFrame(OpClose, payload)
	}()

	_, _, err := server.ReadMessage()
	if !errors.Is(err, ErrInvalidPayload) {
		t.Fatalf("expected ErrInvalidPayload for invalid UTF-8 close reason, got %v", err)
	}
}

// Ported from: TEST_F(WebSocketChannelEventInterfaceTest, DataAfterCloseIsRejected)
func TestChromium_DataAfterCloseIsRejected(t *testing.T) {
	client, server := tcpPair(t)
	defer client.Close()
	defer server.Close()

	go func() {
		// Send Close frame
		payload, _ := AppendClosePayload(StatusNormalClosure, "OK")
		_ = client.writeControlFrame(OpClose, payload)
		// Immediately follow with data frame (which must be rejected)
		_ = client.writeDataFrame(true, false, OpText, []byte("Payload"))
	}()

	// First ReadMessage consumes Close frame
	_, _, err := server.ReadMessage()
	var ce *CloseError
	if !errors.As(err, &ce) {
		t.Fatalf("expected CloseError, got %v", err)
	}

	// Second ReadMessage on that channel should reject or return ErrDataAfterClose / ErrClosed
	_, _, err2 := server.ReadMessage()
	if err2 == nil {
		t.Fatal("expected error reading after close, got nil")
	}
}

// Ported from: TEST_F(WebSocketChannelEventInterfaceTest, MaskedFramesAreRejected)
func TestChromium_MaskedFramesAreRejectedByClient(t *testing.T) {
	client, server := tcpPair(t)
	defer client.Close()
	defer server.Close()

	go func() {
		// Server incorrectly sends masked frame
		h := Header{
			Fin:     true,
			Opcode:  OpText,
			Masked:  true,
			MaskKey: [4]byte{1, 2, 3, 4},
			Length:  5,
		}
		data := []byte("Hello")
		MaskBytes(data, h.MaskKey)
		hdrBytes := AppendHeader(nil, h)
		_, _ = server.rw.Write(append(hdrBytes, data...))
	}()

	_, _, err := client.ReadMessage()
	if !errors.Is(err, ErrMaskForbidden) {
		t.Fatalf("expected ErrMaskForbidden when client receives masked frame, got %v", err)
	}
}

// Ported from RFC 6455 §5.1: server MUST close connection if unmasked frame received
func TestChromium_UnmaskedFramesAreRejectedByServer(t *testing.T) {
	client, server := tcpPair(t)
	defer client.Close()
	defer server.Close()

	go func() {
		// Client sends unmasked frame directly
		h := Header{
			Fin:    true,
			Opcode: OpText,
			Masked: false,
			Length: 5,
		}
		hdrBytes := AppendHeader(nil, h)
		_, _ = client.rw.Write(append(hdrBytes, []byte("Hello")...))
	}()

	_, _, err := server.ReadMessage()
	if !errors.Is(err, ErrMaskRequired) {
		t.Fatalf("expected ErrMaskRequired when server receives unmasked frame, got %v", err)
	}
}

// Ported from: TEST_F(WebSocketChannelEventInterfaceTest, UnknownOpCodeIsRejected)
func TestChromium_UnknownOpCodeIsRejected(t *testing.T) {
	client, server := tcpPair(t)
	defer client.Close()
	defer server.Close()

	go func() {
		// Send frame with reserved opcode 0x3
		h := Header{
			Fin:     true,
			Opcode:  Opcode(0x3),
			Masked:  true,
			MaskKey: [4]byte{1, 2, 3, 4},
			Length:  0,
		}
		hdrBytes := AppendHeader(nil, h)
		_, _ = client.rw.Write(hdrBytes)
	}()

	_, _, err := server.ReadMessage()
	if !errors.Is(err, ErrUnknownOpcode) {
		t.Fatalf("expected ErrUnknownOpcode, got %v", err)
	}
}

// Ported from: TEST_F(WebSocketChannelEventInterfaceTest, ReservedBitsMustNotBeSet)
func TestChromium_ReservedBitsMustNotBeSet(t *testing.T) {
	client, server := tcpPair(t)
	defer client.Close()
	defer server.Close()

	go func() {
		// Send frame with RSV2 set without extension
		h := Header{
			Fin:     true,
			Rsv2:    true,
			Opcode:  OpText,
			Masked:  true,
			MaskKey: [4]byte{1, 2, 3, 4},
			Length:  0,
		}
		hdrBytes := AppendHeader(nil, h)
		_, _ = client.rw.Write(hdrBytes)
	}()

	_, _, err := server.ReadMessage()
	if !errors.Is(err, ErrRSVSet) {
		t.Fatalf("expected ErrRSVSet, got %v", err)
	}
}

// Ported from: TEST_F(WebSocketBasicStreamSocketSingleReadTest, ControlFrameWithoutFin)
func TestChromium_ControlFrameWithoutFin(t *testing.T) {
	client, server := tcpPair(t)
	defer client.Close()
	defer server.Close()

	go func() {
		// Control frame Ping with FIN=0 (fragmented control frame is forbidden!)
		h := Header{
			Fin:     false,
			Opcode:  OpPing,
			Masked:  true,
			MaskKey: [4]byte{1, 2, 3, 4},
			Length:  0,
		}
		hdrBytes := AppendHeader(nil, h)
		_, _ = client.rw.Write(hdrBytes)
	}()

	_, _, err := server.ReadMessage()
	if !errors.Is(err, ErrControlFrameFragmented) {
		t.Fatalf("expected ErrControlFrameFragmented, got %v", err)
	}
}

// Ported from: TEST_F(WebSocketBasicStreamSocketSingleReadTest, OverlongControlFrame)
func TestChromium_OverlongControlFrame(t *testing.T) {
	client, server := tcpPair(t)
	defer client.Close()
	defer server.Close()

	go func() {
		// Ping frame with 126 bytes payload (control frames max 125 bytes)
		h := Header{
			Fin:     true,
			Opcode:  OpPing,
			Masked:  true,
			MaskKey: [4]byte{1, 2, 3, 4},
			Length:  126,
		}
		payload := make([]byte, 126)
		MaskBytes(payload, h.MaskKey)
		hdrBytes := AppendHeader(nil, h)
		_, _ = client.rw.Write(append(hdrBytes, payload...))
	}()

	_, _, err := server.ReadMessage()
	if !errors.Is(err, ErrControlFrameTooLarge) {
		t.Fatalf("expected ErrControlFrameTooLarge, got %v", err)
	}
}

// Ported from: TEST_F(WebSocketChannelEventInterfaceTest, ReadBinaryFramesAre8BitClean)
func TestChromium_ReadBinaryFramesAre8BitClean(t *testing.T) {
	client, server := tcpPair(t)
	defer client.Close()
	defer server.Close()

	// All 256 byte values
	allBytes := make([]byte, 256)
	for i := range allBytes {
		allBytes[i] = byte(i)
	}

	go func() {
		_ = client.WriteMessage(OpBinary, allBytes)
	}()

	op, msg, err := server.ReadMessage()
	if err != nil {
		t.Fatalf("unexpected read error: %v", err)
	}
	if op != OpBinary || !bytes.Equal(msg, allBytes) {
		t.Fatal("8-bit binary payload corruption detected!")
	}
}

// Simultaneous Close handshake
func TestChromium_SimultaneousCloseHandshake(t *testing.T) {
	client, server := tcpPair(t)
	defer client.Close()
	defer server.Close()

	var wg sync.WaitGroup
	wg.Add(2)

	var clientCloseErr, serverCloseErr error

	go func() {
		defer wg.Done()
		_ = client.WriteClose(StatusNormalClosure, "Client simultaneous close")
		_, _, clientCloseErr = client.ReadMessage()
	}()

	go func() {
		defer wg.Done()
		_ = server.WriteClose(StatusNormalClosure, "Server simultaneous close")
		_, _, serverCloseErr = server.ReadMessage()
	}()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		var ce1, ce2 *CloseError
		if !errors.As(clientCloseErr, &ce1) {
			t.Fatalf("client expected CloseError, got: %v", clientCloseErr)
		}
		if !errors.As(serverCloseErr, &ce2) {
			t.Fatalf("server expected CloseError, got: %v", serverCloseErr)
		}
		if client.State() != StateClosed || server.State() != StateClosed {
			t.Fatalf("expected both states Closed, got client=%v, server=%v", client.State(), server.State())
		}
	case <-time.After(3 * time.Second):
		t.Fatal("simultaneous close handshake timed out")
	}
}
