// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ws_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/lemon4ksan/mach/proto/ws"
)

func TestWire_ParseHeader_Incomplete(t *testing.T) {
	// Empty slice
	_, _, err := ws.ParseHeader(nil)
	if !errors.Is(err, ws.ErrIncomplete) {
		t.Fatalf("expected ErrIncomplete, got %v", err)
	}

	// 1 byte
	_, _, err = ws.ParseHeader([]byte{0x81})
	if !errors.Is(err, ws.ErrIncomplete) {
		t.Fatalf("expected ErrIncomplete, got %v", err)
	}

	// 126 length but only 3 bytes
	_, _, err = ws.ParseHeader([]byte{0x81, 126, 0x01})
	if !errors.Is(err, ws.ErrIncomplete) {
		t.Fatalf("expected ErrIncomplete, got %v", err)
	}

	// 127 length but only 9 bytes
	_, _, err = ws.ParseHeader([]byte{0x81, 127, 0, 0, 0, 0, 0, 1, 0})
	if !errors.Is(err, ws.ErrIncomplete) {
		t.Fatalf("expected ErrIncomplete, got %v", err)
	}

	// Masked bit set but missing mask key bytes
	_, _, err = ws.ParseHeader([]byte{0x81, 0x80, 0x01, 0x02})
	if !errors.Is(err, ws.ErrIncomplete) {
		t.Fatalf("expected ErrIncomplete, got %v", err)
	}
}

func TestWire_ControlFrameValidation(t *testing.T) {
	// Control frame payload > 125 must fail
	longPing := []byte{0x89, 126, 0x00, 126}
	_, _, err := ws.ParseHeader(longPing)
	if !errors.Is(err, ws.ErrControlFrameTooLarge) {
		t.Fatalf("expected ErrControlFrameTooLarge, got %v", err)
	}

	// Control frame with Fin=false must fail
	fragPing := []byte{0x09, 0x00}
	_, _, err = ws.ParseHeader(fragPing)
	if !errors.Is(err, ws.ErrControlFrameFragmented) {
		t.Fatalf("expected ErrControlFrameFragmented, got %v", err)
	}

	// WriteHeader must reject control frame payload > 125
	buf := make([]byte, 14)
	_, err = ws.WriteHeader(buf, ws.Header{
		Fin:        true,
		Opcode:     ws.OpcodePing,
		PayloadLen: 126,
	})
	if !errors.Is(err, ws.ErrControlFrameTooLarge) {
		t.Fatalf("WriteHeader: expected ErrControlFrameTooLarge, got %v", err)
	}

	// WriteHeader must reject fragmented control frame
	_, err = ws.WriteHeader(buf, ws.Header{
		Fin:        false,
		Opcode:     ws.OpcodePing,
		PayloadLen: 0,
	})
	if !errors.Is(err, ws.ErrControlFrameFragmented) {
		t.Fatalf("WriteHeader: expected ErrControlFrameFragmented, got %v", err)
	}
}

func TestWire_WriteFrame_And_AppendFrame(t *testing.T) {
	payload := []byte("hello, websocket framing engine!")
	key := ws.MaskKey{0x12, 0x34, 0x56, 0x78}

	h := ws.Header{
		Fin:        true,
		Opcode:     ws.OpcodeText,
		Masked:     true,
		MaskKey:    key,
		PayloadLen: int64(len(payload)),
	}

	// WriteFrame
	dst := make([]byte, ws.HeaderLen(h)+len(payload))
	n, err := ws.WriteFrame(dst, h, payload)
	if err != nil {
		t.Fatalf("WriteFrame error: %v", err)
	}
	if n != len(dst) {
		t.Fatalf("WriteFrame wrote %d; want %d", n, len(dst))
	}

	// Verify original payload wasn't modified
	if !bytes.Equal(payload, []byte("hello, websocket framing engine!")) {
		t.Fatal("source payload was mutated by WriteFrame")
	}

	// Parse header back
	parsedH, hlen, err := ws.ParseHeader(dst)
	if err != nil {
		t.Fatalf("ParseHeader error: %v", err)
	}
	if hlen != ws.HeaderLen(h) {
		t.Fatalf("hlen = %d; want %d", hlen, ws.HeaderLen(h))
	}

	// Unmask and compare
	extractedPayload := make([]byte, len(dst)-hlen)
	copy(extractedPayload, dst[hlen:])
	ws.Mask(extractedPayload, parsedH.MaskKey)
	if !bytes.Equal(extractedPayload, payload) {
		t.Fatalf("extracted unmasked payload mismatch: got %q; want %q", extractedPayload, payload)
	}

	// AppendFrame
	var appBuf []byte
	appBuf = ws.AppendFrame(appBuf, h, payload)
	if !bytes.Equal(appBuf, dst) {
		t.Fatalf("AppendFrame result does not match WriteFrame result")
	}
}

func TestWire_ParseClosePayload_And_Format(t *testing.T) {
	tests := []struct {
		code   ws.CloseCode
		reason string
	}{
		{ws.StatusNormalClosure, "regular close"},
		{ws.StatusGoingAway, "server restarting"},
		{ws.StatusProtocolError, "protocol error"},
		{ws.StatusPolicyViolation, "policy violated"},
		{4000, "custom application close"},
		{4999, "max private range code"},
	}

	for _, tc := range tests {
		buf := make([]byte, 2+len(tc.reason))
		n, err := ws.FormatClosePayload(buf, tc.code, tc.reason)
		if err != nil {
			t.Fatalf("FormatClosePayload error: %v", err)
		}
		if n != len(buf) {
			t.Fatalf("FormatClosePayload wrote %d; want %d", n, len(buf))
		}

		code, reason, err := ws.ParseClosePayload(buf)
		if err != nil {
			t.Fatalf("ParseClosePayload error: %v", err)
		}
		if code != tc.code {
			t.Fatalf("parsed code = %v; want %v", code, tc.code)
		}
		if reason != tc.reason {
			t.Fatalf("parsed reason = %q; want %q", reason, tc.reason)
		}
	}
}

func TestWire_OpcodeAndCloseCodeStrings(t *testing.T) {
	opcodes := []ws.Opcode{
		ws.OpcodeContinuation,
		ws.OpcodeText,
		ws.OpcodeBinary,
		ws.OpcodeClose,
		ws.OpcodePing,
		ws.OpcodePong,
		ws.Opcode(0x5),
	}

	for _, op := range opcodes {
		s := op.String()
		if len(s) == 0 {
			t.Fatalf("empty string for opcode %v", op)
		}
	}

	codes := []ws.CloseCode{
		ws.StatusNormalClosure,
		ws.StatusGoingAway,
		ws.StatusProtocolError,
		ws.StatusUnsupportedData,
		ws.StatusNoStatusReceived,
		ws.StatusAbnormalClosure,
		ws.StatusInvalidFramePayloadData,
		ws.StatusPolicyViolation,
		ws.StatusMessageTooBig,
		ws.StatusMandatoryExtension,
		ws.StatusInternalServerError,
		ws.StatusTLSHandshake,
		ws.CloseCode(4500),
	}

	for _, c := range codes {
		s := c.String()
		if len(s) == 0 {
			t.Fatalf("empty string for close code %v", c)
		}
	}
}

func TestWire_WriteHeader_BufferTooSmall(t *testing.T) {
	h := ws.Header{
		Fin:        true,
		Opcode:     ws.OpcodeText,
		PayloadLen: 1000, // 4 bytes header
	}

	small := make([]byte, 3)
	_, err := ws.WriteHeader(small, h)
	if !errors.Is(err, ws.ErrBufferTooSmall) {
		t.Fatalf("expected ErrBufferTooSmall, got %v", err)
	}

	// Negative length
	h.PayloadLen = -1
	_, err = ws.WriteHeader(make([]byte, 14), h)
	if !errors.Is(err, ws.ErrInvalidLength) {
		t.Fatalf("expected ErrInvalidLength, got %v", err)
	}
}

func TestWire_FormatClosePayload_BufferTooSmall(t *testing.T) {
	buf := make([]byte, 3)
	_, err := ws.FormatClosePayload(buf, ws.StatusNormalClosure, "too long for 3 bytes")
	if !errors.Is(err, ws.ErrBufferTooSmall) {
		t.Fatalf("expected ErrBufferTooSmall, got %v", err)
	}
}

func TestWire_Parser_Reset(t *testing.T) {
	var parser ws.Parser
	// Feed partial
	var chunks []ws.FrameChunk
	_ = parser.Decode([]byte{0x81}, &chunks)
	parser.Reset()
	if parser.Err() != nil {
		t.Fatalf("expected nil Err after reset, got %v", parser.Err())
	}
	// Decode valid frame after reset
	err := parser.Decode([]byte("\x81\x04Test"), &chunks)
	if err != nil {
		t.Fatalf("unexpected error after reset: %v", err)
	}
	if len(chunks) != 1 || string(chunks[0].Payload) != "Test" {
		t.Fatalf("unexpected frame chunk: %+v", chunks)
	}
}
