// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package hpack_test

import (
	"errors"
	"math"
	"testing"

	"github.com/lemon4ksan/foundation/testing/assert"
	"github.com/lemon4ksan/foundation/testing/require"

	"github.com/lemon4ksan/mach/proto/codec/hpack"
)

// Tests that dynamic table size update rejecting integers > MaxCapacity cannot be bypassed
// by 64-bit integer truncation (e.g. n = (1<<32) + 100 where uint32(n) == 100 <= MaxCapacity).
func TestChromium_HPACK_DynamicTableSizeUpdate_64BitOverflowBypass(t *testing.T) {
	hp := hpack.AcquireHPACK()
	defer hpack.ReleaseHPACK(hp)

	hp.SetMaxCapacity(500)

	// Encode dynamic table size update with n = (1<<32) + 100 = 4294967396.
	// 5-bit prefix: 0x20 | 0x1f = 0x3f
	// Remaining: 4294967396 - 31 = 4294967365 = 0x100000045
	// In 7-bit chunks:
	// 4294967365 % 128 = 69 (0x45) | 0x80 = 0xc5
	// next: (4294967365 >> 7) = 33554432
	// 33554432 = 0x2000000
	var block []byte
	block = hpack.AppendInt(block, 5, uint64(math.MaxUint32)+101)
	block[0] |= 0x20

	hf := hpack.AcquireHeaderField()
	defer hpack.ReleaseHeaderField(hf)

	_, err := hp.Next(hf, block)
	require.Error(
		t,
		err,
		"table size update > MaxCapacity (via 64-bit value) MUST be rejected with ErrTableSizeLimitExceeded",
	)
	assert.True(t, errors.Is(err, hpack.ErrTableSizeLimitExceeded), "error must be ErrTableSizeLimitExceeded")
}

// Tests that when HPACK encoder table size is reduced via SetMaxTableSize,
// the encoder emits a dynamic table size update instruction at the beginning of the next header block (RFC 7541 §4.2).
func TestChromium_HPACK_EncoderEmitsDynamicTableSizeUpdate(t *testing.T) {
	enc := hpack.AcquireHPACK()
	defer hpack.ReleaseHPACK(enc)

	// Set table size to 2048 (default is 4096)
	enc.SetMaxTableSize(2048)

	hf := hpack.AcquireHeaderField()
	defer hpack.ReleaseHeaderField(hf)
	hf.Set("custom-key", "custom-value")

	out := enc.AppendHeader(nil, hf, true)
	require.NotEmpty(t, out)

	// The first byte of output MUST be a Dynamic Table Size Update instruction ('001' prefix = 0x20)
	assert.Equal(
		t,
		byte(0x20),
		out[0]&0xe0,
		"encoder MUST emit dynamic table size update (0x20) at start of first header block after size change",
	)
}

// Ported from Chromium net/spdy: SpdySessionTest.UpdateHeaderTableSize
// Verifies dynamic table eviction when table size shrinks.
func TestChromium_HPACK_DynamicTableShrinkEvictsOldest(t *testing.T) {
	hp := hpack.AcquireHPACK()
	defer hpack.ReleaseHPACK(hp)

	hf := hpack.AcquireHeaderField()
	defer hpack.ReleaseHeaderField(hf)

	// Insert 3 entries of size: 32 + len(key) + len(val)
	// Entry 1: key1: val1 -> 32 + 4 + 4 = 40 bytes
	hf.Set("key1", "val1")
	hp.AppendHeader(nil, hf, true)

	// Entry 2: key2: val2 -> 40 bytes
	hf.Set("key2", "val2")
	hp.AppendHeader(nil, hf, true)

	// Entry 3: key3: val3 -> 40 bytes
	hf.Set("key3", "val3")
	hp.AppendHeader(nil, hf, true)

	assert.Equal(t, 3, hp.DynamicEntries())
	assert.Equal(t, uint32(120), hp.DynamicSize())

	// Shrink max table size to 50 bytes. Only the most recent entry (key3) fits (40 bytes <= 50).
	// key1 and key2 MUST be evicted.
	hp.SetMaxTableSize(50)

	assert.Equal(t, 1, hp.DynamicEntries())
	assert.Equal(t, uint32(40), hp.DynamicSize())

	// Index 62 is the most recent entry (key3)
	hf.Reset()
	_, err := hp.Next(hf, []byte{0x80 | 62})
	require.NoError(t, err)
	assert.Equal(t, "key3", hf.Key())
	assert.Equal(t, "val3", hf.Value())

	// Index 63 (evicted key2) MUST be out of bounds
	hf.Reset()
	_, err = hp.Next(hf, []byte{0x80 | 63})
	require.Error(t, err)
	assert.True(t, errors.Is(err, hpack.ErrIndexOutOfBounds))
}

// Tests Huffman decoding attack vectors:
// 1. Explicit EOS code (30 bits of 1s: 0x3fffffff) inside string
// 2. Overlong padding (> 7 bits of 1s)
// 3. Padding bits containing 0s
func TestChromium_HPACK_HuffmanDecodingAttacks(t *testing.T) {
	t.Run("EOSCodeWithinStringLiteral", func(t *testing.T) {
		// EOS symbol is 30 bits of 1s: 11111111 11111111 11111111 111111
		// Followed by 2 bits of 1s padding: 32 bits = 4 bytes of 0xff
		_, err := hpack.AppendHuffmanDecode(nil, []byte{0xff, 0xff, 0xff, 0xff})
		require.Error(t, err, "explicit EOS symbol MUST be rejected with ErrInvalidHuffman")
		assert.True(t, errors.Is(err, hpack.ErrInvalidHuffman))
	})

	t.Run("OverlongPaddingGreaterThan7Bits", func(t *testing.T) {
		// 'a' is 00011 (5 bits). 3 bits padding = 111 -> 0x1f.
		// Add an extra byte of 0xff (8 more bits of 1s) -> total 11 bits padding > 7 bits!
		_, err := hpack.AppendHuffmanDecode(nil, []byte{0x1f, 0xff})
		require.Error(t, err, "padding > 7 bits MUST be rejected with ErrInvalidHuffman")
		assert.True(t, errors.Is(err, hpack.ErrInvalidHuffman))
	})

	t.Run("CorruptedPaddingBitsNotAllOnes", func(t *testing.T) {
		// 'a' is 00011 (5 bits). Corrupt last 3 bits to 101 -> 00011 101 = 0x1d.
		_, err := hpack.AppendHuffmanDecode(nil, []byte{0x1d})
		require.Error(t, err, "padding with 0 bits MUST be rejected with ErrInvalidHuffman")
		assert.True(t, errors.Is(err, hpack.ErrInvalidHuffman))
	})
}
