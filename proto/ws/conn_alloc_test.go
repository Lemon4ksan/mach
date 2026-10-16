// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ws

import (
	"io"
	"testing"
)

func BenchmarkMaskBytes(b *testing.B) {
	data := make([]byte, 4096)
	mask := [4]byte{0x12, 0x34, 0x56, 0x78}
	b.SetBytes(int64(len(data)))
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		MaskBytes(data, mask)
	}
}

func BenchmarkAppendHeader(b *testing.B) {
	h := Header{
		Fin:     true,
		Opcode:  OpBinary,
		Masked:  true,
		MaskKey: [4]byte{1, 2, 3, 4},
		Length:  1024,
	}
	var dst [MaxHeaderSize]byte
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = AppendHeader(dst[:0], h)
	}
}

func BenchmarkReadHeader(b *testing.B) {
	h := Header{
		Fin:     true,
		Opcode:  OpText,
		Masked:  true,
		MaskKey: [4]byte{1, 2, 3, 4},
		Length:  512,
	}
	rawHeader := AppendHeader(nil, h)
	r := &repeatReader{data: rawHeader}
	var buf [MaxHeaderSize]byte

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := ReadHeader(r, buf[:])
		if err != nil {
			b.Fatal(err)
		}
		r.reset()
	}
}

func BenchmarkServerWriteMessage(b *testing.B) {
	sink := io.Discard
	conn := NewServerConn(&nopReadWriter{Writer: sink})
	payload := make([]byte, 1024)
	b.SetBytes(int64(len(payload)))
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = conn.WriteMessage(OpBinary, payload)
	}
}

func BenchmarkClientWriteMessage(b *testing.B) {
	sink := io.Discard
	conn := NewClientConn(&nopReadWriter{Writer: sink})
	payload := make([]byte, 1024)
	b.SetBytes(int64(len(payload)))
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = conn.WriteMessage(OpBinary, payload)
	}
}

func BenchmarkServerReadFrame(b *testing.B) {
	h := Header{
		Fin:     true,
		Opcode:  OpBinary,
		Masked:  true,
		MaskKey: [4]byte{1, 2, 3, 4},
		Length:  1024,
	}
	payload := make([]byte, 1024)
	wire := AppendFrame(nil, h, payload)

	r := &repeatReader{data: wire}
	conn := NewServerConn(r)
	var buf [2048]byte

	b.SetBytes(int64(len(payload)))
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _, err := conn.ReadFrame(buf[:])
		if err != nil {
			b.Fatal(err)
		}
		r.reset()
	}
}

func BenchmarkServerReadMessageBuf(b *testing.B) {
	h := Header{
		Fin:     true,
		Opcode:  OpBinary,
		Masked:  true,
		MaskKey: [4]byte{1, 2, 3, 4},
		Length:  1024,
	}
	payload := make([]byte, 1024)
	wire := AppendFrame(nil, h, payload)

	r := &repeatReader{data: wire}
	conn := NewServerConn(r)

	b.SetBytes(int64(len(payload)))
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _, err := conn.ReadMessageBuf()
		if err != nil {
			b.Fatal(err)
		}
		r.reset()
	}
}

func TestAlloc_ZeroAllocHotPaths(t *testing.T) {
	// 1. MaskBytes must have 0 allocs
	buf := make([]byte, 1024)
	mask := [4]byte{0xAA, 0xBB, 0xCC, 0xDD}
	allocs := testing.AllocsPerRun(1000, func() {
		MaskBytes(buf, mask)
	})
	if allocs != 0 {
		t.Fatalf("MaskBytes allocated %v times, want 0", allocs)
	}

	// 2. AppendHeader to preallocated slice must have 0 allocs
	var hdrBuf [MaxHeaderSize]byte
	h := Header{
		Fin:     true,
		Opcode:  OpBinary,
		Masked:  true,
		MaskKey: mask,
		Length:  2048,
	}
	allocs = testing.AllocsPerRun(1000, func() {
		_ = AppendHeader(hdrBuf[:0], h)
	})
	if allocs != 0 {
		t.Fatalf("AppendHeader allocated %v times, want 0", allocs)
	}

	// 3. ReadHeader with scratch buffer must have 0 allocs
	rawHdr := AppendHeader(nil, h)
	r := &repeatReader{data: rawHdr}
	var readBuf [MaxHeaderSize]byte
	allocs = testing.AllocsPerRun(1000, func() {
		_, _ = ReadHeader(r, readBuf[:])
		r.reset()
	})
	if allocs != 0 {
		t.Fatalf("ReadHeader allocated %v times, want 0", allocs)
	}

	// 4. Server WriteMessage with preallocated frame buffer must have 0 allocs
	sink := io.Discard
	serverConn := NewServerConn(&nopReadWriter{Writer: sink})
	data := []byte("Benchmark zero-alloc payload message")
	allocs = testing.AllocsPerRun(1000, func() {
		_ = serverConn.WriteMessage(OpText, data)
	})
	if allocs != 0 {
		t.Fatalf("Server WriteMessage allocated %v times, want 0", allocs)
	}

	// 5. ReadFrame with preallocated buffer must have 0 allocs
	frameWire := AppendFrame(nil, h, data)
	rFrame := &repeatReader{data: frameWire}
	clientConn := NewClientConn(&nopReadWriter{Writer: io.Discard})
	clientConn.rw = rFrame
	var frameBuf [512]byte
	allocs = testing.AllocsPerRun(1000, func() {
		_, _, _ = clientConn.ReadFrame(frameBuf[:])
		rFrame.reset()
	})
	if allocs != 0 {
		t.Fatalf("ReadFrame allocated %v times, want 0", allocs)
	}

	// 6. ReadMessageBuf must have 0 allocs
	rMsg := &repeatReader{data: frameWire}
	msgConn := NewClientConn(&nopReadWriter{Writer: io.Discard})
	msgConn.rw = rMsg
	allocs = testing.AllocsPerRun(1000, func() {
		_, _, _ = msgConn.ReadMessageBuf()
		rMsg.reset()
	})
	if allocs != 0 {
		t.Fatalf("ReadMessageBuf allocated %v times, want 0", allocs)
	}
}

type nopReadWriter struct {
	io.Writer
}

func (n *nopReadWriter) Read(p []byte) (int, error) {
	return 0, io.EOF
}

type repeatReader struct {
	data []byte
	off  int
}

func (r *repeatReader) Read(p []byte) (int, error) {
	n := copy(p, r.data[r.off:])
	r.off += n
	return n, nil
}

func (r *repeatReader) reset() {
	r.off = 0
}

func (r *repeatReader) Write(p []byte) (int, error) {
	return len(p), nil
}
