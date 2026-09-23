// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package overlay

import (
	"bytes"
	"testing"
)

// LegacyDataFrame simulates the traditional struct-based approach.
type LegacyDataFrame struct {
	Length   uint32
	Type     uint8
	Flags    uint8
	StreamID uint32
	Payload  []byte
}

func DecodeLegacy(b []byte) *LegacyDataFrame {
	if len(b) < 9 {
		return nil
	}

	length := uint32(b[0])<<16 | uint32(b[1])<<8 | uint32(b[2])
	if len(b) < int(9+length) {
		return nil
	}

	f := &LegacyDataFrame{
		Length:   length,
		Type:     b[3],
		Flags:    b[4],
		StreamID: uint32(b[5]&0x7F)<<24 | uint32(b[6])<<16 | uint32(b[7])<<8 | uint32(b[8]),
		Payload:  b[9 : 9+length],
	}

	return f
}

func BenchmarkLegacyStructDecode(b *testing.B) {
	// A 9-byte header + 4 byte payload
	raw := []byte{
		0x00, 0x00, 0x04, // Length = 4
		0x00,                   // Type = DATA
		0x01,                   // Flags = END_STREAM
		0x00, 0x00, 0x00, 0x01, // StreamID = 1
		0x11, 0x22, 0x33, 0x44, // Payload
	}

	b.ReportAllocs()
	b.ResetTimer()

	var (
		id         uint32
		flags      uint8
		payloadLen int
	)

	for i := 0; i < b.N; i++ {
		frame := DecodeLegacy(raw)
		id = frame.StreamID
		flags = frame.Flags
		payloadLen = len(frame.Payload)
	}

	_ = id
	_ = flags
	_ = payloadLen
}

func BenchmarkInSituOverlay(b *testing.B) {
	raw := []byte{
		0x00, 0x00, 0x04, // Length = 4
		0x00,                   // Type = DATA
		0x01,                   // Flags = END_STREAM
		0x00, 0x00, 0x00, 0x01, // StreamID = 1
		0x11, 0x22, 0x33, 0x44, // Payload
	}

	b.ReportAllocs()
	b.ResetTimer()

	var (
		id         uint32
		flags      uint8
		payloadLen int
	)

	for i := 0; i < b.N; i++ {
		frame := Frame(raw)
		if !frame.IsValid() {
			continue
		}

		id = frame.StreamID()
		flags = frame.Flags()
		payloadLen = len(frame.Payload())
	}

	_ = id
	_ = flags
	_ = payloadLen
}

func TestInSituOverlay_ValidFrames(t *testing.T) {
	// DATA frame: 4 bytes payload, type 0x00, flags 0x01 (END_STREAM), stream 1 (with reserved bit set)
	raw := []byte{
		0x00, 0x00, 0x04,
		0x00,
		0x01,
		0x80, 0x00, 0x00, 0x01, // 0x80 reserved bit should be masked out
		0x11, 0x22, 0x33, 0x44,
	}

	frame := Frame(raw)
	if !frame.IsValid() {
		t.Fatalf("expected frame to be valid")
	}

	if got, want := frame.Length(), uint32(4); got != want {
		t.Errorf("Length() = %d, want %d", got, want)
	}

	if got, want := frame.Type(), uint8(0x00); got != want {
		t.Errorf("Type() = %d, want %d", got, want)
	}

	if got, want := frame.Flags(), uint8(0x01); got != want {
		t.Errorf("Flags() = %d, want %d", got, want)
	}

	if got, want := frame.StreamID(), uint32(1); got != want {
		t.Errorf("StreamID() = %d, want %d", got, want)
	}

	expectedPayload := []byte{0x11, 0x22, 0x33, 0x44}
	if !bytes.Equal(frame.Payload(), expectedPayload) {
		t.Errorf("Payload() = %x, want %x", frame.Payload(), expectedPayload)
	}

	df := DataFrame(frame)
	if !df.IsEndStream() {
		t.Errorf("DataFrame.IsEndStream() = false, want true")
	}

	if df.IsPadded() {
		t.Errorf("DataFrame.IsPadded() = true, want false")
	}

	if df.PadLength() != 0 {
		t.Errorf("DataFrame.PadLength() = %d, want 0", df.PadLength())
	}

	if !bytes.Equal(df.Data(), expectedPayload) {
		t.Errorf("DataFrame.Data() = %x, want %x", df.Data(), expectedPayload)
	}

	// HEADERS frame: 3 bytes payload, type 0x01, flags 0x05 (END_STREAM | END_HEADERS), stream 5
	rawHeaders := []byte{
		0x00, 0x00, 0x03,
		0x01,
		0x05,
		0x00, 0x00, 0x00, 0x05,
		0x82, 0x86, 0x84,
	}

	hf := HeadersFrame(Frame(rawHeaders))
	if !Frame(hf).IsValid() {
		t.Fatalf("expected headers frame to be valid")
	}

	if !hf.IsEndStream() {
		t.Errorf("HeadersFrame.IsEndStream() = false, want true")
	}

	if !hf.IsEndHeaders() {
		t.Errorf("HeadersFrame.IsEndHeaders() = false, want true")
	}

	if !bytes.Equal(hf.HeaderBlockFragment(), []byte{0x82, 0x86, 0x84}) {
		t.Errorf("HeadersFrame.HeaderBlockFragment() = %x, want [82 86 84]", hf.HeaderBlockFragment())
	}
}

func TestInSituOverlay_TruncatedFrames(t *testing.T) {
	// Sub-9 byte slices
	for length := 0; length < 9; length++ {
		f := Frame(make([]byte, length))
		if f.IsValid() {
			t.Errorf("expected slice of length %d to be invalid", length)
		}
	}

	// Header indicates length 5, but slice only has 10 bytes (header 9 + 1 payload byte)
	incomplete := []byte{
		0x00, 0x00, 0x05,
		0x00,
		0x00,
		0x00, 0x00, 0x00, 0x01,
		0xAA,
	}

	f := Frame(incomplete)
	if f.IsValid() {
		t.Errorf("expected incomplete frame to be invalid")
	}
}

func TestInSituOverlay_PaddedDataFrame(t *testing.T) {
	// Padded frame: PadLength=2, Data=[0x01, 0x02], Padding=[0x00, 0x00]
	// Total payload length = 1 (pad length byte) + 2 (data) + 2 (padding) = 5
	raw := []byte{
		0x00, 0x00, 0x05,
		0x00,
		0x08, // PADDED flag
		0x00, 0x00, 0x00, 0x01,
		0x02,       // PadLength = 2
		0x01, 0x02, // Application Data
		0x00, 0x00, // Padding bytes
	}

	df := DataFrame(Frame(raw))
	if !Frame(df).IsValid() {
		t.Fatalf("expected padded frame to be valid")
	}

	if !df.IsPadded() {
		t.Errorf("IsPadded() = false, want true")
	}

	if got := df.PadLength(); got != 2 {
		t.Errorf("PadLength() = %d, want 2", got)
	}

	expectedData := []byte{0x01, 0x02}
	if !bytes.Equal(df.Data(), expectedData) {
		t.Errorf("Data() = %x, want %x", df.Data(), expectedData)
	}

	// Padded frame with zero padding: PadLength=0, Data=[0x99], total payload = 2
	zeroPadRaw := []byte{
		0x00, 0x00, 0x02,
		0x00,
		0x08,
		0x00, 0x00, 0x00, 0x01,
		0x00, // PadLength = 0
		0x99, // Application Data
	}

	zeroDf := DataFrame(Frame(zeroPadRaw))
	if got := zeroDf.PadLength(); got != 0 {
		t.Errorf("PadLength() = %d, want 0", got)
	}

	if !bytes.Equal(zeroDf.Data(), []byte{0x99}) {
		t.Errorf("Data() = %x, want [99]", zeroDf.Data())
	}

	// Malformed padding: payload has 3 bytes, but padLength is 5 (padLen exceeds payload)
	malformedRaw := []byte{
		0x00, 0x00, 0x03,
		0x00,
		0x08,
		0x00, 0x00, 0x00, 0x01,
		0x05, // PadLength = 5 > 3
		0x01, 0x02,
	}

	malformedDf := DataFrame(Frame(malformedRaw))
	if malformedDf.Data() != nil {
		t.Errorf("Data() for malformed padding = %v, want nil", malformedDf.Data())
	}
}
