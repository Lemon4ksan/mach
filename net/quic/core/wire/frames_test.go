// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package wire

import (
	"bytes"
	"io"
	"testing"
)

func TestFrameSerialization_Padding(t *testing.T) {
	buf := AppendPadding(nil, 50)
	if len(buf) != 50 {
		t.Fatalf("AppendPadding len: got %d, want 50", len(buf))
	}
	for i, b := range buf {
		if b != 0x00 {
			t.Fatalf("AppendPadding byte %d non-zero: 0x%02x", i, b)
		}
	}

	var f Frame
	n, err := ParseNextFrame(buf, &f)
	if err != nil {
		t.Fatalf("ParseNextFrame failed: %v", err)
	}
	if n != 50 {
		t.Fatalf("ParseNextFrame consumed: got %d, want 50", n)
	}
	if f.Type != FrameTypePadding || f.PaddingCount != 50 {
		t.Fatalf("Padding frame mismatch: type=%s, count=%d", f.Type, f.PaddingCount)
	}
}

func TestFrameSerialization_Ping(t *testing.T) {
	buf := AppendPing(nil)
	var f Frame
	n, err := ParseNextFrame(buf, &f)
	if err != nil {
		t.Fatalf("ParseNextFrame failed: %v", err)
	}
	if n != 1 || f.Type != FrameTypePing {
		t.Fatalf("PING mismatch: n=%d, type=%s", n, f.Type)
	}
}

func TestFrameSerialization_Ack(t *testing.T) {
	// Simple ACK
	a1 := AckFrame{
		LargestAcked:  1000,
		AckDelay:      25,
		FirstAckRange: 5,
	}
	buf := AppendAckFrame(nil, &a1)
	var f Frame
	n, err := ParseNextFrame(buf, &f)
	if err != nil {
		t.Fatalf("ParseNextFrame failed: %v", err)
	}
	if n != len(buf) || f.Type != FrameTypeAck {
		t.Fatalf("ACK mismatch: n=%d, type=%s", n, f.Type)
	}
	if f.Ack.LargestAcked != 1000 || f.Ack.AckDelay != 25 || f.Ack.FirstAckRange != 5 || f.Ack.RangeCount() != 0 ||
		f.Ack.HasECN {
		t.Fatalf("ACK field mismatch: %+v", f.Ack)
	}

	// ACK with ECN and multiple ranges (>8 ranges to test inline + extra spillover)
	a2 := AckFrame{
		LargestAcked:  50000,
		AckDelay:      100,
		FirstAckRange: 10,
		HasECN:        true,
		ECN: ECNCounts{
			ECT0:  11,
			ECT1:  22,
			ECNCE: 33,
		},
	}
	for i := 0; i < 12; i++ {
		a2.AppendRange(uint64(i*2+1), uint64(i*3+1))
	}
	buf = AppendAckFrame(nil, &a2)
	f.Reset()
	n, err = ParseNextFrame(buf, &f)
	if err != nil {
		t.Fatalf("ParseNextFrame failed: %v", err)
	}
	if n != len(buf) || f.Type != FrameTypeAckECN {
		t.Fatalf("ACK_ECN mismatch: n=%d, type=%s", n, f.Type)
	}
	if f.Ack.LargestAcked != a2.LargestAcked || f.Ack.AckDelay != a2.AckDelay ||
		f.Ack.FirstAckRange != a2.FirstAckRange {
		t.Fatalf("ACK header mismatch: %+v", f.Ack)
	}
	if f.Ack.RangeCount() != 12 {
		t.Fatalf("RangeCount mismatch: got %d, want 12", f.Ack.RangeCount())
	}
	for i := 0; i < 12; i++ {
		origR := a2.Range(i)
		decR := f.Ack.Range(i)
		if origR != decR {
			t.Fatalf("Range %d mismatch: got %+v, want %+v", i, decR, origR)
		}
	}
	if !f.Ack.HasECN || f.Ack.ECN != a2.ECN {
		t.Fatalf("ECN mismatch: got %+v, want %+v", f.Ack.ECN, a2.ECN)
	}
}

func TestFrameSerialization_ResetStream(t *testing.T) {
	orig := ResetStreamFrame{
		StreamID:  42,
		ErrorCode: 0x101,
		FinalSize: 99999,
	}
	buf := AppendResetStreamFrame(nil, &orig)
	var f Frame
	n, err := ParseNextFrame(buf, &f)
	if err != nil {
		t.Fatalf("ParseNextFrame failed: %v", err)
	}
	if n != len(buf) || f.Type != FrameTypeResetStream || f.ResetStream != orig {
		t.Fatalf("RESET_STREAM mismatch: %+v", f.ResetStream)
	}
}

func TestFrameSerialization_StopSending(t *testing.T) {
	orig := StopSendingFrame{
		StreamID:  84,
		ErrorCode: 0x202,
	}
	buf := AppendStopSendingFrame(nil, &orig)
	var f Frame
	n, err := ParseNextFrame(buf, &f)
	if err != nil {
		t.Fatalf("ParseNextFrame failed: %v", err)
	}
	if n != len(buf) || f.Type != FrameTypeStopSending || f.StopSending != orig {
		t.Fatalf("STOP_SENDING mismatch: %+v", f.StopSending)
	}
}

func TestFrameSerialization_Crypto(t *testing.T) {
	data := []byte("handshake-tls13-crypto-payload-data")
	orig := CryptoFrame{
		Offset: 1024,
		Data:   data,
	}
	buf := AppendCryptoFrame(nil, &orig)
	var f Frame
	n, err := ParseNextFrame(buf, &f)
	if err != nil {
		t.Fatalf("ParseNextFrame failed: %v", err)
	}
	if n != len(buf) || f.Type != FrameTypeCrypto {
		t.Fatalf("CRYPTO mismatch: n=%d, type=%s", n, f.Type)
	}
	if f.Crypto.Offset != orig.Offset || !bytes.Equal(f.Crypto.Data, orig.Data) {
		t.Fatalf("CRYPTO payload mismatch")
	}
}

func TestFrameSerialization_NewToken(t *testing.T) {
	token := []byte("token-value-1234567890")
	orig := NewTokenFrame{Token: token}
	buf := AppendNewTokenFrame(nil, &orig)
	var f Frame
	n, err := ParseNextFrame(buf, &f)
	if err != nil {
		t.Fatalf("ParseNextFrame failed: %v", err)
	}
	if n != len(buf) || f.Type != FrameTypeNewToken || !bytes.Equal(f.NewToken.Token, token) {
		t.Fatalf("NEW_TOKEN mismatch")
	}
}

func TestFrameSerialization_Stream(t *testing.T) {
	offsets := []uint64{0, 1, 500000}
	fins := []bool{false, true}
	hasLens := []bool{false, true}
	dataPayload := []byte("hello-stream-data-payload")

	for _, off := range offsets {
		for _, fin := range fins {
			for _, hasLen := range hasLens {
				orig := StreamFrame{
					StreamID: 100,
					Offset:   off,
					Fin:      fin,
					HasLen:   hasLen,
					Data:     dataPayload,
				}
				buf := AppendStreamFrame(nil, &orig)
				var f Frame
				n, err := ParseNextFrame(buf, &f)
				if err != nil {
					t.Fatalf("ParseNextFrame failed: %v", err)
				}
				if n != len(buf) {
					t.Fatalf("STREAM length consumed mismatch: got %d, want %d", n, len(buf))
				}
				s := f.Stream
				if s.StreamID != orig.StreamID || s.Offset != orig.Offset || s.Fin != orig.Fin ||
					!bytes.Equal(s.Data, orig.Data) {
					t.Fatalf(
						"STREAM content mismatch (off=%d, fin=%v, hasLen=%v): got %+v, want %+v",
						off,
						fin,
						hasLen,
						s,
						orig,
					)
				}
			}
		}
	}
}

func TestFrameSerialization_MaxData(t *testing.T) {
	orig := MaxDataFrame{MaximumData: 1048576}
	buf := AppendMaxDataFrame(nil, &orig)
	var f Frame
	n, err := ParseNextFrame(buf, &f)
	if err != nil {
		t.Fatalf("ParseNextFrame failed: %v", err)
	}
	if n != len(buf) || f.Type != FrameTypeMaxData || f.MaxData != orig {
		t.Fatalf("MAX_DATA mismatch")
	}
}

func TestFrameSerialization_MaxStreamData(t *testing.T) {
	orig := MaxStreamDataFrame{StreamID: 4, MaximumStreamData: 65536}
	buf := AppendMaxStreamDataFrame(nil, &orig)
	var f Frame
	n, err := ParseNextFrame(buf, &f)
	if err != nil {
		t.Fatalf("ParseNextFrame failed: %v", err)
	}
	if n != len(buf) || f.Type != FrameTypeMaxStreamData || f.MaxStreamData != orig {
		t.Fatalf("MAX_STREAM_DATA mismatch")
	}
}

func TestFrameSerialization_MaxStreams(t *testing.T) {
	for _, uni := range []bool{false, true} {
		orig := MaxStreamsFrame{MaxStreams: 100, Unidirectional: uni}
		buf := AppendMaxStreamsFrame(nil, &orig)
		var f Frame
		n, err := ParseNextFrame(buf, &f)
		if err != nil {
			t.Fatalf("ParseNextFrame failed: %v", err)
		}
		if n != len(buf) || f.MaxStreams != orig {
			t.Fatalf("MAX_STREAMS mismatch (uni=%v)", uni)
		}
	}
}

func TestFrameSerialization_DataBlocked(t *testing.T) {
	orig := DataBlockedFrame{MaximumData: 1048576}
	buf := AppendDataBlockedFrame(nil, &orig)
	var f Frame
	n, err := ParseNextFrame(buf, &f)
	if err != nil {
		t.Fatalf("ParseNextFrame failed: %v", err)
	}
	if n != len(buf) || f.Type != FrameTypeDataBlocked || f.DataBlocked != orig {
		t.Fatalf("DATA_BLOCKED mismatch")
	}
}

func TestFrameSerialization_StreamDataBlocked(t *testing.T) {
	orig := StreamDataBlockedFrame{StreamID: 8, MaximumStreamData: 2048}
	buf := AppendStreamDataBlockedFrame(nil, &orig)
	var f Frame
	n, err := ParseNextFrame(buf, &f)
	if err != nil {
		t.Fatalf("ParseNextFrame failed: %v", err)
	}
	if n != len(buf) || f.Type != FrameTypeStreamDataBlocked || f.StreamDataBlocked != orig {
		t.Fatalf("STREAM_DATA_BLOCKED mismatch")
	}
}

func TestFrameSerialization_StreamsBlocked(t *testing.T) {
	for _, uni := range []bool{false, true} {
		orig := StreamsBlockedFrame{StreamLimit: 50, Unidirectional: uni}
		buf := AppendStreamsBlockedFrame(nil, &orig)
		var f Frame
		n, err := ParseNextFrame(buf, &f)
		if err != nil {
			t.Fatalf("ParseNextFrame failed: %v", err)
		}
		if n != len(buf) || f.StreamsBlocked != orig {
			t.Fatalf("STREAMS_BLOCKED mismatch")
		}
	}
}

func TestFrameSerialization_NewConnectionID(t *testing.T) {
	orig := NewConnectionIDFrame{
		SequenceNumber: 1,
		RetirePriorTo:  0,
		ConnID:         []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08},
		StatelessResetToken: [16]byte{
			0xaa,
			0xbb,
			0xcc,
			0xdd,
			0xee,
			0xff,
			0x11,
			0x22,
			0x33,
			0x44,
			0x55,
			0x65,
			0x77,
			0x88,
			0x99,
			0x00,
		},
	}
	buf := AppendNewConnectionIDFrame(nil, &orig)
	var f Frame
	n, err := ParseNextFrame(buf, &f)
	if err != nil {
		t.Fatalf("ParseNextFrame failed: %v", err)
	}
	if n != len(buf) || f.Type != FrameTypeNewConnectionID {
		t.Fatalf("NEW_CONNECTION_ID mismatch")
	}
	nci := f.NewConnectionID
	if nci.SequenceNumber != orig.SequenceNumber || nci.RetirePriorTo != orig.RetirePriorTo ||
		!bytes.Equal(nci.ConnID, orig.ConnID) || nci.StatelessResetToken != orig.StatelessResetToken {
		t.Fatalf("NEW_CONNECTION_ID field mismatch")
	}
}

func TestFrameSerialization_RetireConnectionID(t *testing.T) {
	orig := RetireConnectionIDFrame{SequenceNumber: 3}
	buf := AppendRetireConnectionIDFrame(nil, &orig)
	var f Frame
	n, err := ParseNextFrame(buf, &f)
	if err != nil {
		t.Fatalf("ParseNextFrame failed: %v", err)
	}
	if n != len(buf) || f.Type != FrameTypeRetireConnectionID || f.RetireConnectionID != orig {
		t.Fatalf("RETIRE_CONNECTION_ID mismatch")
	}
}

func TestFrameSerialization_PathChallengeAndResponse(t *testing.T) {
	challenge := PathChallengeFrame{Data: [8]byte{1, 2, 3, 4, 5, 6, 7, 8}}
	buf := AppendPathChallengeFrame(nil, &challenge)
	var f Frame
	n, err := ParseNextFrame(buf, &f)
	if err != nil {
		t.Fatalf("ParseNextFrame failed: %v", err)
	}
	if n != len(buf) || f.Type != FrameTypePathChallenge || f.PathChallenge != challenge {
		t.Fatalf("PATH_CHALLENGE mismatch")
	}

	response := PathResponseFrame{Data: [8]byte{8, 7, 6, 5, 4, 3, 2, 1}}
	buf = AppendPathResponseFrame(nil, &response)
	f.Reset()
	n, err = ParseNextFrame(buf, &f)
	if err != nil {
		t.Fatalf("ParseNextFrame failed: %v", err)
	}
	if n != len(buf) || f.Type != FrameTypePathResponse || f.PathResponse != response {
		t.Fatalf("PATH_RESPONSE mismatch")
	}
}

func TestFrameSerialization_ConnectionClose(t *testing.T) {
	// Transport close
	c1 := ConnectionCloseFrame{
		IsApplication: false,
		ErrorCode:     uint64(FlowControlError),
		FrameType:     uint64(FrameTypeStream),
		Reason:        []byte("flow control window exceeded"),
	}
	buf := AppendConnectionCloseFrame(nil, &c1)
	var f Frame
	n, err := ParseNextFrame(buf, &f)
	if err != nil {
		t.Fatalf("ParseNextFrame failed: %v", err)
	}
	if n != len(buf) || f.Type != FrameTypeConnectionCloseTransport {
		t.Fatalf("CONNECTION_CLOSE transport mismatch")
	}
	if f.ConnectionClose.IsApplication || f.ConnectionClose.ErrorCode != c1.ErrorCode ||
		f.ConnectionClose.FrameType != c1.FrameType || !bytes.Equal(f.ConnectionClose.Reason, c1.Reason) {
		t.Fatalf("CONNECTION_CLOSE fields mismatch: %+v", f.ConnectionClose)
	}

	// Application close
	c2 := ConnectionCloseFrame{
		IsApplication: true,
		ErrorCode:     0x404,
		Reason:        []byte("application terminated"),
	}
	buf = AppendConnectionCloseFrame(nil, &c2)
	f.Reset()
	n, err = ParseNextFrame(buf, &f)
	if err != nil {
		t.Fatalf("ParseNextFrame failed: %v", err)
	}
	if n != len(buf) || f.Type != FrameTypeConnectionCloseApp {
		t.Fatalf("CONNECTION_CLOSE app mismatch")
	}
	if !f.ConnectionClose.IsApplication || f.ConnectionClose.ErrorCode != c2.ErrorCode ||
		!bytes.Equal(f.ConnectionClose.Reason, c2.Reason) {
		t.Fatalf("CONNECTION_CLOSE app fields mismatch: %+v", f.ConnectionClose)
	}
}

func TestFrameSerialization_HandshakeDone(t *testing.T) {
	buf := AppendHandshakeDoneFrame(nil)
	var f Frame
	n, err := ParseNextFrame(buf, &f)
	if err != nil {
		t.Fatalf("ParseNextFrame failed: %v", err)
	}
	if n != 1 || f.Type != FrameTypeHandshakeDone {
		t.Fatalf("HANDSHAKE_DONE mismatch")
	}
}

func TestFrameSerialization_Datagram(t *testing.T) {
	data := []byte("unreliable datagram payload")
	// With length
	d1 := DatagramFrame{HasLen: true, Data: data}
	buf := AppendDatagramFrame(nil, &d1)
	var f Frame
	n, err := ParseNextFrame(buf, &f)
	if err != nil {
		t.Fatalf("ParseNextFrame failed: %v", err)
	}
	if n != len(buf) || f.Type != FrameTypeDatagramWithLen || !bytes.Equal(f.Datagram.Data, data) {
		t.Fatalf("DATAGRAM with len mismatch")
	}

	// Without length (extends to end)
	d2 := DatagramFrame{HasLen: false, Data: data}
	buf = AppendDatagramFrame(nil, &d2)
	f.Reset()
	n, err = ParseNextFrame(buf, &f)
	if err != nil {
		t.Fatalf("ParseNextFrame failed: %v", err)
	}
	if n != len(buf) || f.Type != FrameTypeDatagram || !bytes.Equal(f.Datagram.Data, data) {
		t.Fatalf("DATAGRAM without len mismatch")
	}
}

func TestMultiFrameSequenceAndIterator(t *testing.T) {
	var packetPayload []byte
	// Pack multiple frames: PING + ACK + STREAM(hasLen=true) + HANDSHAKE_DONE + PADDING(10)
	packetPayload = AppendPing(packetPayload)
	ack := AckFrame{LargestAcked: 5, AckDelay: 2, FirstAckRange: 1}
	packetPayload = AppendAckFrame(packetPayload, &ack)
	stream := StreamFrame{StreamID: 1, Offset: 0, Fin: true, HasLen: true, Data: []byte("final data")}
	packetPayload = AppendStreamFrame(packetPayload, &stream)
	packetPayload = AppendHandshakeDoneFrame(packetPayload)
	packetPayload = AppendPadding(packetPayload, 10)

	expectedTypes := []FrameType{
		FrameTypePing,
		FrameTypeAck,
		FrameTypeStream,
		FrameTypeHandshakeDone,
		FrameTypePadding,
	}

	var parsedTypes []FrameType
	for f, err := range Frames(packetPayload) {
		if err != nil {
			t.Fatalf("Frames iterator encountered error: %v", err)
		}
		if f.Type >= 0x08 && f.Type <= 0x0f {
			parsedTypes = append(parsedTypes, FrameTypeStream)
		} else {
			parsedTypes = append(parsedTypes, f.Type)
		}
	}

	if len(parsedTypes) != len(expectedTypes) {
		t.Fatalf("Frame count mismatch: got %d, want %d", len(parsedTypes), len(expectedTypes))
	}
	for i, typ := range parsedTypes {
		if typ != expectedTypes[i] {
			t.Fatalf("Frame %d type mismatch: got %s, want %s", i, typ, expectedTypes[i])
		}
	}
}

func TestFrameParser_ErrorCases(t *testing.T) {
	var f Frame
	// Empty buffer -> io.EOF
	if _, err := ParseNextFrame(nil, &f); err != io.EOF {
		t.Fatalf("Expected io.EOF, got %v", err)
	}

	// Truncated varint type
	if _, err := ParseNextFrame([]byte{0x80}, &f); err != ErrTruncatedFrame {
		t.Fatalf("Expected ErrTruncatedFrame, got %v", err)
	}

	// Truncated ACK
	if _, err := ParseNextFrame([]byte{0x02, 0x01}, &f); err != ErrTruncatedFrame {
		t.Fatalf("Expected ErrTruncatedFrame for truncated ACK, got %v", err)
	}

	// Truncated STREAM with length
	if _, err := ParseNextFrame([]byte{0x0a, 0x01, 0x05, 0xaa}, &f); err != ErrTruncatedFrame {
		t.Fatalf("Expected ErrTruncatedFrame for truncated STREAM, got %v", err)
	}

	// Invalid frame type
	if _, err := ParseNextFrame([]byte{0x2f}, &f); err != ErrInvalidFrameType {
		t.Fatalf("Expected ErrInvalidFrameType, got %v", err)
	}
}
