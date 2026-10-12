// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ws_test

import (
	"bytes"
	"errors"
	"math"
	"testing"

	"github.com/lemon4ksan/mach/proto/ws"
)

// Ported from Chromium net/websockets/websocket_handshake_challenge_test.cc
func TestChromium_WebSocketHandshakeChallenge_RFC6455(t *testing.T) {
	const key = "dGhlIHNhbXBsZSBub25jZQ=="
	accept := ws.ComputeAccept(key)
	const expected = "s3pPLMBiTxaQ9kYGzzhZRbK+xOo="
	if accept != expected {
		t.Fatalf("ComputeAccept(%q) = %q; want %q", key, accept, expected)
	}
}

// Ported from Chromium net/websockets/websocket_frame_test.cc: WebSocketFrameHeaderTest.FrameLengths
func TestChromium_WebSocketFrameHeader_FrameLengths(t *testing.T) {
	tests := []struct {
		frameHeader string
		frameLength int64
	}{
		{"\x81\x00", 0},
		{"\x81\x7D", 125},
		{"\x81\x7E\x00\x7E", 126},
		{"\x81\x7E\xFF\xFF", 0xFFFF},
		{"\x81\x7F\x00\x00\x00\x00\x00\x01\x00\x00", 0x10000},
		{"\x81\x7F\x7F\xFF\xFF\xFF\xFF\xFF\xFF\xFF", math.MaxInt64},
	}

	for _, tc := range tests {
		h := ws.Header{
			Fin:        true,
			Opcode:     ws.OpcodeText,
			PayloadLen: tc.frameLength,
		}

		buf := make([]byte, len(tc.frameHeader))
		n, err := ws.WriteHeader(buf, h)
		if err != nil {
			t.Fatalf("WriteHeader error for len %d: %v", tc.frameLength, err)
		}
		if n != len(tc.frameHeader) {
			t.Fatalf("WriteHeader wrote %d bytes; want %d", n, len(tc.frameHeader))
		}
		if !bytes.Equal(buf, []byte(tc.frameHeader)) {
			t.Fatalf("WriteHeader output mismatch: got %x; want %x", buf, []byte(tc.frameHeader))
		}

		// Verify ParseHeader decodes it back
		parsed, hlen, err := ws.ParseHeader([]byte(tc.frameHeader))
		if err != nil {
			t.Fatalf("ParseHeader error for len %d: %v", tc.frameLength, err)
		}
		if hlen != len(tc.frameHeader) {
			t.Fatalf("ParseHeader hlen = %d; want %d", hlen, len(tc.frameHeader))
		}
		if parsed.PayloadLen != tc.frameLength {
			t.Fatalf("ParseHeader payload length = %d; want %d", parsed.PayloadLen, tc.frameLength)
		}
		if !parsed.Fin || parsed.Opcode != ws.OpcodeText || parsed.Masked {
			t.Fatalf("ParseHeader header flags mismatch: %+v", parsed)
		}
	}
}

// Ported from Chromium net/websockets/websocket_frame_test.cc: WebSocketFrameHeaderTest.FrameLengthsWithMasking
func TestChromium_WebSocketFrameHeader_FrameLengthsWithMasking(t *testing.T) {
	key := ws.MaskKey{0xDE, 0xAD, 0xBE, 0xEF}
	tests := []struct {
		frameHeader string
		frameLength int64
	}{
		{"\x81\x80\xDE\xAD\xBE\xEF", 0},
		{"\x81\xFD\xDE\xAD\xBE\xEF", 125},
		{"\x81\xFE\x00\x7E\xDE\xAD\xBE\xEF", 126},
		{"\x81\xFE\xFF\xFF\xDE\xAD\xBE\xEF", 0xFFFF},
		{"\x81\xFF\x00\x00\x00\x00\x00\x01\x00\x00\xDE\xAD\xBE\xEF", 0x10000},
		{"\x81\xFF\x7F\xFF\xFF\xFF\xFF\xFF\xFF\xFF\xDE\xAD\xBE\xEF", math.MaxInt64},
	}

	for _, tc := range tests {
		h := ws.Header{
			Fin:        true,
			Opcode:     ws.OpcodeText,
			Masked:     true,
			MaskKey:    key,
			PayloadLen: tc.frameLength,
		}

		buf := make([]byte, len(tc.frameHeader))
		n, err := ws.WriteHeader(buf, h)
		if err != nil {
			t.Fatalf("WriteHeader error: %v", err)
		}
		if n != len(tc.frameHeader) {
			t.Fatalf("WriteHeader wrote %d bytes; want %d", n, len(tc.frameHeader))
		}
		if !bytes.Equal(buf, []byte(tc.frameHeader)) {
			t.Fatalf("WriteHeader output mismatch: got %x; want %x", buf, []byte(tc.frameHeader))
		}

		// Verify ParseHeader
		parsed, hlen, err := ws.ParseHeader([]byte(tc.frameHeader))
		if err != nil {
			t.Fatalf("ParseHeader error: %v", err)
		}
		if hlen != len(tc.frameHeader) {
			t.Fatalf("ParseHeader hlen = %d; want %d", hlen, len(tc.frameHeader))
		}
		if parsed.PayloadLen != tc.frameLength || parsed.MaskKey != key || !parsed.Masked {
			t.Fatalf("ParseHeader parsed incorrectly: %+v", parsed)
		}
	}
}

// Ported from Chromium net/websockets/websocket_frame_test.cc: WebSocketFrameHeaderTest.FrameOpCodes
func TestChromium_WebSocketFrameHeader_FrameOpCodes(t *testing.T) {
	tests := []struct {
		frameHeader string
		opcode      ws.Opcode
	}{
		{"\x80\x00", ws.OpcodeContinuation},
		{"\x81\x00", ws.OpcodeText},
		{"\x82\x00", ws.OpcodeBinary},
		{"\x88\x00", ws.OpcodeClose},
		{"\x89\x00", ws.OpcodePing},
		{"\x8A\x00", ws.OpcodePong},
		// Undefined opcodes
		{"\x83\x00", 0x3},
		{"\x84\x00", 0x4},
		{"\x85\x00", 0x5},
		{"\x86\x00", 0x6},
		{"\x87\x00", 0x7},
		{"\x8B\x00", 0xB},
		{"\x8C\x00", 0xC},
		{"\x8D\x00", 0xD},
		{"\x8E\x00", 0xE},
		{"\x8F\x00", 0xF},
	}

	for _, tc := range tests {
		h := ws.Header{
			Fin:        true,
			Opcode:     tc.opcode,
			PayloadLen: 0,
		}

		buf := make([]byte, len(tc.frameHeader))
		n, err := ws.WriteHeader(buf, h)
		if err != nil {
			t.Fatalf("WriteHeader opcode %v error: %v", tc.opcode, err)
		}
		if n != 2 || !bytes.Equal(buf, []byte(tc.frameHeader)) {
			t.Fatalf("WriteHeader output mismatch for opcode %v", tc.opcode)
		}

		parsed, _, err := ws.ParseHeader([]byte(tc.frameHeader))
		if err != nil {
			t.Fatalf("ParseHeader opcode %v error: %v", tc.opcode, err)
		}
		if parsed.Opcode != tc.opcode {
			t.Fatalf("ParseHeader opcode got %v; want %v", parsed.Opcode, tc.opcode)
		}
	}
}

// Ported from Chromium net/websockets/websocket_frame_test.cc: WebSocketFrameHeaderTest.FinalBitAndReservedBits
func TestChromium_WebSocketFrameHeader_FinalBitAndReservedBits(t *testing.T) {
	tests := []struct {
		frameHeader string
		fin         bool
		rsv1        bool
		rsv2        bool
		rsv3        bool
	}{
		{"\x81\x00", true, false, false, false},
		{"\x01\x00", false, false, false, false},
		{"\xC1\x00", true, true, false, false},
		{"\xA1\x00", true, false, true, false},
		{"\x91\x00", true, false, false, true},
		{"\x71\x00", false, true, true, true},
		{"\xF1\x00", true, true, true, true},
	}

	for _, tc := range tests {
		h := ws.Header{
			Fin:        tc.fin,
			Rsv1:       tc.rsv1,
			Rsv2:       tc.rsv2,
			Rsv3:       tc.rsv3,
			Opcode:     ws.OpcodeText,
			PayloadLen: 0,
		}

		buf := make([]byte, len(tc.frameHeader))
		n, err := ws.WriteHeader(buf, h)
		if err != nil {
			t.Fatalf("WriteHeader error: %v", err)
		}
		if n != 2 || !bytes.Equal(buf, []byte(tc.frameHeader)) {
			t.Fatalf("WriteHeader mismatch: got %x; want %x", buf, []byte(tc.frameHeader))
		}

		parsed, _, err := ws.ParseHeader([]byte(tc.frameHeader))
		if err != nil {
			t.Fatalf("ParseHeader error: %v", err)
		}
		if parsed.Fin != tc.fin || parsed.Rsv1 != tc.rsv1 || parsed.Rsv2 != tc.rsv2 || parsed.Rsv3 != tc.rsv3 {
			t.Fatalf("ParseHeader flags mismatch: got %+v; want Fin=%v Rsv1=%v Rsv2=%v Rsv3=%v",
				parsed, tc.fin, tc.rsv1, tc.rsv2, tc.rsv3)
		}
	}
}

// Ported from Chromium net/websockets/websocket_frame_test.cc: WebSocketFrameHeaderTest.InsufficientBufferSize
func TestChromium_WebSocketFrameHeader_InsufficientBufferSize(t *testing.T) {
	tests := []struct {
		payloadLen         int64
		masked             bool
		expectedHeaderSize int
	}{
		{0, false, 2},
		{125, false, 2},
		{126, false, 4},
		{0xFFFF, false, 4},
		{0x10000, false, 10},
		{math.MaxInt64, false, 10},
		{0, true, 6},
		{125, true, 6},
		{126, true, 8},
		{0xFFFF, true, 8},
		{0x10000, true, 14},
		{math.MaxInt64, true, 14},
	}

	for _, tc := range tests {
		h := ws.Header{
			Fin:        true,
			Opcode:     ws.OpcodeText,
			Masked:     tc.masked,
			PayloadLen: tc.payloadLen,
		}

		buf := make([]byte, tc.expectedHeaderSize-1)
		_, err := ws.WriteHeader(buf, h)
		if !errors.Is(err, ws.ErrBufferTooSmall) {
			t.Fatalf("expected ErrBufferTooSmall for size %d, got %v", len(buf), err)
		}
	}
}

// Ported from Chromium net/websockets/websocket_frame_test.cc: WebSocketFrameTest.MaskPayload
func TestChromium_WebSocketFrame_MaskPayload(t *testing.T) {
	tests := []struct {
		maskingKey [4]byte
		offset     int
		input      string
		output     string
	}{
		{[4]byte{0xDE, 0xAD, 0xBE, 0xEF}, 0, "FooBar", "\x98\xC2\xD1\xAD\xBF\xDF"},
		{[4]byte{0xDE, 0xAD, 0xBE, 0xEF}, 1, "FooBar", "\xEB\xD1\x80\x9C\xCC\xCC"},
		{[4]byte{0xDE, 0xAD, 0xBE, 0xEF}, 2, "FooBar", "\xF8\x80\xB1\xEF\xDF\x9D"},
		{[4]byte{0xDE, 0xAD, 0xBE, 0xEF}, 3, "FooBar", "\xA9\xB1\xC2\xFC\x8E\xAC"},
		{[4]byte{0xDE, 0xAD, 0xBE, 0xEF}, 4, "FooBar", "\x98\xC2\xD1\xAD\xBF\xDF"},
		{[4]byte{0xDE, 0xAD, 0xBE, 0xEF}, 42, "FooBar", "\xF8\x80\xB1\xEF\xDF\x9D"},
		{[4]byte{0xDE, 0xAD, 0xBE, 0xEF}, 0, "", ""},
		{[4]byte{0xDE, 0xAD, 0xBE, 0xEF}, 0, "\xDE\xAD\xBE\xEF", "\x00\x00\x00\x00"},
		{[4]byte{0xDE, 0xAD, 0xBE, 0xEF}, 0, "\x00\x00\x00\x00", "\xDE\xAD\xBE\xEF"},
		{[4]byte{0x00, 0x00, 0x00, 0x00}, 0, "FooBar", "FooBar"},
		{[4]byte{0xFF, 0xFF, 0xFF, 0xFF}, 0, "FooBar", "\xB9\x90\x90\xBD\x9E\x8D"},
	}

	for i, tc := range tests {
		data := []byte(tc.input)
		ws.MaskWithOffset(data, tc.maskingKey, tc.offset)
		if !bytes.Equal(data, []byte(tc.output)) {
			t.Fatalf("[%d] MaskWithOffset output = %x; want %x", i, data, []byte(tc.output))
		}
	}
}

// Ported from Chromium net/websockets/websocket_frame_test.cc: WebSocketFrameTest.MaskPayloadAlignment
func TestChromium_WebSocketFrame_MaskPayloadAlignment(t *testing.T) {
	const maxVectorAlignment = 32
	const maxVectorSize = 32
	const maskingKeyLength = 4
	const scratchBufferSize = maxVectorAlignment + maxVectorSize*2

	testMask := [4]byte{0xD2, 0xBA, 0x5A, 0xBE}
	testInput := []byte(
		"\x3d\x77\x1d\x1b\x19\x8c\x48\xa3\x19\x6d\xf7\xcc\x39\xe7\x57\x0b" +
			"\x69\x8c\xda\x4b\xfc\xac\x2c\xd3\x49\x96\x6e\x8a\x7b\x5a\x32\x76" +
			"\xd0\x11\x43\xa0\x89\xfc\x76\x2b\x10\x2f\x4c\x7b\x4f\xa6\xdd\xe4" +
			"\xfc\x8e\xd8\x72\xcf\x7e\x37\xcd\x31\xcd\xc1\xc0\x89\x0c\xa7\x4c" +
			"\xda\xa8\x4b\x75\xa1\xcb\xa9\x77\x19\x4d\x6e\xdf\xc8\x08\x1c\xb6" +
			"\x6d\xfb\x38\x04\x44\xd5\xba\x57\x9f\x76\xb0\x2e\x07\x91\xe6\xa8")

	testOutput := []byte(
		"\xef\xcd\x47\xa5\xcb\x36\x12\x1d\xcb\xd7\xad\x72\xeb\x5d\x0d\xb5" +
			"\xbb\x36\x80\xf5\x2e\x16\x76\x6d\x9b\x2c\x34\x34\xa9\xe0\x68\xc8" +
			"\x02\xab\x19\x1e\x5b\x46\x2c\x95\xc2\x95\x16\xc5\x9d\x1c\x87\x5a" +
			"\x2e\x34\x82\xcc\x1d\xc4\x6d\x73\xe3\x77\x9b\x7e\x5b\xb6\xfd\xf2" +
			"\x08\x12\x11\xcb\x73\x71\xf3\xc9\xcb\xf7\x34\x61\x1a\xb2\x46\x08" +
			"\xbf\x41\x62\xba\x96\x6f\xe0\xe9\x4d\xcc\xea\x90\xd5\x2b\xbc\x16")

	testInputSize := len(testInput)
	scratch := make([]byte, scratchBufferSize)

	for frameOffset := 0; frameOffset < maskingKeyLength; frameOffset++ {
		for alignment := 0; alignment < maxVectorAlignment; alignment++ {
			avail := min(scratchBufferSize-alignment, testInputSize-frameOffset)
			for chunkSize := 1; chunkSize < maxVectorSize; chunkSize++ {
				// Copy test input
				copy(scratch[alignment:alignment+avail], testInput[frameOffset:frameOffset+avail])

				for chunkStart := 0; chunkStart < avail; chunkStart += chunkSize {
					thisChunkSize := min(chunkSize, avail-chunkStart)
					chunkSlice := scratch[alignment+chunkStart : alignment+chunkStart+thisChunkSize]
					ws.MaskWithOffset(chunkSlice, testMask, frameOffset+chunkStart)
				}

				expected := testOutput[frameOffset : frameOffset+avail]
				actual := scratch[alignment : alignment+avail]
				if !bytes.Equal(actual, expected) {
					t.Fatalf(
						"MaskPayloadAlignment mismatch: frameOffset=%d, alignment=%d, chunkSize=%d\ngot:  %x\nwant: %x",
						frameOffset,
						alignment,
						chunkSize,
						actual,
						expected,
					)
				}
			}
		}
	}
}

// Ported from Chromium net/websockets/websocket_frame_test.cc: Opcode helper tests
func TestChromium_WebSocketFrameHeader_OpcodeClassifiers(t *testing.T) {
	// Data opcodes
	if !ws.OpcodeContinuation.IsData() || !ws.OpcodeText.IsData() || !ws.OpcodeBinary.IsData() {
		t.Fatal("Data opcodes should return true for IsData")
	}
	if ws.OpcodeClose.IsData() || ws.OpcodePing.IsData() || ws.OpcodePong.IsData() {
		t.Fatal("Control opcodes must return false for IsData")
	}

	// Control opcodes
	if ws.OpcodeContinuation.IsControl() || ws.OpcodeText.IsControl() || ws.OpcodeBinary.IsControl() {
		t.Fatal("Data opcodes must return false for IsControl")
	}
	if !ws.OpcodeClose.IsControl() || !ws.OpcodePing.IsControl() || !ws.OpcodePong.IsControl() {
		t.Fatal("Control opcodes must return true for IsControl")
	}

	// Reserved data opcodes
	if ws.OpcodeContinuation.IsReserved() || ws.OpcodeText.IsReserved() || ws.OpcodeBinary.IsReserved() {
		t.Fatal("Standard data opcodes must not be reserved")
	}
	for op := ws.Opcode(0x3); op <= 0x7; op++ {
		if !op.IsReserved() {
			t.Fatalf("Opcode 0x%X should be reserved", op)
		}
	}

	// Reserved control opcodes
	if ws.OpcodeClose.IsReserved() || ws.OpcodePing.IsReserved() || ws.OpcodePong.IsReserved() {
		t.Fatal("Standard control opcodes must not be reserved")
	}
	for op := ws.Opcode(0xB); op <= 0xF; op++ {
		if !op.IsReserved() {
			t.Fatalf("Opcode 0x%X should be reserved", op)
		}
	}
}

// Ported from Chromium net/websockets/websocket_frame_parser_test.cc: WebSocketFrameParserTest.DecodeNormalFrame
func TestChromium_WebSocketFrameParser_DecodeNormalFrame(t *testing.T) {
	const hello = "Hello, world!"
	helloFrame := []byte("\x81\x0DHello, world!")

	var parser ws.Parser
	var frames []ws.FrameChunk
	err := parser.Decode(helloFrame, &frames)
	if err != nil {
		t.Fatalf("Decode error: %v", err)
	}
	if len(frames) != 1 {
		t.Fatalf("expected 1 frame chunk, got %d", len(frames))
	}
	chunk := frames[0]
	if chunk.Header == nil {
		t.Fatal("expected non-nil header")
	}
	if !chunk.Header.Fin || chunk.Header.Rsv1 || chunk.Header.Rsv2 || chunk.Header.Rsv3 {
		t.Fatalf("unexpected header flags: %+v", chunk.Header)
	}
	if chunk.Header.Opcode != ws.OpcodeText {
		t.Fatalf("opcode = %v; want Text", chunk.Header.Opcode)
	}
	if chunk.Header.Masked {
		t.Fatal("expected unmasked frame")
	}
	if chunk.Header.PayloadLen != int64(len(hello)) {
		t.Fatalf("payload len = %d; want %d", chunk.Header.PayloadLen, len(hello))
	}
	if !chunk.FinalChunk {
		t.Fatal("expected FinalChunk to be true")
	}
	if string(chunk.Payload) != hello {
		t.Fatalf("payload = %q; want %q", chunk.Payload, hello)
	}
}

// Ported from Chromium net/websockets/websocket_frame_parser_test.cc: WebSocketFrameParserTest.DecodeMaskedFrame
func TestChromium_WebSocketFrameParser_DecodeMaskedFrame(t *testing.T) {
	const hello = "Hello, world!"
	maskedHelloFrame := []byte("\x81\x8D\xDE\xAD\xBE\xEF\x96\xC8\xD2\x83\xB1\x81\x9E\x98\xB1\xDF\xD2\x8B\xFF")

	var parser ws.Parser
	var frames []ws.FrameChunk
	err := parser.Decode(maskedHelloFrame, &frames)
	if err != nil {
		t.Fatalf("Decode error: %v", err)
	}
	if len(frames) != 1 {
		t.Fatalf("expected 1 frame chunk, got %d", len(frames))
	}
	chunk := frames[0]
	if chunk.Header == nil {
		t.Fatal("expected non-nil header")
	}
	if !chunk.Header.Fin || !chunk.Header.Masked {
		t.Fatalf("unexpected flags: %+v", chunk.Header)
	}
	if chunk.Header.Opcode != ws.OpcodeText {
		t.Fatalf("opcode = %v; want Text", chunk.Header.Opcode)
	}

	// Unmask payload and verify
	payload := make([]byte, len(chunk.Payload))
	copy(payload, chunk.Payload)
	ws.Mask(payload, chunk.Header.MaskKey)
	if string(payload) != hello {
		t.Fatalf("unmasked payload = %q; want %q", payload, hello)
	}
}

// Ported from Chromium net/websockets/websocket_frame_parser_test.cc: WebSocketFrameParserTest.DecodeManyFrames
func TestChromium_WebSocketFrameParser_DecodeManyFrames(t *testing.T) {
	inputs := []struct {
		frame   string
		payload string
	}{
		{"\x81\x05First", "First"},
		{"\x81\x06Second", "Second"},
		{"\x81\x05Third", "Third"},
		{"\x81\x06Fourth", "Fourth"},
		{"\x81\x05Fifth", "Fifth"},
		{"\x81\x05Sixth", "Sixth"},
		{"\x81\x07Seventh", "Seventh"},
		{"\x81\x06Eighth", "Eighth"},
		{"\x81\x05Ninth", "Ninth"},
		{"\x81\x05Tenth", "Tenth"},
	}

	var combined []byte
	for _, in := range inputs {
		combined = append(combined, in.frame...)
	}

	var parser ws.Parser
	var frames []ws.FrameChunk
	err := parser.Decode(combined, &frames)
	if err != nil {
		t.Fatalf("Decode error: %v", err)
	}
	if len(frames) != len(inputs) {
		t.Fatalf("got %d frames; want %d", len(frames), len(inputs))
	}

	for i, in := range inputs {
		f := frames[i]
		if f.Header == nil {
			t.Fatalf("[%d] expected header", i)
		}
		if !f.FinalChunk {
			t.Fatalf("[%d] expected final chunk", i)
		}
		if string(f.Payload) != in.payload {
			t.Fatalf("[%d] payload = %q; want %q", i, f.Payload, in.payload)
		}
		if f.Header.Opcode != ws.OpcodeText {
			t.Fatalf("[%d] opcode = %v; want Text", i, f.Header.Opcode)
		}
	}
}

// Ported from Chromium net/websockets/websocket_frame_parser_test.cc: WebSocketFrameParserTest.DecodePartialFrame
func TestChromium_WebSocketFrameParser_DecodePartialFrame(t *testing.T) {
	const hello = "Hello, world!"
	helloFrame := []byte("\x81\x0DHello, world!")
	const frameHeaderSize = 2

	for cuttingPos := 0; cuttingPos < len(hello); cuttingPos++ {
		input1 := helloFrame[:frameHeaderSize+cuttingPos]
		input2 := helloFrame[frameHeaderSize+cuttingPos:]

		expected1 := hello[:cuttingPos]
		expected2 := hello[cuttingPos:]

		var parser ws.Parser

		var frames1 []ws.FrameChunk
		if err := parser.Decode(input1, &frames1); err != nil {
			t.Fatalf("Decode input1 cut=%d error: %v", cuttingPos, err)
		}
		if len(frames1) != 1 {
			t.Fatalf("cut=%d: frames1 len = %d; want 1", cuttingPos, len(frames1))
		}
		f1 := frames1[0]
		if f1.Header == nil {
			t.Fatalf("cut=%d: expected header on first chunk", cuttingPos)
		}
		if f1.FinalChunk {
			t.Fatalf("cut=%d: chunk1 should not be final", cuttingPos)
		}
		if string(f1.Payload) != expected1 {
			t.Fatalf("cut=%d: payload1 = %q; want %q", cuttingPos, f1.Payload, expected1)
		}

		var frames2 []ws.FrameChunk
		if err := parser.Decode(input2, &frames2); err != nil {
			t.Fatalf("Decode input2 cut=%d error: %v", cuttingPos, err)
		}
		if len(frames2) != 1 {
			t.Fatalf("cut=%d: frames2 len = %d; want 1", cuttingPos, len(frames2))
		}
		f2 := frames2[0]
		if f2.Header != nil {
			t.Fatalf("cut=%d: expected nil header on continuation chunk", cuttingPos)
		}
		if !f2.FinalChunk {
			t.Fatalf("cut=%d: chunk2 should be final", cuttingPos)
		}
		if string(f2.Payload) != expected2 {
			t.Fatalf("cut=%d: payload2 = %q; want %q", cuttingPos, f2.Payload, expected2)
		}
	}
}

// Ported from Chromium net/websockets/websocket_frame_parser_test.cc: WebSocketFrameParserTest.DecodePartialMaskedFrame
func TestChromium_WebSocketFrameParser_DecodePartialMaskedFrame(t *testing.T) {
	const hello = "Hello, world!"
	maskedHelloFrame := []byte("\x81\x8D\xDE\xAD\xBE\xEF\x96\xC8\xD2\x83\xB1\x81\x9E\x98\xB1\xDF\xD2\x8B\xFF")
	const frameHeaderSize = 6
	key := ws.MaskKey{0xDE, 0xAD, 0xBE, 0xEF}

	for cuttingPos := 0; cuttingPos < len(hello); cuttingPos++ {
		input1 := maskedHelloFrame[:frameHeaderSize+cuttingPos]
		input2 := maskedHelloFrame[frameHeaderSize+cuttingPos:]

		expected1 := hello[:cuttingPos]
		expected2 := hello[cuttingPos:]

		var parser ws.Parser

		var frames1 []ws.FrameChunk
		if err := parser.Decode(input1, &frames1); err != nil {
			t.Fatalf("Decode input1 cut=%d error: %v", cuttingPos, err)
		}
		if len(frames1) != 1 {
			t.Fatalf("cut=%d: frames1 len = %d; want 1", cuttingPos, len(frames1))
		}
		f1 := frames1[0]
		if f1.Header == nil {
			t.Fatalf("cut=%d: expected header on first chunk", cuttingPos)
		}
		p1 := make([]byte, len(f1.Payload))
		copy(p1, f1.Payload)
		ws.MaskWithOffset(p1, key, 0)
		if string(p1) != expected1 {
			t.Fatalf("cut=%d: unmasked payload1 = %q; want %q", cuttingPos, p1, expected1)
		}

		var frames2 []ws.FrameChunk
		if err := parser.Decode(input2, &frames2); err != nil {
			t.Fatalf("Decode input2 cut=%d error: %v", cuttingPos, err)
		}
		if len(frames2) != 1 {
			t.Fatalf("cut=%d: frames2 len = %d; want 1", cuttingPos, len(frames2))
		}
		f2 := frames2[0]
		if f2.Header != nil {
			t.Fatalf("cut=%d: continuation chunk must have nil Header", cuttingPos)
		}
		p2 := make([]byte, len(f2.Payload))
		copy(p2, f2.Payload)
		ws.MaskWithOffset(p2, key, cuttingPos)
		if string(p2) != expected2 {
			t.Fatalf("cut=%d: unmasked payload2 = %q; want %q", cuttingPos, p2, expected2)
		}
	}
}

// Ported from Chromium net/websockets/websocket_frame_parser_test.cc: WebSocketFrameParserTest.InvalidLengthEncoding
func TestChromium_WebSocketFrameParser_InvalidLengthEncoding(t *testing.T) {
	tests := [][]byte{
		// 16-bit extended length for < 126
		[]byte("\x81\x7E\x00\x00"),
		[]byte("\x81\x7E\x00\x7D"),
		// 64-bit extended length for <= 0xFFFF
		[]byte("\x81\x7F\x00\x00\x00\x00\x00\x00\x00\x00"),
		[]byte("\x81\x7F\x00\x00\x00\x00\x00\x00\xFF\xFF"),
	}

	for i, tc := range tests {
		var parser ws.Parser
		var frames []ws.FrameChunk
		err := parser.Decode(tc, &frames)
		if !errors.Is(err, ws.ErrInvalidLength) {
			t.Fatalf("[%d] expected ErrInvalidLength, got %v", i, err)
		}

		// Subsequent calls should return the same sticky error
		err2 := parser.Decode(nil, &frames)
		if !errors.Is(err2, ws.ErrInvalidLength) {
			t.Fatalf("[%d] sticky error mismatch, got %v", i, err2)
		}
	}
}

// Ported from Chromium net/websockets/websocket_frame_parser_test.cc: WebSocketFrameParserTest.DecodePartialHeader
func TestChromium_WebSocketFrameParser_DecodePartialHeader(t *testing.T) {
	tests := []struct {
		header     []byte
		payloadLen int64
		err        error
	}{
		{[]byte("\x81\x00"), 0, nil},
		{[]byte("\x81\x7D"), 125, nil},
		{[]byte("\x81\x7E\x00\x7E"), 126, nil},
		{[]byte("\x81\x7E\xFF\xFF"), 0xFFFF, nil},
		{[]byte("\x81\x7F\x00\x00\x00\x00\x00\x01\x00\x00"), 0x10000, nil},
		{[]byte("\x81\x7F\x00\x00\x00\x00\x7F\xFF\xFF\xFF"), 0x7FFFFFFF, nil},
		// MSB set (invalid / too big)
		{[]byte("\x81\x7F\x80\x00\x00\x00\x00\x00\x00\x00"), 0, ws.ErrPayloadTooLarge},
	}

	for i, tc := range tests {
		var parser ws.Parser
		var frames []ws.FrameChunk

		lastByte := len(tc.header) - 1
		for j := 0; j < len(tc.header); j++ {
			err := parser.Decode(tc.header[j:j+1], &frames)
			isLast := (j == lastByte)

			if tc.err != nil && isLast {
				if !errors.Is(err, tc.err) {
					t.Fatalf("[%d] byte %d: expected err %v, got %v", i, j, tc.err, err)
				}
				break
			}
			if err != nil {
				t.Fatalf("[%d] byte %d unexpected error: %v", i, j, err)
			}
			if tc.err == nil && isLast {
				if len(frames) != 1 {
					t.Fatalf("[%d] expected 1 frame at final byte, got %d", i, len(frames))
				}
				if frames[0].Header.PayloadLen != tc.payloadLen {
					t.Fatalf("[%d] payload length = %d; want %d", i, frames[0].Header.PayloadLen, tc.payloadLen)
				}
			} else {
				if len(frames) != 0 {
					t.Fatalf("[%d] byte %d should have 0 frames, got %d", i, j, len(frames))
				}
			}
		}
	}
}

// Ported from Chromium ParseCloseFrame tests in net/websockets/websocket_frame.cc
func TestChromium_ParseCloseFrame(t *testing.T) {
	// Empty payload -> StatusNoStatusReceived
	code, reason, err := ws.ParseClosePayload(nil)
	if err != nil || code != ws.StatusNoStatusReceived || reason != "" {
		t.Fatalf("empty close frame: got code=%v, reason=%q, err=%v", code, reason, err)
	}

	// 1-byte payload -> ProtocolError
	_, _, err = ws.ParseClosePayload([]byte{0x03})
	if !errors.Is(err, ws.ErrInvalidClosePayload) {
		t.Fatalf("1-byte close frame: expected ErrInvalidClosePayload, got %v", err)
	}

	// Reserved codes: 1005, 1006, 1015
	for _, reserved := range []uint16{1005, 1006, 1015} {
		payload := []byte{byte(reserved >> 8), byte(reserved)}
		_, _, err := ws.ParseClosePayload(payload)
		if !errors.Is(err, ws.ErrInvalidCloseCode) {
			t.Fatalf("reserved code %d: expected ErrInvalidCloseCode, got %v", reserved, err)
		}
	}

	// Valid code without reason (e.g. 1000)
	code, reason, err = ws.ParseClosePayload([]byte{0x03, 0xE8})
	if err != nil || code != ws.StatusNormalClosure || reason != "" {
		t.Fatalf("code 1000 without reason: got code=%v, reason=%q, err=%v", code, reason, err)
	}

	// Valid code with valid UTF-8 reason
	code, reason, err = ws.ParseClosePayload([]byte("\x03\xE8Normal closing"))
	if err != nil || code != ws.StatusNormalClosure || reason != "Normal closing" {
		t.Fatalf("valid reason: got code=%v, reason=%q, err=%v", code, reason, err)
	}

	// Valid code with invalid UTF-8 reason
	_, _, err = ws.ParseClosePayload([]byte("\x03\xE8\xFF\xFF"))
	if !errors.Is(err, ws.ErrInvalidUTF8) {
		t.Fatalf("invalid UTF-8 reason: expected ErrInvalidUTF8, got %v", err)
	}
}
