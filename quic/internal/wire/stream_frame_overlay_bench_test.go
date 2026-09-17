// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package wire

import (
	"math/rand"
	"testing"

	"github.com/lemon4ksan/mach/quic/internal/protocol"
)

func BenchmarkParseStreamFrame_Overlay(b *testing.B) {
	// Prepare frames
	var rawFrames [][]byte
	for i := range 10 {
		data := make([]byte, 200+i)
		rand.Read(data)
		sf := &StreamFrame{
			StreamID:       protocol.StreamID(1337 + i),
			Offset:         protocol.ByteCount(1e7 + i),
			Data:           data,
			DataLenPresent: true,
		}

		buf, _ := sf.Append(nil, protocol.Version1)
		// We only want the payload bytes (skip frame type byte for parsing)
		// Frame type is the first byte.
		rawFrames = append(rawFrames, buf)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		for _, raw := range rawFrames {
			typ := FrameType(raw[0])

			_, _, err := ParseStreamFrameOverlay(raw[1:], typ)
			if err != nil {
				b.Fatal(err)
			}
		}
	}
}

func BenchmarkParseStreamFrame_Classic(b *testing.B) {
	// Prepare frames
	var rawFrames [][]byte
	for i := range 10 {
		data := make([]byte, 200+i)
		rand.Read(data)
		sf := &StreamFrame{
			StreamID:       protocol.StreamID(1337 + i),
			Offset:         protocol.ByteCount(1e7 + i),
			Data:           data,
			DataLenPresent: true,
		}

		buf, _ := sf.Append(nil, protocol.Version1)
		rawFrames = append(rawFrames, buf)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		for _, raw := range rawFrames {
			typ := FrameType(raw[0])

			_, _, err := ParseStreamFrame(raw[1:], typ, protocol.Version1)
			if err != nil {
				b.Fatal(err)
			}
		}
	}
}
