// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ws_test

import (
	"testing"

	"github.com/lemon4ksan/mach/proto/ws"
)

func TestAlloc_ParseHeader(t *testing.T) {
	headerBytes := []byte("\x81\x8D\xDE\xAD\xBE\xEF") // Masked text frame with 13-byte length

	var sinkHeader ws.Header
	var sinkLen int
	var sinkErr error

	allocs := testing.AllocsPerRun(100, func() {
		sinkHeader, sinkLen, sinkErr = ws.ParseHeader(headerBytes)
	})

	if allocs > 0 {
		t.Fatalf("ParseHeader allocated %f times; want 0", allocs)
	}
	_ = sinkHeader
	_ = sinkLen
	_ = sinkErr
}

func TestAlloc_WriteHeader(t *testing.T) {
	h := ws.Header{
		Fin:        true,
		Opcode:     ws.OpcodeText,
		Masked:     true,
		MaskKey:    ws.MaskKey{0xDE, 0xAD, 0xBE, 0xEF},
		PayloadLen: 125,
	}

	buf := make([]byte, 14)
	var sinkN int
	var sinkErr error

	allocs := testing.AllocsPerRun(100, func() {
		sinkN, sinkErr = ws.WriteHeader(buf, h)
	})

	if allocs > 0 {
		t.Fatalf("WriteHeader allocated %f times; want 0", allocs)
	}
	_ = sinkN
	_ = sinkErr
}

func TestAlloc_WriteFrame(t *testing.T) {
	h := ws.Header{
		Fin:        true,
		Opcode:     ws.OpcodeBinary,
		Masked:     true,
		MaskKey:    ws.MaskKey{1, 2, 3, 4},
		PayloadLen: 64,
	}
	payload := make([]byte, 64)
	dst := make([]byte, 14+64)

	var sinkN int
	var sinkErr error

	allocs := testing.AllocsPerRun(100, func() {
		sinkN, sinkErr = ws.WriteFrame(dst, h, payload)
	})

	if allocs > 0 {
		t.Fatalf("WriteFrame allocated %f times; want 0", allocs)
	}
	_ = sinkN
	_ = sinkErr
}

func TestAlloc_Mask_SWAR(t *testing.T) {
	sizes := []int{64, 1024, 65536}
	key := ws.MaskKey{0xAA, 0x55, 0xAA, 0x55}

	for _, sz := range sizes {
		buf := make([]byte, sz)
		allocs := testing.AllocsPerRun(100, func() {
			ws.Mask(buf, key)
		})
		if allocs > 0 {
			t.Fatalf("Mask(%d bytes) allocated %f times; want 0", sz, allocs)
		}

		allocsOffset := testing.AllocsPerRun(100, func() {
			ws.MaskWithOffset(buf, key, 3)
		})
		if allocsOffset > 0 {
			t.Fatalf("MaskWithOffset(%d bytes) allocated %f times; want 0", sz, allocsOffset)
		}
	}
}

func TestAlloc_ValidateKey(t *testing.T) {
	const key = "dGhlIHNhbXBsZSBub25jZQ=="
	var sinkValid bool

	allocs := testing.AllocsPerRun(100, func() {
		sinkValid = ws.ValidateKey(key)
	})

	if allocs > 0 {
		t.Fatalf("ValidateKey allocated %f times; want 0", allocs)
	}
	if !sinkValid {
		t.Fatal("expected key to be valid")
	}
}

func TestAlloc_AppendAccept(t *testing.T) {
	const key = "dGhlIHNhbXBsZSBub25jZQ=="
	dst := make([]byte, 0, 64)

	allocs := testing.AllocsPerRun(100, func() {
		res := ws.AppendAccept(dst[:0], key)
		_ = res
	})

	if allocs > 0 {
		t.Fatalf("AppendAccept allocated %f times; want 0", allocs)
	}
}

func TestAlloc_HasToken(t *testing.T) {
	const hdr = "keep-alive, Upgrade, something-else"
	const tok = "upgrade"
	var sinkHas bool

	allocs := testing.AllocsPerRun(100, func() {
		sinkHas = ws.HasToken(hdr, tok)
	})

	if allocs > 0 {
		t.Fatalf("HasToken allocated %f times; want 0", allocs)
	}
	if !sinkHas {
		t.Fatal("expected HasToken to return true")
	}
}

func TestAlloc_FormatClosePayload(t *testing.T) {
	buf := make([]byte, 64)
	var sinkN int
	var sinkErr error

	allocs := testing.AllocsPerRun(100, func() {
		sinkN, sinkErr = ws.FormatClosePayload(buf, ws.StatusNormalClosure, "bye")
	})

	if allocs > 0 {
		t.Fatalf("FormatClosePayload allocated %f times; want 0", allocs)
	}
	_ = sinkN
	_ = sinkErr
}

func TestAlloc_Parser_Decode(t *testing.T) {
	data := []byte("\x81\x05Hello\x82\x05World")
	var parser ws.Parser
	chunks := make([]ws.FrameChunk, 0, 16)

	allocs := testing.AllocsPerRun(100, func() {
		chunks = chunks[:0]
		err := parser.Decode(data, &chunks)
		if err != nil {
			t.Fatal(err)
		}
		if len(chunks) != 2 {
			t.Fatalf("expected 2 chunks, got %d", len(chunks))
		}
	})

	if allocs > 0 {
		t.Fatalf("Parser.Decode allocated %f times; want 0", allocs)
	}
}
