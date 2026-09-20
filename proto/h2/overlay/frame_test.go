// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package overlay

import (
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
