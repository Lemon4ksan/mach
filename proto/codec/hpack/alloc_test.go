// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package hpack_test

import (
	"testing"

	"github.com/lemon4ksan/mach/proto/codec/hpack"
)

func TestAlloc(t *testing.T) {
	allocs := testing.AllocsPerRun(1000, func() {
		hp := hpack.AcquireHPACK()
		defer hpack.ReleaseHPACK(hp)

		hf := hpack.AcquireHeaderField()
		defer hpack.ReleaseHeaderField(hf)

		hf.Set(":method", "GET")
	})
	if allocs != 0 {
		t.Errorf("expected 0 allocations, got %v", allocs)
	}
}

func TestHuffman_ZeroAlloc(t *testing.T) {
	rawStr := "www.example.com/endpoint/query?key=value&foo=bar"
	rawBytes := []byte(rawStr)

	encodedBytes := hpack.HuffmanEncode(nil, rawBytes)
	encodedStr := string(encodedBytes)

	buf := make([]byte, 0, 256)

	t.Run("HuffmanEncode_String", func(t *testing.T) {
		allocs := testing.AllocsPerRun(1000, func() {
			_ = hpack.HuffmanEncode(buf[:0], rawStr)
		})
		if allocs != 0 {
			t.Errorf("expected 0 allocs, got %v", allocs)
		}
	})

	t.Run("HuffmanEncode_Bytes", func(t *testing.T) {
		allocs := testing.AllocsPerRun(1000, func() {
			_ = hpack.HuffmanEncode(buf[:0], rawBytes)
		})
		if allocs != 0 {
			t.Errorf("expected 0 allocs, got %v", allocs)
		}
	})

	t.Run("HuffmanEncodeLength_String", func(t *testing.T) {
		allocs := testing.AllocsPerRun(1000, func() {
			_ = hpack.HuffmanEncodeLength(rawStr)
		})
		if allocs != 0 {
			t.Errorf("expected 0 allocs, got %v", allocs)
		}
	})

	t.Run("HuffmanEncodeLength_Bytes", func(t *testing.T) {
		allocs := testing.AllocsPerRun(1000, func() {
			_ = hpack.HuffmanEncodeLength(rawBytes)
		})
		if allocs != 0 {
			t.Errorf("expected 0 allocs, got %v", allocs)
		}
	})

	t.Run("AppendHuffmanDecode_String", func(t *testing.T) {
		allocs := testing.AllocsPerRun(1000, func() {
			_, _ = hpack.AppendHuffmanDecode(buf[:0], encodedStr)
		})
		if allocs != 0 {
			t.Errorf("expected 0 allocs, got %v", allocs)
		}
	})

	t.Run("AppendHuffmanDecode_Bytes", func(t *testing.T) {
		allocs := testing.AllocsPerRun(1000, func() {
			_, _ = hpack.AppendHuffmanDecode(buf[:0], encodedBytes)
		})
		if allocs != 0 {
			t.Errorf("expected 0 allocs, got %v", allocs)
		}
	})

	t.Run("AppendHuffman_String", func(t *testing.T) {
		allocs := testing.AllocsPerRun(1000, func() {
			_ = hpack.AppendHuffman(buf[:0], rawStr)
		})
		if allocs != 0 {
			t.Errorf("expected 0 allocs, got %v", allocs)
		}
	})

	t.Run("AppendHuffman_Bytes", func(t *testing.T) {
		allocs := testing.AllocsPerRun(1000, func() {
			_ = hpack.AppendHuffman(buf[:0], rawBytes)
		})
		if allocs != 0 {
			t.Errorf("expected 0 allocs, got %v", allocs)
		}
	})
}
