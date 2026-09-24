// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package zerocopy

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestStreamReaderCreationAndReset(t *testing.T) {
	// Size < 4096 clamped to 4096
	sr := NewStreamReader(strings.NewReader("hello"), 100)
	if len(sr.buf) != 4096 {
		t.Fatalf("expected buffer size 4096, got %d", len(sr.buf))
	}

	// Size >= 4096 preserved
	sr2 := NewStreamReader(strings.NewReader("hello"), 8192)
	if len(sr2.buf) != 8192 {
		t.Fatalf("expected buffer size 8192, got %d", len(sr2.buf))
	}

	// Reset
	sr.Reset(strings.NewReader("reset content"))
	if sr.Buffered() != 0 {
		t.Fatalf("expected 0 buffered after reset, got %d", sr.Buffered())
	}
}

func TestStreamReaderPeekAndDiscard(t *testing.T) {
	content := "0123456789ABCDEF"
	sr := NewStreamReader(strings.NewReader(content), 4096)

	// Negative count
	if _, err := sr.Peek(-1); err == nil {
		t.Fatal("expected error on negative Peek count")
	}
	if _, err := sr.Discard(-1); err == nil {
		t.Fatal("expected error on negative Discard count")
	}

	// Discard 0
	n0, err := sr.Discard(0)
	if err != nil || n0 != 0 {
		t.Fatalf("Discard(0) failed: %d, %v", n0, err)
	}

	// Peek within content
	peeked, err := sr.Peek(10)
	if err != nil || string(peeked) != "0123456789" {
		t.Fatalf("Peek(10) = %q, %v", peeked, err)
	}
	if sr.Buffered() != len(content) {
		t.Fatalf("expected buffered %d, got %d", len(content), sr.Buffered())
	}

	// Discard 5 bytes
	nDiscard, err := sr.Discard(5)
	if err != nil || nDiscard != 5 {
		t.Fatalf("Discard(5) = %d, %v", nDiscard, err)
	}
	if sr.Buffered() != len(content)-5 {
		t.Fatalf("expected buffered %d, got %d", len(content)-5, sr.Buffered())
	}

	// Peek next 5 bytes
	peeked2, err := sr.Peek(5)
	if err != nil || string(peeked2) != "56789" {
		t.Fatalf("Peek(5) after discard = %q, %v", peeked2, err)
	}

	// Peek exceeding buffer size -> ErrBufferFull
	if _, err := sr.Peek(5000); !errors.Is(err, ErrBufferFull) {
		t.Fatalf("expected ErrBufferFull, got %v", err)
	}

	// Peek exceeding available input -> io.ErrUnexpectedEOF or EOF
	peekedExceed, err := sr.Peek(100)
	if !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		t.Fatalf("expected EOF error, got %v", err)
	}
	if string(peekedExceed) != "56789ABCDEF" {
		t.Fatalf("unexpected peeked bytes on EOF: %q", peekedExceed)
	}

	// Discard remaining + past EOF
	nPast, err := sr.Discard(100)
	if !errors.Is(err, io.EOF) || nPast != len("56789ABCDEF") {
		t.Fatalf("Discard past EOF = %d, %v", nPast, err)
	}

	// Discard when already at EOF
	nAtEOF, err := sr.Discard(5)
	if !errors.Is(err, io.EOF) || nAtEOF != 0 {
		t.Fatalf("Discard at EOF = %d, %v", nAtEOF, err)
	}
}

func TestStreamReaderReadSlice(t *testing.T) {
	data := "line1\nline2\r\nline3\nrest_without_delim"
	sr := NewStreamReader(strings.NewReader(data), 4096)

	// Read line 1
	slice1, err := sr.ReadSlice('\n')
	if err != nil || string(slice1) != "line1\n" {
		t.Fatalf("ReadSlice 1 = %q, %v", slice1, err)
	}

	// Read line 2
	slice2, err := sr.ReadSlice('\n')
	if err != nil || string(slice2) != "line2\r\n" {
		t.Fatalf("ReadSlice 2 = %q, %v", slice2, err)
	}

	// Read line 3
	slice3, err := sr.ReadSlice('\n')
	if err != nil || string(slice3) != "line3\n" {
		t.Fatalf("ReadSlice 3 = %q, %v", slice3, err)
	}

	// Read slice without delimiter until EOF
	slice4, err := sr.ReadSlice('\n')
	if !errors.Is(err, io.EOF) || string(slice4) != "rest_without_delim" {
		t.Fatalf("ReadSlice without delimiter = %q, %v", slice4, err)
	}
}

func TestStreamReaderReadAndReadByte(t *testing.T) {
	data := "abcdefghijklmnopqrstuvwxyz"
	sr := NewStreamReader(strings.NewReader(data), 4096)

	// Read 0 bytes
	n0, err := sr.Read(nil)
	if err != nil || n0 != 0 {
		t.Fatalf("Read(nil) = %d, %v", n0, err)
	}

	// ReadByte
	b1, err := sr.ReadByte()
	if err != nil || b1 != 'a' {
		t.Fatalf("ReadByte = %c, %v", b1, err)
	}

	// Small buffer read
	buf := make([]byte, 5)
	n, err := sr.Read(buf)
	if err != nil || n != 5 || string(buf) != "bcdef" {
		t.Fatalf("Read small = %q, %d, %v", buf, n, err)
	}

	// Large buffer read bypassing stream reader buffer
	largeBuf := make([]byte, 5000)
	srLarge := NewStreamReader(strings.NewReader(strings.Repeat("X", 5000)), 4096)
	nLarge, err := srLarge.Read(largeBuf)
	if err != nil || nLarge != 5000 {
		t.Fatalf("Read large bypass = %d, %v", nLarge, err)
	}

	// Read to EOF
	allRemaining, err := io.ReadAll(sr)
	if err != nil || string(allRemaining) != "ghijklmnopqrstuvwxyz" {
		t.Fatalf("ReadAll remaining failed: %q, %v", allRemaining, err)
	}

	// ReadByte after EOF
	_, err = sr.ReadByte()
	if !errors.Is(err, io.EOF) {
		t.Fatalf("expected EOF on ReadByte after stream exhausted, got %v", err)
	}

	// Read after EOF
	bufEOF := make([]byte, 10)
	nEOF, err := sr.Read(bufEOF)
	if !errors.Is(err, io.EOF) || nEOF != 0 {
		t.Fatalf("expected 0, EOF on Read after stream exhausted, got %d, %v", nEOF, err)
	}
}

func TestStreamReaderFillShift(t *testing.T) {
	// Exercise StreamReader.fill memory shifting when b.r > 0
	content := bytes.Repeat([]byte("A"), 3000)
	content = append(content, bytes.Repeat([]byte("B"), 3000)...)

	sr := NewStreamReader(bytes.NewReader(content), 4096)

	// Discard 2000 bytes (b.r becomes 2000)
	_, _ = sr.Discard(2000)

	// Now Peek 3000 bytes - triggers fill() which shifts memory from b.r:b.w to 0!
	peeked, err := sr.Peek(3000)
	if err != nil || len(peeked) != 3000 {
		t.Fatalf("Peek after discard memory shift failed: len=%d, %v", len(peeked), err)
	}
}
