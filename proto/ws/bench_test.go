// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ws_test

import (
	"testing"

	"github.com/lemon4ksan/mach/proto/ws"
)

var benchKey = ws.MaskKey{0x12, 0x34, 0x56, 0x78}

func benchmarkMaskSWAR(b *testing.B, size int) {
	buf := make([]byte, size)
	b.SetBytes(int64(size))
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		ws.Mask(buf, benchKey)
	}
}

func benchmarkMaskScalar(b *testing.B, size int) {
	buf := make([]byte, size)
	b.SetBytes(int64(size))
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		ws.MaskScalar(buf, benchKey, 0)
	}
}

func BenchmarkMask_SWAR_64B(b *testing.B)  { benchmarkMaskSWAR(b, 64) }
func BenchmarkMask_SWAR_1KB(b *testing.B)  { benchmarkMaskSWAR(b, 1024) }
func BenchmarkMask_SWAR_64KB(b *testing.B) { benchmarkMaskSWAR(b, 65536) }
func BenchmarkMask_SWAR_1MB(b *testing.B)  { benchmarkMaskSWAR(b, 1024*1024) }

func BenchmarkMask_Scalar_64B(b *testing.B)  { benchmarkMaskScalar(b, 64) }
func BenchmarkMask_Scalar_1KB(b *testing.B)  { benchmarkMaskScalar(b, 1024) }
func BenchmarkMask_Scalar_64KB(b *testing.B) { benchmarkMaskScalar(b, 65536) }
func BenchmarkMask_Scalar_1MB(b *testing.B)  { benchmarkMaskScalar(b, 1024*1024) }

func BenchmarkParseHeader_Small(b *testing.B) {
	data := []byte("\x81\x85\x12\x34\x56\x78Hello")
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _, _ = ws.ParseHeader(data)
	}
}

func BenchmarkParseHeader_Extended16(b *testing.B) {
	data := []byte("\x81\xFE\x04\x00\x12\x34\x56\x78")
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _, _ = ws.ParseHeader(data)
	}
}

func BenchmarkWriteHeader(b *testing.B) {
	h := ws.Header{
		Fin:        true,
		Opcode:     ws.OpcodeText,
		Masked:     true,
		MaskKey:    benchKey,
		PayloadLen: 1024,
	}
	buf := make([]byte, 14)
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = ws.WriteHeader(buf, h)
	}
}

func BenchmarkWriteFrame_1KB(b *testing.B) {
	h := ws.Header{
		Fin:        true,
		Opcode:     ws.OpcodeBinary,
		Masked:     true,
		MaskKey:    benchKey,
		PayloadLen: 1024,
	}
	payload := make([]byte, 1024)
	dst := make([]byte, 14+1024)
	b.SetBytes(1024)
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = ws.WriteFrame(dst, h, payload)
	}
}

func BenchmarkHandshake_ComputeAccept(b *testing.B) {
	const key = "dGhlIHNhbXBsZSBub25jZQ=="
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = ws.ComputeAccept(key)
	}
}

func BenchmarkHandshake_AppendAccept(b *testing.B) {
	const key = "dGhlIHNhbXBsZSBub25jZQ=="
	dst := make([]byte, 0, 64)
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = ws.AppendAccept(dst[:0], key)
	}
}

func BenchmarkHandshake_ValidateKey(b *testing.B) {
	const key = "dGhlIHNhbXBsZSBub25jZQ=="
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = ws.ValidateKey(key)
	}
}

func BenchmarkMask_SWAR_Unaligned_1KB(b *testing.B) {
	buf := make([]byte, 1024+7)
	unaligned := buf[3 : 3+1024]
	b.SetBytes(1024)
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		ws.Mask(unaligned, benchKey)
	}
}

func BenchmarkParser_Decode_10Frames(b *testing.B) {
	data := []byte(
		"\x81\x05Frame\x81\x05Frame\x81\x05Frame\x81\x05Frame\x81\x05Frame" +
			"\x81\x05Frame\x81\x05Frame\x81\x05Frame\x81\x05Frame\x81\x05Frame",
	)
	var parser ws.Parser
	chunks := make([]ws.FrameChunk, 0, 16)
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		chunks = chunks[:0]
		_ = parser.Decode(data, &chunks)
	}
}
