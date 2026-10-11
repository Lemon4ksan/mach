// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package hpack_test

import (
	"errors"
	"testing"

	"github.com/lemon4ksan/foundation/testing/assert"
	"github.com/lemon4ksan/foundation/testing/require"

	"github.com/lemon4ksan/mach/proto/codec/hpack"
)

// TestHPACK_RFC7541_Section4_2_DynamicTableSizeUpdatePosition tests that dynamic table size
// updates MUST occur at the beginning of the first header block following the change,
// and that any update occurring elsewhere MUST be treated as a decoding error (RFC 7541 §4.2 & §6.3).
func TestHPACK_RFC7541_Section4_2_DynamicTableSizeUpdatePosition(t *testing.T) {
	t.Parallel()

	t.Run("ValidTableSizeUpdateAtBeginning", func(t *testing.T) {
		hp := hpack.AcquireHPACK()
		defer hpack.ReleaseHPACK(hp)

		// 0x20 is dynamic table size update with size 0 (001 00000)
		// 0x82 is indexed header :method: GET (index 2)
		block := []byte{0x20, 0x82}

		hf := hpack.AcquireHeaderField()
		defer hpack.ReleaseHeaderField(hf)

		rem, err := hp.Next(hf, block)
		require.NoError(t, err)
		assert.Equal(t, ":method", hf.Key())
		assert.Equal(t, "GET", hf.Value())
		assert.Empty(t, rem)
	})

	t.Run("MultipleTableSizeUpdatesAtBeginning", func(t *testing.T) {
		hp := hpack.AcquireHPACK()
		defer hpack.ReleaseHPACK(hp)

		// RFC 7541 §4.2 allows up to two updates when size changes multiple times
		// 0x20: size 0
		// 0x3f, 0xe1, 0x1f: size 4096 (001 11111, then 4096-31)
		// 0x82: :method: GET
		var block []byte
		block = hpack.AppendInt(block, 5, 0)
		// 0x20 prefix for table size update
		block[0] = 0x20

		var update2 []byte
		update2 = hpack.AppendInt(update2, 5, 2048)
		update2[0] |= 0x20
		block = append(block, update2...)
		block = append(block, 0x82)

		hf := hpack.AcquireHeaderField()
		defer hpack.ReleaseHeaderField(hf)

		rem, err := hp.Next(hf, block)
		require.NoError(t, err)
		assert.Equal(t, ":method", hf.Key())
		assert.Equal(t, "GET", hf.Value())
		assert.Empty(t, rem)
	})

	t.Run("ForbiddenTableSizeUpdateAfterHeaderField", func(t *testing.T) {
		hp := hpack.AcquireHPACK()
		defer hpack.ReleaseHPACK(hp)

		// 0x82 is indexed header :method: GET
		// 0x20 is dynamic table size update (size 0) placed AFTER a header field
		block := []byte{0x82, 0x20}

		hf := hpack.AcquireHeaderField()
		defer hpack.ReleaseHeaderField(hf)

		// First header field succeeds
		rem, err := hp.Next(hf, block)
		require.NoError(t, err)
		assert.Equal(t, ":method", hf.Key())
		assert.Equal(t, []byte{0x20}, rem)

		// Next instruction is dynamic table size update in the middle of block -> MUST error!
		hf.Reset()
		_, err = hp.Next(hf, rem)
		require.Error(t, err)
		assert.True(t, errors.Is(err, hpack.ErrTableSizeUpdateNotAllowed))
	})

	t.Run("ForbiddenTableSizeUpdateInDecodeIterator", func(t *testing.T) {
		hp := hpack.AcquireHPACK()
		defer hpack.ReleaseHPACK(hp)

		// Block: indexed field 0x82 followed by dynamic table size update 0x20
		block := []byte{0x82, 0x20}

		var count int
		var gotErr error
		for hf, err := range hp.Decode(block) {
			if err != nil {
				gotErr = err
				break
			}
			count++
			assert.Equal(t, ":method", hf.Key())
		}

		assert.Equal(t, 1, count)
		require.Error(t, gotErr)
		assert.True(t, errors.Is(gotErr, hpack.ErrTableSizeUpdateNotAllowed))
	})

	t.Run("TableSizeExceedingMaxCapacity", func(t *testing.T) {
		hp := hpack.AcquireHPACK()
		defer hpack.ReleaseHPACK(hp)

		hp.SetMaxCapacity(100)

		// Encode table size update of 200 > MaxCapacity (100)
		var block []byte
		block = hpack.AppendInt(block, 5, 200)
		block[0] |= 0x20

		hf := hpack.AcquireHeaderField()
		defer hpack.ReleaseHeaderField(hf)

		_, err := hp.Next(hf, block)
		require.Error(t, err)
		assert.True(t, errors.Is(err, hpack.ErrTableSizeLimitExceeded))
	})
}

// TestHPACK_RFC7541_Section5_2_HuffmanPaddingAndEOS tests Huffman decoding validation:
// 1. Padding strictly longer than 7 bits MUST be treated as a decoding error.
// 2. Padding not consisting of all 1s (most significant bits of EOS) MUST be treated as a decoding error.
// 3. String literal containing explicit EOS symbol MUST be treated as a decoding error.
func TestHPACK_RFC7541_Section5_2_HuffmanPaddingAndEOS(t *testing.T) {
	t.Parallel()

	t.Run("ValidHuffmanPadding", func(t *testing.T) {
		// "a" is 5 bits: 00011. Padded with 3 bits of 1s (111) -> 00011 111 = 0x1f.
		decoded, err := hpack.AppendHuffmanDecode(nil, []byte{0x1f})
		require.NoError(t, err)
		assert.Equal(t, "a", string(decoded))
	})

	t.Run("InvalidPadding_NonOneBits", func(t *testing.T) {
		// "a" is 00011. Corrupt padding with 110 (last bit 0) -> 00011 110 = 0x1e.
		_, err := hpack.AppendHuffmanDecode(nil, []byte{0x1e})
		require.Error(t, err)
		assert.True(t, errors.Is(err, hpack.ErrInvalidHuffman))
	})

	t.Run("InvalidPadding_OverlongPaddingGreaterThan7Bits", func(t *testing.T) {
		// "a" (0x1f) + extra 8 bits of 1s (0xff) -> padding is 11 bits > 7 bits!
		_, err := hpack.AppendHuffmanDecode(nil, []byte{0x1f, 0xff})
		require.Error(t, err)
		assert.True(t, errors.Is(err, hpack.ErrInvalidHuffman))
	})

	t.Run("StandaloneEOSPaddingByte", func(t *testing.T) {
		// 0xff by itself without preceding symbol is 8 bits of 1s -> padding > 7 bits!
		_, err := hpack.AppendHuffmanDecode(nil, []byte{0xff})
		require.Error(t, err)
		assert.True(t, errors.Is(err, hpack.ErrInvalidHuffman))
	})

	t.Run("ExplicitEOSSymbolInString", func(t *testing.T) {
		// EOS symbol is 30 bits of 1s (0x3fffffff).
		// 30 bits of 1s + 2 bits of 1s padding = 32 bits of 0xff (4 bytes of 0xff).
		_, err := hpack.AppendHuffmanDecode(nil, []byte{0xff, 0xff, 0xff, 0xff})
		require.Error(t, err)
		assert.True(t, errors.Is(err, hpack.ErrInvalidHuffman))
	})

	t.Run("HeaderBlock_InvalidHuffmanPadding_CausesCompressionError", func(t *testing.T) {
		hp := hpack.AcquireHPACK()
		defer hpack.ReleaseHPACK(hp)

		// Literal Header Field without Indexing - Indexed Name (idx = 2, :method)
		// 0x02
		// Value length: 1 byte, Huffman encoded (0x81)
		// Value: 0x1e (corrupt padding: 'a' with 110 padding)
		block := []byte{0x02, 0x81, 0x1e}

		hf := hpack.AcquireHeaderField()
		defer hpack.ReleaseHeaderField(hf)

		_, err := hp.Next(hf, block)
		require.Error(t, err)
		assert.True(t, errors.Is(err, hpack.ErrInvalidHuffman))
	})

	t.Run("HuffmanDecodeToString_ValidAndInvalid", func(t *testing.T) {
		s, err := hpack.HuffmanDecodeToString([]byte{0x1f})
		require.NoError(t, err)
		assert.Equal(t, "a", s)

		_, err = hpack.HuffmanDecodeToString([]byte{0x1e})
		require.Error(t, err)
		assert.True(t, errors.Is(err, hpack.ErrInvalidHuffman))
	})
}

// TestHPACK_RFC7541_Section2_3_3_IndexAddressSpace tests index address space limits:
// Static entries: 1..61
// Dynamic entries: 62..
// Index 0: forbidden
// Index out of bounds: error
func TestHPACK_RFC7541_Section2_3_3_IndexAddressSpace(t *testing.T) {
	t.Parallel()

	hp := hpack.AcquireHPACK()
	defer hpack.ReleaseHPACK(hp)

	hf := hpack.AcquireHeaderField()
	defer hpack.ReleaseHeaderField(hf)

	// Index 0 is forbidden (RFC 7541 §6.1)
	_, err := hp.Next(hf, []byte{0x80})
	require.Error(t, err)
	assert.True(t, errors.Is(err, hpack.ErrIndexZero))

	// Index 61 is static table limit (:subreponse / www-authenticate)
	rem, err := hp.Next(hf, []byte{0x80 | 61})
	require.NoError(t, err)
	assert.Equal(t, "www-authenticate", hf.Key())
	assert.Empty(t, rem)

	// Index 62 is dynamic entry 1, but dynamic table is empty -> MUST error with ErrIndexOutOfBounds!
	hf.Reset()
	_, err = hp.Next(hf, []byte{0x80 | 62})
	require.Error(t, err)
	assert.True(t, errors.Is(err, hpack.ErrIndexOutOfBounds))

	// Add dynamic entry
	hf.Set("x-custom", "value1")
	hp.AppendHeader(nil, hf, true)

	// Now index 62 exists
	hf.Reset()
	rem, err = hp.Next(hf, []byte{0x80 | 62})
	require.NoError(t, err)
	assert.Equal(t, "x-custom", hf.Key())
	assert.Equal(t, "value1", hf.Value())
	assert.Empty(t, rem)

	// Index 63 is still out of bounds
	hf.Reset()
	_, err = hp.Next(hf, []byte{0x80 | 63})
	require.Error(t, err)
	assert.True(t, errors.Is(err, hpack.ErrIndexOutOfBounds))
}

// TestHPACK_RFC7541_Section4_4_EntryLargerThanMaxTableSize tests that attempting to add
// an entry larger than maximum dynamic table size causes the table to be emptied
// of all existing entries and results in an empty table (RFC 7541 §4.4).
func TestHPACK_RFC7541_Section4_4_EntryLargerThanMaxTableSize(t *testing.T) {
	t.Parallel()

	hp := hpack.AcquireHPACK()
	defer hpack.ReleaseHPACK(hp)

	hp.SetMaxTableSize(100)

	hf := hpack.AcquireHeaderField()
	defer hpack.ReleaseHeaderField(hf)

	// Insert small entry
	hf.Set("foo", "bar")
	hp.AppendHeader(nil, hf, true)
	assert.Equal(t, 1, hp.DynamicEntries())
	assert.True(t, hp.DynamicSize() > 0)

	// Now insert entry larger than 100 bytes (e.g. 120 bytes)
	hf.Set("large-key", string(make([]byte, 120)))
	hp.AppendHeader(nil, hf, true)

	// RFC 7541 §4.4: Dynamic table must be emptied and remain empty
	assert.Equal(t, 0, hp.DynamicEntries())
	assert.Equal(t, uint32(0), hp.DynamicSize())
}
