// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h1_test

import (
	"bufio"
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/lemon4ksan/foundation/silicon/bytesconv"

	"github.com/lemon4ksan/mach/proto/h1"
)

func TestChunkedRoundtrip(t *testing.T) {
	buf := bytesconv.AcquireByteBuffer()
	defer bytesconv.ReleaseByteBuffer(buf)

	cw := h1.NewChunkedWriter(buf)
	part1 := []byte("Hello, ")
	part2 := []byte("Chunked ")
	part3 := []byte("World!")

	_, _ = cw.Write(part1)
	_, _ = cw.Write(part2)
	_, _ = cw.Write(part3)
	_ = cw.Close()

	cr := h1.NewChunkedReader(bufio.NewReader(bytes.NewReader(buf.B)))
	decoded, err := io.ReadAll(cr)
	if err != nil {
		t.Fatalf("failed decoding chunked output: %v", err)
	}

	expected := "Hello, Chunked World!"
	if string(decoded) != expected {
		t.Fatalf("expected %q, got %q", expected, string(decoded))
	}
}

func TestChunkedReader_RFC9112(t *testing.T) {
	// Leading whitespace rejection
	t.Run("LeadingWhitespaceRejection", func(t *testing.T) {
		raw := " 5\r\nhello\r\n0\r\n\r\n"
		cr := h1.NewChunkedReader(bufio.NewReader(strings.NewReader(raw)))
		_, err := io.ReadAll(cr)
		if err == nil {
			t.Fatal("expected error on leading whitespace")
		}
	})

	// Chunk extensions and BWS
	t.Run("ChunkExtensionsAndBWS", func(t *testing.T) {
		raw := "5;ext=foo\r\nhello\r\n5 ;ext=bar;token=val\r\nworld\r\n0;terminal\r\n\r\n"
		cr := h1.NewChunkedReader(bufio.NewReader(strings.NewReader(raw)))
		data, err := io.ReadAll(cr)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if string(data) != "helloworld" {
			t.Fatalf("expected helloworld, got %s", string(data))
		}
	})

	// Overflow
	t.Run("ChunkSizeIntegerOverflow", func(t *testing.T) {
		raw := "7fffffffffffffff0\r\nhello\r\n0\r\n\r\n"
		cr := h1.NewChunkedReader(bufio.NewReader(strings.NewReader(raw)))
		_, err := io.ReadAll(cr)
		if err == nil {
			t.Fatal("expected error on overflow")
		}
	})
}

func TestChunkedZeroAllocs(t *testing.T) {
	var hexBuf [16]byte
	allocs := testing.AllocsPerRun(100, func() {
		_ = h1.FormatHexUint(&hexBuf, 255)
		_, _, _ = h1.ParseHexUint([]byte("ff"))
	})
	if allocs != 0 {
		t.Fatalf("expected 0 allocs, got %v", allocs)
	}
}
