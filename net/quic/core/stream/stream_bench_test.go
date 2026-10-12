// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package stream

import (
	"context"
	"testing"
)

func BenchmarkReassemblyBuffer_PushInOrder(b *testing.B) {
	rb := NewReassemblyBuffer()
	payload := make([]byte, 1024)
	b.SetBytes(1024)
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = rb.Push(rb.HighestOffset(), payload, false)
	}
}

func BenchmarkReassemblyBuffer_Read(b *testing.B) {
	rb := NewReassemblyBuffer()
	payload := make([]byte, 1024)
	buf := make([]byte, 1024)
	b.SetBytes(1024)
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = rb.Push(rb.ReadOffset(), payload, false)
		_, _ = rb.Read(buf)
	}
}

func BenchmarkReassemblyBuffer_Peek(b *testing.B) {
	rb := NewReassemblyBuffer()
	payload := make([]byte, 4096)
	_ = rb.Push(0, payload, false)
	buf := make([]byte, 1024)
	b.SetBytes(1024)
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_, _ = rb.Peek(buf)
	}
}

func BenchmarkStream_WriteFastPath(b *testing.B) {
	cfc := NewConnectionFlowController(500*1024*1024, 500*1024*1024, 500*1024*1024, nil, nil)
	sfc := NewStreamFlowController(0, cfc, 500*1024*1024, 500*1024*1024, 500*1024*1024, nil, nil)
	ss := NewSendStream(context.Background(), 0, nil, sfc)

	payload := make([]byte, 1024)
	b.SetBytes(1024)
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = ss.TryWriteAll(payload)
	}
}

func BenchmarkReassemblyBuffer_OutOfOrder(b *testing.B) {
	rb := NewReassemblyBuffer()
	payload := make([]byte, 1024)
	buf := make([]byte, 2048)
	b.SetBytes(2048)
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		// Simulate out-of-order packet arrival: chunk 1 then chunk 0
		_ = rb.Push(1024, payload, false)
		_ = rb.Push(0, payload, false)
		_, _ = rb.Read(buf)
		rb.Reset()
	}
}
