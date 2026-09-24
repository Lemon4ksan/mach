// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package qpack

import (
	"bytes"
	"math"
	"math/rand"
	"testing"

	"github.com/lemon4ksan/foundation/testing/assert"
	"github.com/lemon4ksan/foundation/testing/require"
)

// -----------------------------------------------------------------------------
// Adversarial Wire Encoding Helpers
// -----------------------------------------------------------------------------

func encodeVarintAdversarial(prefixMask byte, prefixBits uint8, value uint64) []byte {
	maxPrefix := uint64((1 << prefixBits) - 1)
	if value < maxPrefix {
		return []byte{prefixMask | byte(value)}
	}
	res := []byte{prefixMask | byte(maxPrefix)}
	value -= maxPrefix
	for value >= 128 {
		res = append(res, byte(value%128)|0x80)
		value /= 128
	}
	res = append(res, byte(value))
	return res
}

func encodeHeaderPrefixAdversarial(encodedRIC uint64, sign bool, deltaBase uint64) []byte {
	out := encodeVarintAdversarial(0x00, 8, encodedRIC)
	var signMask byte
	if sign {
		signMask = 0x80
	}
	out = append(out, encodeVarintAdversarial(signMask, 7, deltaBase)...)
	return out
}

func encodeLiteralStringAdversarial(huffman bool, data []byte) []byte {
	var mask byte
	if huffman {
		mask = 0x80
	}
	out := encodeVarintAdversarial(mask, 7, uint64(len(data)))
	return append(out, data...)
}

func encodeLiteralFieldWithoutRefAdversarial(name, value string) []byte {
	nameLenBytes := encodeVarintAdversarial(0x20, 3, uint64(len(name)))
	out := append(nameLenBytes, []byte(name)...)
	valBytes := encodeLiteralStringAdversarial(false, []byte(value))
	return append(out, valBytes...)
}

// -----------------------------------------------------------------------------
// 1. Wire Corruption: Truncated Prefix
// -----------------------------------------------------------------------------

func TestM5DecoderAdversarial_WireCorruption_TruncatedPrefix(t *testing.T) {
	testCases := []struct {
		name string
		data []byte
	}{
		{
			name: "EmptyHeaderBlock",
			data: []byte{},
		},
		{
			name: "IncompleteRICVarint1Byte",
			// 0xff requires continuation bytes for 8-bit prefix varint
			data: []byte{0xff},
		},
		{
			name: "IncompleteRICVarint2Bytes",
			data: []byte{0xff, 0x80},
		},
		{
			name: "MissingDeltaBaseAfterRICZero",
			data: []byte{0x00},
		},
		{
			name: "MissingDeltaBaseAfterRICNonZero",
			data: []byte{0x05},
		},
		{
			name: "IncompleteDeltaBaseVarintSignZero",
			// Delta Base has S=0 and prefix 0x7f, requiring continuation bytes
			data: []byte{0x00, 0x7f},
		},
		{
			name: "IncompleteDeltaBaseVarintSignOne",
			// Delta Base has S=1 and prefix 0x7f, requiring continuation bytes
			data: []byte{0x00, 0xff},
		},
		{
			name: "IncompleteDeltaBaseVarintContinuation",
			data: []byte{0x00, 0xff, 0x80},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			runProgressiveDecoderTests(t, 1024, 1, 0, func(t *testing.T, f *progressiveDecoderTestFixture) {
				f.DecodeHeaderBlock(tc.data)
				assert.Truef(t, f.handler.errorDetected, "error should be detected for %s", tc.name)
				assert.Equal(t, uint64(QPACK_DECOMPRESSION_FAILED), f.handler.errorCode)
				assert.Equal(t, "Incomplete header data prefix.", f.handler.errorMessage)
				assert.False(t, f.handler.decodingCompleted)
			})
		})
	}
}

// -----------------------------------------------------------------------------
// 2. Wire Corruption: Truncated String Literals
// -----------------------------------------------------------------------------

func TestM5DecoderAdversarial_WireCorruption_TruncatedStringLiterals(t *testing.T) {
	testCases := []struct {
		name string
		data []byte
	}{
		{
			name: "LiteralWithoutRef_NameTruncated",
			// Prefix 0000, opcode 0x27, 0x05 (name len 12), only 4 bytes of name
			data: decodeHexOrPanic("00002705666f6f62"),
		},
		{
			name: "LiteralWithoutRef_ValueTruncated",
			// Prefix 0000, name "k" (len 1), value len 10, only 3 bytes of value
			data: decodeHexOrPanic("0000216b0a76616c"),
		},
		{
			name: "LiteralWithoutRef_NameLengthVarintTruncated",
			// Opcode 0x27 followed by continuation byte 0x80 indicating more length bytes
			data: decodeHexOrPanic("00002780"),
		},
		{
			name: "LiteralWithoutRef_ValueLengthVarintTruncated",
			// Valid name "k", but value length varint truncated
			data: decodeHexOrPanic("0000216b7f80"),
		},
		{
			name: "LiteralWithStaticNameRef_ValueTruncated",
			// Prefix 0000, static index 0 (:authority), value len 15, only 4 bytes sent
			data: decodeHexOrPanic("0000500f6578616d"),
		},
		{
			name: "LiteralWithStaticNameRef_ValueLengthVarintTruncated",
			// Static index 0, value length varint truncated
			data: decodeHexOrPanic("0000507f80"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			runProgressiveDecoderTests(t, 1024, 1, 0, func(t *testing.T, f *progressiveDecoderTestFixture) {
				f.DecodeHeaderBlock(tc.data)
				assert.Truef(t, f.handler.errorDetected, "error should be detected for %s", tc.name)
				assert.Equal(t, uint64(QPACK_DECOMPRESSION_FAILED), f.handler.errorCode)
				assert.Equal(t, "Incomplete header block.", f.handler.errorMessage)
				assert.False(t, f.handler.decodingCompleted)
			})
		})
	}

	t.Run("LiteralWithDynamicNameRef_ValueTruncated", func(t *testing.T) {
		runProgressiveDecoderTests(t, 1024, 1, 0, func(t *testing.T, f *progressiveDecoderTestFixture) {
			f.headerTable.SetDynamicTableCapacity(1024)
			f.headerTable.InsertEntry("dyn-key", "dyn-val")

			// RIC=1 (encoded 2), Base=1 (DeltaBase=0, S=0) -> prefix 0200
			// Dynamic name ref: 0100 T(0) NNNN(0) = 0x40 (rel index 0), value len 10, only 3 bytes sent
			data := decodeHexOrPanic("0200400a76616c")
			f.DecodeHeaderBlock(data)
			assert.True(t, f.handler.errorDetected)
			assert.Equal(t, "Incomplete header block.", f.handler.errorMessage)
		})
	})

	t.Run("LiteralWithPostBaseNameRef_ValueTruncated", func(t *testing.T) {
		runProgressiveDecoderTests(t, 1024, 1, 0, func(t *testing.T, f *progressiveDecoderTestFixture) {
			f.headerTable.SetDynamicTableCapacity(1024)
			f.headerTable.InsertEntry("dyn-key", "dyn-val")

			// RIC=1 (encoded 2), DeltaBase=0, S=1 -> Base = 1 - 0 - 1 = 0 -> prefix 0280
			// PostBase name ref: 0000 NNN(0) = 0x00, value len 8, only 2 bytes sent
			data := decodeHexOrPanic("028000086162")
			f.DecodeHeaderBlock(data)
			assert.True(t, f.handler.errorDetected)
			assert.Equal(t, "Incomplete header block.", f.handler.errorMessage)
		})
	})
}

// -----------------------------------------------------------------------------
// 3. Wire Corruption: Invalid Huffman Bit Patterns
// -----------------------------------------------------------------------------

func TestM5DecoderAdversarial_WireCorruption_InvalidHuffmanBitPatterns(t *testing.T) {
	testCases := []struct {
		name string
		data []byte
	}{
		{
			name: "NameInvalidPaddingNonOneBits",
			// Last byte ends in 0 bit padding instead of 1s
			data: decodeHexOrPanic("00002f0125a849e95ba97d7e8925a849e95bb8e8b4bf"),
		},
		{
			name: "ValueInvalidPaddingNonOneBits",
			data: decodeHexOrPanic("00002f0125a849e95ba97d7f8925a849e95bb8e8b4be"),
		},
		{
			name: "NameOversizedPaddingMoreThan7Bits",
			// Trailing 0xff byte represents padding > 7 bits
			data: decodeHexOrPanic("00002f0225a849e95ba97d7fff8925a849e95bb8e8b4bf"),
		},
		{
			name: "ValueOversizedPaddingMoreThan7Bits",
			data: decodeHexOrPanic("00002f0125a849e95ba97d7f8a25a849e95bb8e8b4bfff"),
		},
		{
			name: "NameExplicitEOSSymbolForbidden",
			// EOS symbol is 30 consecutive 1s: 0x3fffffff. Wire includes EOS pattern:
			// 0x2f (H=1), len 4, 0xff 0xff 0xff 0xff
			data: decodeHexOrPanic("00002f04ffffffff8925a849e95bb8e8b4bf"),
		},
		{
			name: "ValueExplicitEOSSymbolForbidden",
			data: decodeHexOrPanic("00002f0125a849e95ba97d7f84ffffffff"),
		},
		{
			name: "StaticNameRef_ValueInvalidPadding",
			// Static index 0 (:authority), H=1 on value, invalid padding
			data: decodeHexOrPanic("0000508925a849e95bb8e8b4be"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			runProgressiveDecoderTests(t, 1024, 1, 0, func(t *testing.T, f *progressiveDecoderTestFixture) {
				f.DecodeHeaderBlock(tc.data)
				assert.Truef(t, f.handler.errorDetected, "error should be detected for %s", tc.name)
				assert.Equal(t, uint64(QPACK_DECOMPRESSION_FAILED), f.handler.errorCode)
				assert.Equal(t, "Error in Huffman-encoded string.", f.handler.errorMessage)
			})
		})
	}

	t.Run("EncoderStream_InvalidHuffmanOnName", func(t *testing.T) {
		f := newDecoderTestFixture(t, FragmentModeSingleChunk, 1024, 1)
		// InsertWithoutNameRef: 01 H(1) NNNNN(5), name len 5 with invalid padding
		// 0x65, invalid huffman bytes, 0x03, 'b', 'a', 'r'
		badEncoderStream := decodeHexOrPanic("6525a849e97e03626172")
		f.DecodeEncoderStreamData(badEncoderStream)
		require.Equal(t, 1, len(f.encoderStreamErrorDelegate.calls))
		assert.Equal(
			t,
			uint64(QUIC_QPACK_ENCODER_STREAM_HUFFMAN_ENCODING_ERROR),
			f.encoderStreamErrorDelegate.calls[0].ErrorCode,
		)
		assert.Equal(t, "Error in Huffman-encoded string.", f.encoderStreamErrorDelegate.calls[0].ErrorMessage)
	})

	t.Run("EncoderStream_InvalidHuffmanOnValue", func(t *testing.T) {
		f := newDecoderTestFixture(t, FragmentModeSingleChunk, 1024, 1)
		// InsertWithNameRef: 1 T(1) NNNNNN(0), H(1) len 5 with invalid padding
		badEncoderStream := decodeHexOrPanic("c08525a849e97e")
		f.DecodeEncoderStreamData(badEncoderStream)
		require.Equal(t, 1, len(f.encoderStreamErrorDelegate.calls))
		assert.Equal(
			t,
			uint64(QUIC_QPACK_ENCODER_STREAM_HUFFMAN_ENCODING_ERROR),
			f.encoderStreamErrorDelegate.calls[0].ErrorCode,
		)
		assert.Equal(t, "Error in Huffman-encoded string.", f.encoderStreamErrorDelegate.calls[0].ErrorMessage)
	})
}

// -----------------------------------------------------------------------------
// 4. Wire Corruption: Malformed Varints
// -----------------------------------------------------------------------------

func TestM5DecoderAdversarial_WireCorruption_MalformedVarints(t *testing.T) {
	testCases := []struct {
		name string
		data []byte
	}{
		{
			name: "Prefix_RICVarintTooManyExtensionBytes",
			// 0xff followed by 11 continuation bytes
			data: decodeHexOrPanic("ff8080808080808080808080"),
		},
		{
			name: "Prefix_DeltaBaseVarintTooManyExtensionBytes",
			// RIC=0 (0x00), DeltaBase 0x7f followed by 11 continuation bytes
			data: decodeHexOrPanic("007f8080808080808080808080"),
		},
		{
			name: "Prefix_RICOverflow64Bit",
			// 0xff followed by 10 extension bytes of 0xff
			data: decodeHexOrPanic("ffffffffffffffffffffff"),
		},
		{
			name: "Prefix_DeltaBaseOverflow64Bit",
			data: decodeHexOrPanic("007fffffffffffffffffffff"),
		},
		{
			name: "FieldLine_IndexedStaticIndexTooLarge",
			// Prefix 0000, 1 1 Index(6-bit prefix: 0x3f) followed by 11 extension bytes
			data: decodeHexOrPanic("0000ff8080808080808080808080"),
		},
		{
			name: "FieldLine_LiteralNameLengthTooLarge",
			// Prefix 0000, opcode 0x27 followed by 11 extension bytes
			data: decodeHexOrPanic("0000278080808080808080808080"),
		},
		{
			name: "FieldLine_LiteralValueLengthTooLarge",
			// Prefix 0000, name "k", value length varint with 11 extension bytes
			data: decodeHexOrPanic("0000216b7f8080808080808080808080"),
		},
		{
			name: "FieldLine_PostBaseIndexTooLarge",
			// Prefix 0000, post-base indexed opcode 0x1f followed by 11 extension bytes
			data: decodeHexOrPanic("00001f8080808080808080808080"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			runProgressiveDecoderTests(t, 1024, 1, 0, func(t *testing.T, f *progressiveDecoderTestFixture) {
				f.DecodeHeaderBlock(tc.data)
				assert.Truef(t, f.handler.errorDetected, "error should be detected for %s", tc.name)
				assert.Equal(t, uint64(QPACK_DECOMPRESSION_FAILED), f.handler.errorCode)
				assert.Equal(t, "Encoded integer too large.", f.handler.errorMessage)
			})
		})
	}

	t.Run("EncoderStream_CapacityVarintTooLarge", func(t *testing.T) {
		f := newDecoderTestFixture(t, FragmentModeSingleChunk, 1024, 1)
		// SetDynamicTableCapacity opcode 0x3f followed by 11 extension bytes
		f.DecodeEncoderStreamData(decodeHexOrPanic("3f8080808080808080808080"))
		require.Equal(t, 1, len(f.encoderStreamErrorDelegate.calls))
		assert.Equal(
			t,
			uint64(QUIC_QPACK_ENCODER_STREAM_INTEGER_TOO_LARGE),
			f.encoderStreamErrorDelegate.calls[0].ErrorCode,
		)
		assert.Equal(t, "Encoded integer too large.", f.encoderStreamErrorDelegate.calls[0].ErrorMessage)
	})

	t.Run("EncoderStream_DuplicateIndexTooLarge", func(t *testing.T) {
		f := newDecoderTestFixture(t, FragmentModeSingleChunk, 1024, 1)
		// Duplicate opcode 0x1f followed by 11 extension bytes
		f.DecodeEncoderStreamData(decodeHexOrPanic("1f8080808080808080808080"))
		require.Equal(t, 1, len(f.encoderStreamErrorDelegate.calls))
		assert.Equal(
			t,
			uint64(QUIC_QPACK_ENCODER_STREAM_INTEGER_TOO_LARGE),
			f.encoderStreamErrorDelegate.calls[0].ErrorCode,
		)
		assert.Equal(t, "Encoded integer too large.", f.encoderStreamErrorDelegate.calls[0].ErrorMessage)
	})
}

// -----------------------------------------------------------------------------
// 5. Wire Corruption: Prefix & Base Integer Overflows
// -----------------------------------------------------------------------------

func TestM5DecoderAdversarial_WireCorruption_PrefixBaseIntegerOverflows(t *testing.T) {
	t.Run("RICDecodeFailure_MaxEntriesZeroWithNonZeroRIC", func(t *testing.T) {
		// Table with MaxCapacity 0 has MaxEntries 0.
		// Encoded RIC = 1 cannot be decoded when MaxEntries = 0.
		runProgressiveDecoderTests(t, 0, 1, 0, func(t *testing.T, f *progressiveDecoderTestFixture) {
			f.DecodeHeaderBlock(decodeHexOrPanic("0100"))
			assert.True(t, f.handler.errorDetected)
			assert.Equal(t, uint64(QPACK_DECOMPRESSION_FAILED), f.handler.errorCode)
			assert.Equal(t, "Error decoding Required Insert Count.", f.handler.errorMessage)
		})
	})

	t.Run("DeltaBaseUnderflow_RIC0_Sign1_DeltaBase0", func(t *testing.T) {
		// RIC = 0, Sign = 1 (S=1), DeltaBase = 0: Base = 0 - 0 - 1 = -1 underflow
		runProgressiveDecoderTests(t, 1024, 1, 0, func(t *testing.T, f *progressiveDecoderTestFixture) {
			f.DecodeHeaderBlock(decodeHexOrPanic("0080"))
			assert.True(t, f.handler.errorDetected)
			assert.Equal(t, uint64(QPACK_DECOMPRESSION_FAILED), f.handler.errorCode)
			assert.Equal(t, "Error calculating Base.", f.handler.errorMessage)
		})
	})

	t.Run("DeltaBaseUnderflow_RIC1_Sign1_DeltaBase1", func(t *testing.T) {
		// RIC = 1 (encoded 2), Sign = 1 (0x81), DeltaBase = 1: Base = 1 - 1 - 1 = -1 underflow
		runProgressiveDecoderTests(t, 1024, 1, 0, func(t *testing.T, f *progressiveDecoderTestFixture) {
			f.headerTable.SetDynamicTableCapacity(1024)
			f.headerTable.InsertEntry("k", "v")

			f.DecodeHeaderBlock(decodeHexOrPanic("0281"))
			assert.True(t, f.handler.errorDetected)
			assert.Equal(t, uint64(QPACK_DECOMPRESSION_FAILED), f.handler.errorCode)
			assert.Equal(t, "Error calculating Base.", f.handler.errorMessage)
		})
	})

	t.Run("DeltaBaseUnderflow_MaxUint64", func(t *testing.T) {
		// Sign=1, DeltaBase=MaxUint64
		// RIC=1 (encoded 2), S=1, DeltaBase=MaxUint64
		runProgressiveDecoderTests(t, 1024, 1, 0, func(t *testing.T, f *progressiveDecoderTestFixture) {
			f.headerTable.SetDynamicTableCapacity(1024)
			f.headerTable.InsertEntry("k", "v")

			var wire []byte
			wire = append(wire, 0x02) // RIC=1
			// S=1 (0x80), DeltaBase varint 7-bit prefix: 0xff, followed by continuation bytes for MaxUint64
			wire = append(wire, encodeVarintAdversarial(0x80, 7, math.MaxUint64)...)
			f.DecodeHeaderBlock(wire)
			assert.True(t, f.handler.errorDetected)
			assert.Equal(t, "Error calculating Base.", f.handler.errorMessage)
		})
	})

	t.Run("DeltaBaseOverflow_PositiveBase", func(t *testing.T) {
		// RIC=1 (encoded 2), Sign=0, DeltaBase=MaxUint64 (overflows 1 + MaxUint64)
		runProgressiveDecoderTests(t, 1024, 1, 0, func(t *testing.T, f *progressiveDecoderTestFixture) {
			f.headerTable.SetDynamicTableCapacity(1024)
			f.headerTable.InsertEntry("k", "v")

			var wire []byte
			wire = append(wire, 0x02) // RIC=1
			wire = append(wire, encodeVarintAdversarial(0x00, 7, math.MaxUint64)...)
			f.DecodeHeaderBlock(wire)
			assert.True(t, f.handler.errorDetected)
			assert.Equal(t, "Error calculating Base.", f.handler.errorMessage)
		})
	})
}

// -----------------------------------------------------------------------------
// 6. Fragmented Feeds Across All 5 Field Line Types
// -----------------------------------------------------------------------------

func TestM5DecoderAdversarial_FragmentedFeeds_AllFiveFieldLineTypes(t *testing.T) {
	// Build a comprehensive valid header block containing all 5 field line types across 8 representations:
	// Dynamic table has 2 entries:
	//   Index 0: "x-custom-dyn-1": "dyn-val-1"
	//   Index 1: "x-custom-post-1": "post-val-1"
	// Base = 1, RIC = 2:
	//   Prefix: RIC=2 (encoded 3), DeltaBase=0, Sign=1 -> Base = 2 - 0 - 1 = 1. Wire: [0x03, 0x80]
	//
	// Field Lines:
	// 1. Indexed Static: :method: GET (static index 17) -> [0xd1]
	// 2. Indexed Dynamic (Relative Index 0 -> Abs 0): x-custom-dyn-1: dyn-val-1 -> [0x80]
	// 3. Indexed Dynamic Post-Base (Post-Base Index 0 -> Abs 1): x-custom-post-1: post-val-1 -> [0x10]
	// 4. Literal Static Name Ref: :path (static index 1), value "/adversarial/test/path" -> [0x51, 0x16, ...val]
	// 5. Literal Dynamic Name Ref (Relative Index 0): x-custom-dyn-1, value "custom-override-val" -> [0x40, 0x13, ...val]
	// 6. Literal Post-Base Name Ref (Post-Base Index 0): x-custom-post-1, value "post-override-val" -> [0x00, 0x11, ...val]
	// 7. Literal Without Name Ref (Plain text): "x-plain-header": "plain-val" -> [0x27, 0x07, "x-plain-header", 0x09, "plain-val"]
	// 8. Literal Without Name Ref (Huffman encoded): "custom-key": "custom-value" ->
	//    [0x2f, 0x01, 0x25, 0xa8, 0x49, 0xe9, 0x5b, 0xa9, 0x7d, 0x7f, 0x89, 0x25, 0xa8, 0x49, 0xe9, 0x5b, 0xb8, 0xe8, 0xb4, 0xbf]

	var block bytes.Buffer
	// Prefix
	block.Write([]byte{0x03, 0x80})

	// 1. Indexed Static
	block.WriteByte(0xd1)

	// 2. Indexed Dynamic
	block.WriteByte(0x80)

	// 3. Indexed Dynamic Post-Base
	block.WriteByte(0x10)

	// 4. Literal Static Name Ref (:path = index 1)
	pathVal := "/adversarial/test/path"
	block.WriteByte(0x51)
	block.WriteByte(byte(len(pathVal)))
	block.WriteString(pathVal)

	// 5. Literal Dynamic Name Ref (Rel index 0)
	dynValOverride := "custom-override-val"
	block.WriteByte(0x40)
	block.WriteByte(byte(len(dynValOverride)))
	block.WriteString(dynValOverride)

	// 6. Literal Post-Base Name Ref (Post-base index 0)
	postValOverride := "post-override-val"
	block.WriteByte(0x00)
	block.WriteByte(byte(len(postValOverride)))
	block.WriteString(postValOverride)

	// 7. Literal Without Name Ref (Plain)
	plainField := encodeLiteralFieldWithoutRefAdversarial("x-plain-header", "plain-val")
	block.Write(plainField)

	// 8. Literal Without Name Ref (Huffman)
	huffmanField := decodeHexOrPanic("2f0125a849e95ba97d7f8925a849e95bb8e8b4bf")
	block.Write(huffmanField)

	fullData := block.Bytes()

	expectedHeaders := []headerCall{
		{Name: ":method", Value: "GET"},
		{Name: "x-custom-dyn-1", Value: "dyn-val-1"},
		{Name: "x-custom-post-1", Value: "post-val-1"},
		{Name: ":path", Value: "/adversarial/test/path"},
		{Name: "x-custom-dyn-1", Value: "custom-override-val"},
		{Name: "x-custom-post-1", Value: "post-override-val"},
		{Name: "x-plain-header", Value: "plain-val"},
		{Name: "custom-key", Value: "custom-value"},
	}

	setupFixture := func() (*ProgressiveDecoder, *mockHeadersHandler, *mockDecodingCompletedVisitor) {
		table := NewDecoderHeaderTable()
		table.SetMaximumDynamicTableCapacity(1024)
		table.SetDynamicTableCapacity(1024)
		table.InsertEntry("x-custom-dyn-1", "dyn-val-1")
		table.InsertEntry("x-custom-post-1", "post-val-1")

		enforcer := newMockBlockedStreamLimitEnforcer(1)
		visitor := newMockDecodingCompletedVisitor()
		handler := newMockHeadersHandler()

		decoder := NewProgressiveDecoder(1, 0, enforcer, visitor, table, handler)
		return decoder, handler, visitor
	}

	assertHeadersMatch := func(t *testing.T, handler *mockHeadersHandler, visitor *mockDecodingCompletedVisitor) {
		assert.False(t, handler.errorDetected)
		assert.True(t, handler.decodingCompleted)
		require.Equal(t, len(expectedHeaders), len(handler.headers))
		for i, exp := range expectedHeaders {
			assert.Equalf(t, exp.Name, handler.headers[i].Name, "header %d name mismatch", i)
			assert.Equalf(t, exp.Value, handler.headers[i].Value, "header %d value mismatch", i)
		}
		require.Equal(t, 1, len(visitor.completedCalls))
		assert.Equal(t, uint64(2), visitor.completedCalls[0].RequiredInsertCount)
	}

	// 1. Single chunk verification
	t.Run("SingleChunk", func(t *testing.T) {
		dec, handler, visitor := setupFixture()
		dec.Decode(fullData)
		dec.EndHeaderBlock()
		assertHeadersMatch(t, handler, visitor)
	})

	// 2. Fixed 1-byte slices (every single byte boundary)
	t.Run("Fixed1ByteSlices", func(t *testing.T) {
		dec, handler, visitor := setupFixture()
		for i := 0; i < len(fullData); i++ {
			dec.Decode(fullData[i : i+1])
		}
		dec.EndHeaderBlock()
		assertHeadersMatch(t, handler, visitor)
	})

	// 3. Fixed 2-byte slices
	t.Run("Fixed2ByteSlices", func(t *testing.T) {
		dec, handler, visitor := setupFixture()
		for i := 0; i < len(fullData); i += 2 {
			end := min(i+2, len(fullData))
			dec.Decode(fullData[i:end])
		}
		dec.EndHeaderBlock()
		assertHeadersMatch(t, handler, visitor)
	})

	// 4. Fixed 3-byte slices
	t.Run("Fixed3ByteSlices", func(t *testing.T) {
		dec, handler, visitor := setupFixture()
		for i := 0; i < len(fullData); i += 3 {
			end := min(i+3, len(fullData))
			dec.Decode(fullData[i:end])
		}
		dec.EndHeaderBlock()
		assertHeadersMatch(t, handler, visitor)
	})

	// 5. Stress testing with 50 randomized slice sequences (chunk sizes chosen from 1, 2, 3)
	t.Run("Random1to3ByteSlices", func(t *testing.T) {
		for seed := int64(1); seed <= 50; seed++ {
			rng := rand.New(rand.NewSource(seed))
			dec, handler, visitor := setupFixture()

			offset := 0
			for offset < len(fullData) {
				chunkSize := rng.Intn(3) + 1 // 1, 2, or 3
				end := min(offset+chunkSize, len(fullData))
				dec.Decode(fullData[offset:end])
				offset = end
			}
			dec.EndHeaderBlock()
			assertHeadersMatch(t, handler, visitor)
		}
	})
}

// -----------------------------------------------------------------------------
// 7. Blocked Stream Limit Enforcement
// -----------------------------------------------------------------------------

func TestM5DecoderAdversarial_BlockedStreamLimit_MaxBlockedZero(t *testing.T) {
	// When maximumBlockedStreams = 0, the very first stream that requires uninserted
	// dynamic table entries must be rejected immediately.
	f := newDecoderTestFixture(t, FragmentModeSingleChunk, 1024, 0)
	f.StartDecoding()

	// Feed prefix with RIC=1 (encoded 2), Base=1 -> dynamic table is empty, so stream blocks
	f.DecodeData(decodeHexOrPanic("020080"))

	assert.True(t, f.handler.errorDetected)
	assert.Equal(t, uint64(QPACK_DECOMPRESSION_FAILED), f.handler.errorCode)
	assert.Equal(t, "Limit on number of blocked streams exceeded.", f.handler.errorMessage)
	assert.False(t, f.handler.decodingCompleted)

	// Verify decoding is completely aborted: subsequent feeds or EndDecoding do nothing
	f.DecodeData(decodeHexOrPanic("d1"))
	f.EndDecoding()
	assert.False(t, f.handler.decodingCompleted)
	assert.Equal(t, 0, len(f.handler.headers))

	// Even if table later receives entry, aborted stream never completes
	f.DecodeEncoderStreamData(decodeHexOrPanic("3fe1076294e703626172"))
	assert.False(t, f.handler.decodingCompleted)
}

func TestM5DecoderAdversarial_BlockedStreamLimit_ExceedLimitAndAbort(t *testing.T) {
	// maximumBlockedStreams = 1
	f := newDecoderTestFixture(t, FragmentModeSingleChunk, 1024, 1)

	// Stream 1 blocks (RIC=1)
	handler1 := newMockHeadersHandler()
	dec1 := f.qpackDecoder.CreateProgressiveDecoder(1, handler1)
	dec1.Decode(decodeHexOrPanic("020080"))
	assert.False(t, handler1.errorDetected)
	assert.False(t, handler1.decodingCompleted)

	// Stream 2 attempts to block (RIC=1) -> exceeds limit (1 active already)
	handler2 := newMockHeadersHandler()
	dec2 := f.qpackDecoder.CreateProgressiveDecoder(2, handler2)
	dec2.Decode(decodeHexOrPanic("020080"))

	assert.True(t, handler2.errorDetected)
	assert.Equal(t, uint64(QPACK_DECOMPRESSION_FAILED), handler2.errorCode)
	assert.Equal(t, "Limit on number of blocked streams exceeded.", handler2.errorMessage)
	assert.False(t, handler2.decodingCompleted)

	// Stream 2 is aborted: further data or EndHeaderBlock() ignored
	dec2.Decode(decodeHexOrPanic("d1"))
	dec2.EndHeaderBlock()
	assert.False(t, handler2.decodingCompleted)

	// Stream 1 is intact: deliver dynamic entry on encoder stream
	f.DecodeEncoderStreamData(decodeHexOrPanic("3fe1076294e703626172"))

	dec1.EndHeaderBlock()
	assert.False(t, handler1.errorDetected)
	assert.True(t, handler1.decodingCompleted)
	require.Equal(t, 1, len(handler1.headers))
	assert.Equal(t, "foo", handler1.headers[0].Name)
	assert.Equal(t, "bar", handler1.headers[0].Value)

	// Stream 2 remains aborted
	assert.False(t, handler2.decodingCompleted)
}

func TestM5DecoderAdversarial_BlockedStreamLimit_DynamicAccounting(t *testing.T) {
	// maximumBlockedStreams = 2
	f := newDecoderTestFixture(t, FragmentModeSingleChunk, 1024, 2)

	// Stream 1 blocks (count = 1)
	h1 := newMockHeadersHandler()
	d1 := f.qpackDecoder.CreateProgressiveDecoder(1, h1)
	d1.Decode(decodeHexOrPanic("020080"))
	assert.False(t, h1.errorDetected)

	// Stream 2 blocks (count = 2)
	h2 := newMockHeadersHandler()
	d2 := f.qpackDecoder.CreateProgressiveDecoder(2, h2)
	d2.Decode(decodeHexOrPanic("020080"))
	assert.False(t, h2.errorDetected)

	// Stream 3 tries to block (count = 3 > 2) -> rejected immediately
	h3 := newMockHeadersHandler()
	d3 := f.qpackDecoder.CreateProgressiveDecoder(3, h3)
	d3.Decode(decodeHexOrPanic("020080"))
	assert.True(t, h3.errorDetected)
	assert.Equal(t, "Limit on number of blocked streams exceeded.", h3.errorMessage)

	// Insert entry on encoder stream -> Stream 1 and 2 unblock!
	f.DecodeEncoderStreamData(decodeHexOrPanic("3fe1076294e703626172"))
	d1.EndHeaderBlock()
	d2.EndHeaderBlock()
	assert.True(t, h1.decodingCompleted)
	assert.True(t, h2.decodingCompleted)

	// Active blocked count is now 0. Stream 4 requires RIC=2 (only 1 entry in table)
	// Stream 4 blocks successfully (count = 1 <= 2)
	h4 := newMockHeadersHandler()
	d4 := f.qpackDecoder.CreateProgressiveDecoder(4, h4)
	d4.Decode(decodeHexOrPanic("030080"))
	assert.False(t, h4.errorDetected)
}

// -----------------------------------------------------------------------------
// 8. Buffer Limit Enforcement
// -----------------------------------------------------------------------------

func TestM5DecoderAdversarial_BufferLimit_Exceeded_SingleAndChunked(t *testing.T) {
	t.Run("SingleChunkExceedsLimit", func(t *testing.T) {
		f := newDecoderTestFixture(t, FragmentModeSingleChunk, 1024, 1)
		handler := newMockHeadersHandler()
		decoder := f.qpackDecoder.CreateProgressiveDecoderWithMaxBufferedData(1, 20, handler)

		// Prefix with RIC=1 -> blocks
		decoder.Decode(decodeHexOrPanic("0200"))
		assert.False(t, handler.errorDetected)

		// Feed 21 bytes while blocked -> exceeds 20 byte limit
		payload21 := bytes.Repeat([]byte{0xd1}, 21)
		decoder.Decode(payload21)

		assert.True(t, handler.errorDetected)
		assert.Equal(t, uint64(QPACK_DECOMPRESSION_FAILED), handler.errorCode)
		assert.Equal(t, "Too much buffered data.", handler.errorMessage)
	})

	t.Run("ChunkedFeedsCumulativelyExceedLimit", func(t *testing.T) {
		f := newDecoderTestFixture(t, FragmentModeSingleChunk, 1024, 1)
		handler := newMockHeadersHandler()
		decoder := f.qpackDecoder.CreateProgressiveDecoderWithMaxBufferedData(1, 20, handler)

		// Prefix with RIC=1 -> blocks
		decoder.Decode(decodeHexOrPanic("0200"))
		assert.False(t, handler.errorDetected)

		// Feed 8 bytes (total 8 <= 20)
		decoder.Decode(bytes.Repeat([]byte{0xd1}, 8))
		assert.False(t, handler.errorDetected)

		// Feed 8 bytes (total 16 <= 20)
		decoder.Decode(bytes.Repeat([]byte{0xd1}, 8))
		assert.False(t, handler.errorDetected)

		// Feed 4 bytes (total 20 <= 20) -> exactly at capacity
		decoder.Decode(bytes.Repeat([]byte{0xd1}, 4))
		assert.False(t, handler.errorDetected)

		// Feed 1 byte (total 21 > 20) -> limit exceeded
		decoder.Decode([]byte{0xd1})
		assert.True(t, handler.errorDetected)
		assert.Equal(t, "Too much buffered data.", handler.errorMessage)
	})

	t.Run("ExactCapacityAllowedAndReplayed", func(t *testing.T) {
		f := newDecoderTestFixture(t, FragmentModeSingleChunk, 1024, 1)
		handler := newMockHeadersHandler()
		decoder := f.qpackDecoder.CreateProgressiveDecoderWithMaxBufferedData(1, 1, handler)

		// Prefix with RIC=1 -> blocks
		decoder.Decode(decodeHexOrPanic("0200"))
		assert.False(t, handler.errorDetected)

		// Feed exactly 1 byte (0x80: relative index 0)
		decoder.Decode(decodeHexOrPanic("80"))
		assert.False(t, handler.errorDetected)
		decoder.EndHeaderBlock()

		// Unblock with dynamic table entry
		f.DecodeEncoderStreamData(decodeHexOrPanic("3fe1076294e703626172"))

		assert.False(t, handler.errorDetected)
		assert.True(t, handler.decodingCompleted)
		require.Equal(t, 1, len(handler.headers))
		assert.Equal(t, "foo", handler.headers[0].Name)
		assert.Equal(t, "bar", handler.headers[0].Value)
	})

	t.Run("UnblockedStreamImmuneToBufferLimit", func(t *testing.T) {
		// Non-blocked stream (RIC=0) with maxBufferedData=10
		f := newDecoderTestFixture(t, FragmentModeSingleChunk, 1024, 1)
		handler := newMockHeadersHandler()
		decoder := f.qpackDecoder.CreateProgressiveDecoderWithMaxBufferedData(1, 10, handler)

		// Feed prefix with RIC=0 (unblocked) + 30 static headers (30 bytes > 10)
		var wire bytes.Buffer
		wire.Write([]byte{0x00, 0x00}) // RIC=0, Base=0
		for i := 0; i < 30; i++ {
			wire.WriteByte(0xd1) // :method: GET
		}

		decoder.Decode(wire.Bytes())
		decoder.EndHeaderBlock()

		assert.False(t, handler.errorDetected)
		assert.True(t, handler.decodingCompleted)
		assert.Equal(t, 30, len(handler.headers))
	})
}

// -----------------------------------------------------------------------------
// 9. Relative Index Bounds
// -----------------------------------------------------------------------------

func TestM5DecoderAdversarial_RelativeIndexBounds_RequestStream(t *testing.T) {
	// RFC 9204 §3.2.4 & §4.5.2: In request stream, Relative Index R converts to
	// Absolute Index = Base - R - 1. If R >= Base, calculation fails -> "Invalid relative index."
	testCases := []struct {
		name       string
		ric        uint64
		base       uint64
		fieldBytes []byte
	}{
		{
			name: "IndexedDynamic_Base0_Rel0",
			// RIC=1, Base=0 (DeltaBase=0, S=1) -> prefix 0280
			// Indexed dynamic opcode: 0x80 (rel index 0). Rel 0 >= Base 0 -> invalid
			ric:        1,
			base:       0,
			fieldBytes: []byte{0x80},
		},
		{
			name: "IndexedDynamic_Base1_Rel1",
			// Base=1, Rel 1 >= Base 1 -> invalid (rel index 1 is opcode 0x81)
			ric:        2,
			base:       1,
			fieldBytes: []byte{0x81},
		},
		{
			name: "IndexedDynamic_Base4_Rel4",
			// Base=4, Rel 4 >= Base 4 -> opcode 0x84
			ric:        5,
			base:       4,
			fieldBytes: []byte{0x84},
		},
		{
			name: "IndexedDynamic_Base4_Rel100",
			// Base=4, Rel 100 >= Base 4 -> opcode 0xbf, 0x25
			ric:        5,
			base:       4,
			fieldBytes: encodeVarintAdversarial(0x80, 6, 100),
		},
		{
			name: "LiteralNameRef_Base0_Rel0",
			// Literal with dynamic name ref: opcode 0x40 (rel index 0), val "v"
			ric:        1,
			base:       0,
			fieldBytes: decodeHexOrPanic("400176"),
		},
		{
			name: "LiteralNameRef_Base2_Rel2",
			// Literal with dynamic name ref: opcode 0x42 (rel index 2), val "v"
			ric:        3,
			base:       2,
			fieldBytes: decodeHexOrPanic("420176"),
		},
		{
			name: "LiteralNameRef_Base2_Rel50",
			// Literal with dynamic name ref: rel index 50, val "v"
			ric:        3,
			base:       2,
			fieldBytes: append(encodeVarintAdversarial(0x40, 4, 50), decodeHexOrPanic("0176")...),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			runProgressiveDecoderTests(t, 1024, 1, 0, func(t *testing.T, f *progressiveDecoderTestFixture) {
				f.headerTable.SetDynamicTableCapacity(1024)
				for i := uint64(0); i < tc.ric; i++ {
					f.headerTable.InsertEntry("k", "v")
				}

				// Build prefix: DeltaBase and Sign such that Base matches tc.base
				// If Base <= RIC - 1: Sign=1, DeltaBase = RIC - Base - 1
				// If Base >= RIC: Sign=0, DeltaBase = Base - RIC
				var sign bool
				var deltaBase uint64
				if tc.base < tc.ric {
					sign = true
					deltaBase = tc.ric - tc.base - 1
				} else {
					sign = false
					deltaBase = tc.base - tc.ric
				}

				encodedRIC := EncodeRequiredInsertCount(tc.ric, f.headerTable.MaxEntries())
				prefix := encodeHeaderPrefixAdversarial(encodedRIC, sign, deltaBase)

				wire := append(prefix, tc.fieldBytes...)
				f.DecodeHeaderBlock(wire)

				assert.Truef(t, f.handler.errorDetected, "error should be detected for %s", tc.name)
				assert.Equal(t, uint64(QPACK_DECOMPRESSION_FAILED), f.handler.errorCode)
				assert.Equal(t, "Invalid relative index.", f.handler.errorMessage)
			})
		})
	}
}

func TestM5DecoderAdversarial_RelativeIndexBounds_EncoderStream(t *testing.T) {
	t.Run("InsertWithNameRef_EmptyTable_Rel0", func(t *testing.T) {
		f := newDecoderTestFixture(t, FragmentModeSingleChunk, 1024, 1)
		// InsertWithNameRef dynamic (T=0): nameIndex=0, val="v"
		// InsertedEntryCount is 0. nameIndex 0 >= 0 -> Invalid relative index.
		f.DecodeEncoderStreamData(decodeHexOrPanic("800176"))
		require.Equal(t, 1, len(f.encoderStreamErrorDelegate.calls))
		assert.Equal(
			t,
			uint64(QUIC_QPACK_ENCODER_STREAM_INSERTION_INVALID_RELATIVE_INDEX),
			f.encoderStreamErrorDelegate.calls[0].ErrorCode,
		)
		assert.Equal(t, "Invalid relative index.", f.encoderStreamErrorDelegate.calls[0].ErrorMessage)
	})

	t.Run("InsertWithNameRef_TableCount2_Rel2", func(t *testing.T) {
		f := newDecoderTestFixture(t, FragmentModeSingleChunk, 1024, 1)
		// Insert 2 literal entries into table
		f.DecodeEncoderStreamData(decodeHexOrPanic("3fe1076294e703626172")) // entry 1
		f.DecodeEncoderStreamData(decodeHexOrPanic("43666f6f03626172"))     // entry 2
		require.Equal(t, uint64(2), f.qpackDecoder.HeaderTable().InsertedEntryCount())

		// Now reference relative index 2 (>= 2)
		f.DecodeEncoderStreamData(decodeHexOrPanic("820176"))
		require.Equal(t, 1, len(f.encoderStreamErrorDelegate.calls))
		assert.Equal(
			t,
			uint64(QUIC_QPACK_ENCODER_STREAM_INSERTION_INVALID_RELATIVE_INDEX),
			f.encoderStreamErrorDelegate.calls[0].ErrorCode,
		)
		assert.Equal(t, "Invalid relative index.", f.encoderStreamErrorDelegate.calls[0].ErrorMessage)
	})

	t.Run("Duplicate_EmptyTable_Index0", func(t *testing.T) {
		f := newDecoderTestFixture(t, FragmentModeSingleChunk, 1024, 1)
		// Duplicate index 0 on empty dynamic table
		f.DecodeEncoderStreamData(decodeHexOrPanic("00"))
		require.Equal(t, 1, len(f.encoderStreamErrorDelegate.calls))
		assert.Equal(
			t,
			uint64(QUIC_QPACK_ENCODER_STREAM_DUPLICATE_INVALID_RELATIVE_INDEX),
			f.encoderStreamErrorDelegate.calls[0].ErrorCode,
		)
		assert.Equal(t, "Invalid relative index.", f.encoderStreamErrorDelegate.calls[0].ErrorMessage)
	})

	t.Run("Duplicate_TableCount1_Index1", func(t *testing.T) {
		f := newDecoderTestFixture(t, FragmentModeSingleChunk, 1024, 1)
		f.DecodeEncoderStreamData(decodeHexOrPanic("3fe1076294e703626172")) // entry 1
		require.Equal(t, uint64(1), f.qpackDecoder.HeaderTable().InsertedEntryCount())

		// Duplicate index 1 (>= 1)
		f.DecodeEncoderStreamData(decodeHexOrPanic("01"))
		require.Equal(t, 1, len(f.encoderStreamErrorDelegate.calls))
		assert.Equal(
			t,
			uint64(QUIC_QPACK_ENCODER_STREAM_DUPLICATE_INVALID_RELATIVE_INDEX),
			f.encoderStreamErrorDelegate.calls[0].ErrorCode,
		)
		assert.Equal(t, "Invalid relative index.", f.encoderStreamErrorDelegate.calls[0].ErrorMessage)
	})
}

// -----------------------------------------------------------------------------
// 10. Additional Adversarial Bounds & Eviction Tests
// -----------------------------------------------------------------------------

func TestM5DecoderAdversarial_AbsoluteIndexExceedsRIC(t *testing.T) {
	// RFC 9204 §4.5.2 & §4.5.3: An absolute index must be smaller than Required Insert Count.
	t.Run("RelativeIndex_AbsoluteIndexEqualsRIC", func(t *testing.T) {
		runProgressiveDecoderTests(t, 1024, 1, 0, func(t *testing.T, f *progressiveDecoderTestFixture) {
			f.headerTable.SetDynamicTableCapacity(1024)
			f.headerTable.InsertEntry("k1", "v1")
			f.headerTable.InsertEntry("k2", "v2")

			// Base = 2, RIC = 1 (encoded 2), DeltaBase = 0, Sign = 0 -> Base = 1 + 0 = 1?
			// To get Base = 2 with RIC = 1: Base = RIC + DeltaBase -> DeltaBase = 1 (Sign=0)
			// Relative Index 0 -> Absolute Index = 2 - 0 - 1 = 1.
			// Absolute Index (1) >= RIC (1) -> violates specification!
			prefix := encodeHeaderPrefixAdversarial(2, false, 1)
			f.DecodeHeaderBlock(append(prefix, 0x80)) // relative index 0

			assert.True(t, f.handler.errorDetected)
			assert.Equal(t, uint64(QPACK_DECOMPRESSION_FAILED), f.handler.errorCode)
			assert.Equal(t, "Absolute Index must be smaller than Required Insert Count.", f.handler.errorMessage)
		})
	})

	t.Run("PostBaseIndex_AbsoluteIndexEqualsRIC", func(t *testing.T) {
		runProgressiveDecoderTests(t, 1024, 1, 0, func(t *testing.T, f *progressiveDecoderTestFixture) {
			f.headerTable.SetDynamicTableCapacity(1024)
			f.headerTable.InsertEntry("k1", "v1")
			f.headerTable.InsertEntry("k2", "v2")

			// RIC = 1 (encoded 2), Base = 1 (DeltaBase = 0, S=0)
			// Post-base index 0 -> Absolute Index = Base + 0 = 1.
			// Absolute Index (1) >= RIC (1) -> violates specification!
			prefix := encodeHeaderPrefixAdversarial(2, false, 0)
			f.DecodeHeaderBlock(append(prefix, 0x10)) // post-base index 0

			assert.True(t, f.handler.errorDetected)
			assert.Equal(t, uint64(QPACK_DECOMPRESSION_FAILED), f.handler.errorCode)
			assert.Equal(t, "Absolute Index must be smaller than Required Insert Count.", f.handler.errorMessage)
		})
	})
}

func TestM5DecoderAdversarial_DynamicEntryAlreadyEvicted(t *testing.T) {
	runProgressiveDecoderTests(t, 1024, 1, 0, func(t *testing.T, f *progressiveDecoderTestFixture) {
		f.headerTable.SetDynamicTableCapacity(1024)
		// Insert entry 0: size = 3 + 3 + 32 = 38
		f.headerTable.InsertEntry("foo", "bar")
		// Insert entry 1: size = 3 + 3 + 32 = 38
		f.headerTable.InsertEntry("baz", "qux")

		// Shrink dynamic table capacity to 40 bytes -> entry 0 is evicted!
		f.headerTable.SetDynamicTableCapacity(40)
		require.Equal(t, uint64(1), f.headerTable.DroppedEntryCount())

		// Now decode header block referencing absolute index 0 (which was evicted)
		// Base = 2, RIC = 2 (encoded 3). Rel index 1 -> Abs = 2 - 1 - 1 = 0
		prefix := encodeHeaderPrefixAdversarial(3, false, 0) // Base = 2 + 0 = 2
		f.DecodeHeaderBlock(append(prefix, 0x81))            // relative index 1

		assert.True(t, f.handler.errorDetected)
		assert.Equal(t, uint64(QPACK_DECOMPRESSION_FAILED), f.handler.errorCode)
		assert.Equal(t, "Dynamic table entry already evicted.", f.handler.errorMessage)
	})
}

func TestM5DecoderAdversarial_StaticTableBounds(t *testing.T) {
	// Static table has 99 entries (indices 0..98)
	t.Run("RequestStream_IndexedStaticOutOfBounds99", func(t *testing.T) {
		runProgressiveDecoderTests(t, 1024, 1, 0, func(t *testing.T, f *progressiveDecoderTestFixture) {
			// Prefix 0000, 1 1 Index(99): 0xc0 | (99 & 0x3f) = 0xc0 | 63 = 0xff, extension 99 - 63 = 36 = 0x24
			f.DecodeHeaderBlock(decodeHexOrPanic("0000ff24"))
			assert.True(t, f.handler.errorDetected)
			assert.Equal(t, "Static table entry not found.", f.handler.errorMessage)
		})
	})

	t.Run("RequestStream_LiteralStaticNameRefOutOfBounds200", func(t *testing.T) {
		runProgressiveDecoderTests(t, 1024, 1, 0, func(t *testing.T, f *progressiveDecoderTestFixture) {
			// Prefix 0000, 0101 (T=1 static) index 200, val "v"
			wire := append([]byte{0x00, 0x00}, encodeVarintAdversarial(0x50, 4, 200)...)
			wire = append(wire, decodeHexOrPanic("0176")...)
			f.DecodeHeaderBlock(wire)
			assert.True(t, f.handler.errorDetected)
			assert.Equal(t, "Static table entry not found.", f.handler.errorMessage)
		})
	})

	t.Run("EncoderStream_InsertWithNameRefStaticOutOfBounds99", func(t *testing.T) {
		f := newDecoderTestFixture(t, FragmentModeSingleChunk, 1024, 1)
		// 1 T(1) NNNNNN(99) = 0xff, 0x24, val "foo" (0x03, 'f', 'o', 'o')
		f.DecodeEncoderStreamData(decodeHexOrPanic("ff2403666f6f"))
		require.Equal(t, 1, len(f.encoderStreamErrorDelegate.calls))
		assert.Equal(
			t,
			uint64(QUIC_QPACK_ENCODER_STREAM_INVALID_STATIC_ENTRY),
			f.encoderStreamErrorDelegate.calls[0].ErrorCode,
		)
		assert.Equal(t, "Invalid static table entry.", f.encoderStreamErrorDelegate.calls[0].ErrorMessage)
	})
}

func TestM5DecoderAdversarial_RICValidation_TooLarge(t *testing.T) {
	runProgressiveDecoderTests(t, 1024, 1, 0, func(t *testing.T, f *progressiveDecoderTestFixture) {
		f.headerTable.SetDynamicTableCapacity(1024)
		f.headerTable.InsertEntry("k1", "v1")
		f.headerTable.InsertEntry("k2", "v2")
		f.headerTable.InsertEntry("k3", "v3")

		// Declares RIC = 3 (encoded 4), but only references static table (RICSoFar = 0)
		f.DecodeHeaderBlock(decodeHexOrPanic("0400d1"))
		assert.True(t, f.handler.errorDetected)
		assert.Equal(t, "Required Insert Count too large.", f.handler.errorMessage)
	})
}

func TestM5DecoderAdversarial_DecoderFeedbackEmissions(t *testing.T) {
	t.Run("SectionAckAndInsertCountIncrement", func(t *testing.T) {
		f := newDecoderTestFixture(t, FragmentModeSingleChunk, 1024, 1)

		// Insert 2 entries on encoder stream
		f.DecodeEncoderStreamData(decodeHexOrPanic("3fe1076294e70362617280035a5a5a"))
		require.Equal(t, uint64(2), f.qpackDecoder.HeaderTable().InsertedEntryCount())

		// Decode header block referencing dynamic entry 0 (RIC=1)
		// RIC=1 (encoded 2), Base=1 -> prefix 0200, relative index 0 -> 0x80
		f.StartDecoding()
		f.DecodeData(decodeHexOrPanic("020080"))
		f.EndDecoding()

		assert.False(t, f.handler.errorDetected)
		assert.True(t, f.handler.decodingCompleted)

		f.FlushDecoderStream()

		// Verification of decoder feedback emissions:
		// 1. Header Acknowledgement for Stream 1: opcode 0x80 | 1 = 0x81
		// 2. Insert Count Increment for delta (InsertedEntryCount(2) - knownReceivedCount(1) = 1):
		//    opcode 0x00 | 1 = 0x01
		written := f.WrittenData()
		assert.Equal(t, decodeHexOrPanic("8101"), written)
		assert.Equal(t, uint64(2), f.qpackDecoder.KnownReceivedCount())
	})

	t.Run("StreamCancellationWithCapacity", func(t *testing.T) {
		f := newDecoderTestFixture(t, FragmentModeSingleChunk, 1024, 1)
		// With dynamic table capacity > 0, stream reset must emit stream cancellation
		f.qpackDecoder.OnStreamReset(5)
		f.qpackDecoder.FlushDecoderStream()

		// Stream Cancellation (01 SSSSSS): 0x40 | 5 = 0x45
		assert.Equal(t, decodeHexOrPanic("45"), f.WrittenData())
	})

	t.Run("StreamCancellationWithoutCapacityIgnored", func(t *testing.T) {
		f := newDecoderTestFixture(t, FragmentModeSingleChunk, 0, 1)
		// With dynamic table capacity == 0, stream reset is a no-op
		f.qpackDecoder.OnStreamReset(5)
		f.qpackDecoder.FlushDecoderStream()

		assert.Equal(t, 0, len(f.WrittenData()))
	})
}

// -----------------------------------------------------------------------------
// 11. Adversarial Dynamic Table Lifecycle, Evictions & Observer Resumption
// -----------------------------------------------------------------------------

func TestM5DecoderAdversarial_EncoderStream_EvictedDynamicEntryReferences(t *testing.T) {
	t.Run("InsertWithNameRef_EvictedEntry", func(t *testing.T) {
		f := newDecoderTestFixture(t, FragmentModeSingleChunk, 1024, 1)
		// Set capacity 1024
		f.DecodeEncoderStreamData(decodeHexOrPanic("3fe107"))
		// Insert entry 0: "foo": "bar" (size 38)
		f.DecodeEncoderStreamData(decodeHexOrPanic("43666f6f03626172"))
		// Insert entry 1: "baz": "qux" (size 38)
		f.DecodeEncoderStreamData(decodeHexOrPanic("4362617a03717578"))
		require.Equal(t, uint64(2), f.qpackDecoder.HeaderTable().InsertedEntryCount())

		// Shrink capacity to 40 bytes -> entry 0 is evicted
		f.DecodeEncoderStreamData(decodeHexOrPanic("3f09"))
		require.Equal(t, uint64(1), f.qpackDecoder.HeaderTable().DroppedEntryCount())

		// Reference relative index 1 (maps to absolute index 2 - 1 - 1 = 0, which is evicted)
		f.DecodeEncoderStreamData(decodeHexOrPanic("810176"))
		require.Equal(t, 1, len(f.encoderStreamErrorDelegate.calls))
		assert.Equal(
			t,
			uint64(QUIC_QPACK_ENCODER_STREAM_INSERTION_DYNAMIC_ENTRY_NOT_FOUND),
			f.encoderStreamErrorDelegate.calls[0].ErrorCode,
		)
		assert.Equal(t, "Dynamic table entry not found.", f.encoderStreamErrorDelegate.calls[0].ErrorMessage)
	})

	t.Run("Duplicate_EvictedEntry", func(t *testing.T) {
		f := newDecoderTestFixture(t, FragmentModeSingleChunk, 1024, 1)
		// Set capacity 1024
		f.DecodeEncoderStreamData(decodeHexOrPanic("3fe107"))
		// Insert entry 0: "foo": "bar" (size 38)
		f.DecodeEncoderStreamData(decodeHexOrPanic("43666f6f03626172"))
		// Insert entry 1: "baz": "qux" (size 38)
		f.DecodeEncoderStreamData(decodeHexOrPanic("4362617a03717578"))
		require.Equal(t, uint64(2), f.qpackDecoder.HeaderTable().InsertedEntryCount())

		// Shrink capacity to 40 bytes -> entry 0 is evicted
		f.DecodeEncoderStreamData(decodeHexOrPanic("3f09"))
		require.Equal(t, uint64(1), f.qpackDecoder.HeaderTable().DroppedEntryCount())

		// Duplicate relative index 1 (maps to absolute index 0, which is evicted)
		f.DecodeEncoderStreamData(decodeHexOrPanic("01"))
		require.Equal(t, 1, len(f.encoderStreamErrorDelegate.calls))
		assert.Equal(
			t,
			uint64(QUIC_QPACK_ENCODER_STREAM_DUPLICATE_DYNAMIC_ENTRY_NOT_FOUND),
			f.encoderStreamErrorDelegate.calls[0].ErrorCode,
		)
		assert.Equal(t, "Dynamic table entry not found.", f.encoderStreamErrorDelegate.calls[0].ErrorMessage)
	})

	t.Run("CapacityExceedsMaximumCapacity", func(t *testing.T) {
		// MaximumDynamicTableCapacity = 100
		f := newDecoderTestFixture(t, FragmentModeSingleChunk, 100, 1)
		// Attempt to set capacity 101: opcode 0x20 | 0x1f = 0x3f, extension 101 - 31 = 70 = 0x46
		f.DecodeEncoderStreamData(decodeHexOrPanic("3f46"))
		require.Equal(t, 1, len(f.encoderStreamErrorDelegate.calls))
		assert.Equal(
			t,
			uint64(QUIC_QPACK_ENCODER_STREAM_SET_DYNAMIC_TABLE_CAPACITY),
			f.encoderStreamErrorDelegate.calls[0].ErrorCode,
		)
		assert.Equal(t, "Error updating dynamic table capacity.", f.encoderStreamErrorDelegate.calls[0].ErrorMessage)
	})
}

func TestM5DecoderAdversarial_BlockedStream_BufferedReplayError(t *testing.T) {
	f := newDecoderTestFixture(t, FragmentModeSingleChunk, 1024, 1)
	handler := newMockHeadersHandler()
	decoder := f.qpackDecoder.CreateProgressiveDecoder(1, handler)

	// Feed prefix declaring RIC=1 (encoded 2), Base=1 -> table is empty, so stream blocks
	decoder.Decode(decodeHexOrPanic("0200"))
	assert.False(t, handler.errorDetected)

	// Feed an invalid instruction while blocked: Static table index 99 out of bounds (0xff, 0x24)
	decoder.Decode(decodeHexOrPanic("ff24"))
	assert.False(t, handler.errorDetected)
	decoder.EndHeaderBlock()
	assert.False(t, handler.decodingCompleted)

	// Now unblock stream by inserting entry on encoder stream
	f.DecodeEncoderStreamData(decodeHexOrPanic("3fe1076294e703626172"))

	// Replay of buffered instruction must trigger fatal error
	assert.True(t, handler.errorDetected)
	assert.Equal(t, uint64(QPACK_DECOMPRESSION_FAILED), handler.errorCode)
	assert.Equal(t, "Static table entry not found.", handler.errorMessage)
	assert.False(t, handler.decodingCompleted)
}

func TestM5DecoderAdversarial_BlockedStream_CloseDefensivelyCleansObserver(t *testing.T) {
	f := newDecoderTestFixture(t, FragmentModeSingleChunk, 1024, 1)
	handler := newMockHeadersHandler()
	decoder := f.qpackDecoder.CreateProgressiveDecoder(1, handler)

	// Feed prefix declaring RIC=1 -> stream blocks
	decoder.Decode(decodeHexOrPanic("0200"))
	assert.False(t, handler.errorDetected)

	// Defensively close stream before unblocking
	decoder.Close()

	// Peer provides dynamic entry on encoder stream -> observer was cleanly removed, no crash
	f.DecodeEncoderStreamData(decodeHexOrPanic("3fe1076294e703626172"))

	assert.False(t, handler.errorDetected)
	assert.False(t, handler.decodingCompleted)
}

func TestM5DecoderAdversarial_ProgressiveDecoder_MultipleBlockedStreamsStaggeredRICs(t *testing.T) {
	f := newDecoderTestFixture(t, FragmentModeSingleChunk, 1024, 3)

	// Stream 1 blocks on RIC=1: prefix 0200, dynamic rel index 0 (0x80)
	h1 := newMockHeadersHandler()
	d1 := f.qpackDecoder.CreateProgressiveDecoder(1, h1)
	d1.Decode(decodeHexOrPanic("020080"))
	d1.EndHeaderBlock()
	assert.False(t, h1.decodingCompleted)

	// Stream 2 blocks on RIC=2: prefix 0300 (RIC=2, Base=2), dynamic rel index 0 (0x80 -> abs 1)
	h2 := newMockHeadersHandler()
	d2 := f.qpackDecoder.CreateProgressiveDecoder(2, h2)
	d2.Decode(decodeHexOrPanic("030080"))
	d2.EndHeaderBlock()
	assert.False(t, h2.decodingCompleted)

	// Stream 3 blocks on RIC=3: prefix 0400 (RIC=3, Base=3), dynamic rel index 0 (0x80 -> abs 2)
	h3 := newMockHeadersHandler()
	d3 := f.qpackDecoder.CreateProgressiveDecoder(3, h3)
	d3.Decode(decodeHexOrPanic("040080"))
	d3.EndHeaderBlock()
	assert.False(t, h3.decodingCompleted)

	// Insert entry 0: "k0": "v0"
	f.DecodeEncoderStreamData(decodeHexOrPanic("3fe107426b30027630"))
	// Stream 1 unblocks!
	assert.True(t, h1.decodingCompleted)
	assert.False(t, h2.decodingCompleted)
	assert.False(t, h3.decodingCompleted)
	require.Equal(t, 1, len(h1.headers))
	assert.Equal(t, "k0", h1.headers[0].Name)

	// Insert entry 1: "k1": "v1"
	f.DecodeEncoderStreamData(decodeHexOrPanic("426b31027631"))
	// Stream 2 unblocks!
	assert.True(t, h2.decodingCompleted)
	assert.False(t, h3.decodingCompleted)
	require.Equal(t, 1, len(h2.headers))
	assert.Equal(t, "k1", h2.headers[0].Name)

	// Insert entry 2: "k2": "v2"
	f.DecodeEncoderStreamData(decodeHexOrPanic("426b32027632"))
	// Stream 3 unblocks!
	assert.True(t, h3.decodingCompleted)
	require.Equal(t, 1, len(h3.headers))
	assert.Equal(t, "k2", h3.headers[0].Name)
}

func TestM5DecoderAdversarial_ProgressiveDecoder_EmptyNameAndValueLiteral(t *testing.T) {
	runProgressiveDecoderTests(t, 1024, 1, 0, func(t *testing.T, f *progressiveDecoderTestFixture) {
		// Prefix: RIC=0, Base=0 (0x00, 0x00)
		// Literal without name ref: name len 0 (0x20), value len 0 (0x00)
		f.DecodeHeaderBlock(decodeHexOrPanic("00002000"))

		assert.False(t, f.handler.errorDetected)
		assert.True(t, f.handler.decodingCompleted)
		require.Equal(t, 1, len(f.handler.headers))
		assert.Equal(t, "", f.handler.headers[0].Name)
		assert.Equal(t, "", f.handler.headers[0].Value)
	})
}

func TestM5DecoderAdversarial_StreamReset_BlockedStreamQuotaRestored(t *testing.T) {
	// RFC 9204 §2.1.2: "A stream can also be unblocked if the stream is reset."
	// Maximum blocked streams = 1
	f := newDecoderTestFixture(t, FragmentModeSingleChunk, 1024, 1)

	// Stream 1 arrives and blocks on RIC=1
	h1 := newMockHeadersHandler()
	d1 := f.qpackDecoder.CreateProgressiveDecoder(1, h1)
	d1.Decode(decodeHexOrPanic("020080"))
	assert.False(t, h1.errorDetected)
	assert.False(t, h1.decodingCompleted)

	// Stream 1 is reset and closed
	f.qpackDecoder.OnStreamReset(1)
	d1.Close()

	// Stream 2 arrives and blocks on RIC=1
	// Since Stream 1 was reset and closed, Stream 2 must be allowed to block (quota = 1)
	h2 := newMockHeadersHandler()
	d2 := f.qpackDecoder.CreateProgressiveDecoder(2, h2)
	d2.Decode(decodeHexOrPanic("020080"))

	assert.Falsef(
		t,
		h2.errorDetected,
		"Stream 2 must be allowed to block after Stream 1 reset; error: %s",
		h2.errorMessage,
	)
	assert.False(t, h2.decodingCompleted)
}
