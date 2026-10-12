// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package stream

import (
	"bytes"
	"errors"
	"io"
	"math/rand"
	"testing"
)

func TestReassemblyBuffer_InOrder(t *testing.T) {
	rb := NewReassemblyBuffer()

	err := rb.Push(0, []byte("Hello, "), false)
	if err != nil {
		t.Fatalf("push failed: %v", err)
	}

	err = rb.Push(7, []byte("world!"), true)
	if err != nil {
		t.Fatalf("push failed: %v", err)
	}

	buf := make([]byte, 32)
	n, err := rb.Read(buf)
	if err != io.EOF {
		t.Fatalf("expected io.EOF, got %v", err)
	}
	if string(buf[:n]) != "Hello, world!" {
		t.Fatalf("unexpected content: %q", string(buf[:n]))
	}
}

func TestReassemblyBuffer_ReverseOrder(t *testing.T) {
	rb := NewReassemblyBuffer()

	// Full message is 0..100
	fullMsg := make([]byte, 100)
	for i := range fullMsg {
		fullMsg[i] = byte(i)
	}

	// Deliver in 5 chunks in reverse order: [80..100], [60..80], [40..60], [20..40], [0..20]
	chunks := []struct {
		offset uint64
		data   []byte
		fin    bool
	}{
		{80, fullMsg[80:100], true},
		{60, fullMsg[60:80], false},
		{40, fullMsg[40:60], false},
		{20, fullMsg[20:40], false},
		{0, fullMsg[0:20], false},
	}

	for i, c := range chunks {
		err := rb.Push(c.offset, c.data, c.fin)
		if err != nil {
			t.Fatalf("chunk %d push failed: %v", i, err)
		}

		if i < len(chunks)-1 {
			// Prior to delivering chunk [0..20], available contiguous bytes from offset 0 must be 0
			if rb.AvailableContiguous() != 0 {
				t.Fatalf("expected 0 contiguous bytes before offset 0 arrived, got %d", rb.AvailableContiguous())
			}
		}
	}

	if rb.AvailableContiguous() != 100 {
		t.Fatalf("expected 100 contiguous bytes, got %d", rb.AvailableContiguous())
	}

	result := make([]byte, 100)
	n, err := rb.Read(result)
	if err != io.EOF {
		t.Fatalf("expected io.EOF, got %v", err)
	}
	if n != 100 || !bytes.Equal(result, fullMsg) {
		t.Fatalf("reassembled bytes mismatch in reverse order test")
	}
}

func TestReassemblyBuffer_RandomOrderWithOverlaps(t *testing.T) {
	rb := NewReassemblyBuffer()

	dataSize := 1024
	fullData := make([]byte, dataSize)
	for i := range fullData {
		fullData[i] = byte(i % 256)
	}

	type frame struct {
		offset uint64
		data   []byte
		fin    bool
	}

	var frames []frame
	chunkSize := 64
	for off := 0; off < dataSize; off += chunkSize {
		end := min(off+chunkSize+16, dataSize) // add 16 bytes deliberate overlap
		fin := (end == dataSize)
		frames = append(frames, frame{
			offset: uint64(off),
			data:   fullData[off:end],
			fin:    fin,
		})
	}

	// Also add some duplicate frames
	frames = append(frames, frames[0], frames[len(frames)/2])

	// Shuffle frames pseudo-randomly
	rng := rand.New(rand.NewSource(42))
	rng.Shuffle(len(frames), func(i, j int) {
		frames[i], frames[j] = frames[j], frames[i]
	})

	for i, f := range frames {
		err := rb.Push(f.offset, f.data, f.fin)
		if err != nil {
			t.Fatalf("frame %d (offset %d) failed push: %v", i, f.offset, err)
		}
	}

	readBuf := make([]byte, dataSize+100)
	var totalRead int
	for {
		chunk := make([]byte, 128)
		n, err := rb.Read(chunk)
		if n > 0 {
			copy(readBuf[totalRead:], chunk[:n])
			totalRead += n
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("read failed: %v", err)
		}
	}

	if totalRead != dataSize {
		t.Fatalf("expected totalRead %d, got %d", dataSize, totalRead)
	}
	if !bytes.Equal(readBuf[:totalRead], fullData) {
		t.Fatalf("random order with overlap reassembly mismatch")
	}
}

func TestReassemblyBuffer_FinalSizeErrors(t *testing.T) {
	// 1. Data beyond final size
	rb := NewReassemblyBuffer()
	err := rb.Push(0, []byte("12345"), true) // Final size = 5
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	err = rb.Push(4, []byte("extra"), false) // offset 4 + len 5 = 9 > 5
	if err == nil {
		t.Fatal("expected FinalSizeError for data beyond final size")
	}
	var tErr *TransportError
	if !errors.As(err, &tErr) || tErr.ErrorCode != FinalSizeError {
		t.Fatalf("expected FinalSizeError, got: %v", err)
	}

	// 2. Conflicting final sizes
	rb2 := NewReassemblyBuffer()
	_ = rb2.Push(0, []byte("abc"), true)    // Final size = 3
	err = rb2.Push(0, []byte("abcd"), true) // Final size = 4
	if err == nil {
		t.Fatal("expected FinalSizeError for conflicting final sizes")
	}
	if !errors.As(err, &tErr) || tErr.ErrorCode != FinalSizeError {
		t.Fatalf("expected FinalSizeError, got: %v", err)
	}

	// 3. Final size less than highest received offset
	rb3 := NewReassemblyBuffer()
	_ = rb3.Push(10, []byte("data"), false)  // Highest offset = 14
	err = rb3.Push(0, []byte("short"), true) // Final size = 5 < 14
	if err == nil {
		t.Fatal("expected FinalSizeError for final size less than highest offset")
	}
	if !errors.As(err, &tErr) || tErr.ErrorCode != FinalSizeError {
		t.Fatalf("expected FinalSizeError, got: %v", err)
	}
}

func TestReassemblyBuffer_Peek(t *testing.T) {
	rb := NewReassemblyBuffer()
	_ = rb.Push(0, []byte("peek-me-first"), true)

	peekBuf := make([]byte, 4)
	n, err := rb.Peek(peekBuf)
	if err != nil && err != io.EOF {
		t.Fatalf("peek error: %v", err)
	}
	if n != 4 || string(peekBuf) != "peek" {
		t.Fatalf("peek mismatch: got %q", string(peekBuf))
	}

	// Ensure read offset didn't advance
	if rb.ReadOffset() != 0 {
		t.Fatalf("read offset advanced during peek: %d", rb.ReadOffset())
	}

	// Second peek receives identical data
	n, _ = rb.Peek(peekBuf)
	if n != 4 || string(peekBuf) != "peek" {
		t.Fatalf("second peek mismatch")
	}

	// Now Read advances read offset
	readBuf := make([]byte, 4)
	n, _ = rb.Read(readBuf)
	if n != 4 || string(readBuf) != "peek" {
		t.Fatalf("read mismatch")
	}
	if rb.ReadOffset() != 4 {
		t.Fatalf("read offset should be 4, got %d", rb.ReadOffset())
	}
}
