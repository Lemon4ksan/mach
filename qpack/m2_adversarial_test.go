// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package qpack

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"math"
	"math/rand"
	"strings"
	"testing"

	"github.com/lemon4ksan/foundation/testing/assert"
	"github.com/lemon4ksan/foundation/testing/require"
)

// adversarialTestDelegate captures decoded instructions and errors for validation.
type adversarialTestDelegate struct {
	decodedInstructions []*Instruction
	decodedValues       []decodedInstructionRecord
	errorCalls          []decoderErrorRecord

	onDecodedCallback func(instruction *Instruction, decoder *InstructionDecoder) bool
	onErrorCallback   func(errorCode InstructionDecoderErrorCode, message string)

	decoderRef *InstructionDecoder
}

type decodedInstructionRecord struct {
	Instruction *Instruction
	SBit        bool
	Varint      uint64
	Varint2     uint64
	Name        string
	Value       string
}

type decoderErrorRecord struct {
	ErrorCode InstructionDecoderErrorCode
	Message   string
}

func newAdversarialTestDelegate() *adversarialTestDelegate {
	return &adversarialTestDelegate{}
}

func (d *adversarialTestDelegate) OnInstructionDecoded(instruction *Instruction) bool {
	d.decodedInstructions = append(d.decodedInstructions, instruction)
	if d.decoderRef != nil {
		d.decodedValues = append(d.decodedValues, decodedInstructionRecord{
			Instruction: instruction,
			SBit:        d.decoderRef.SBit(),
			Varint:      d.decoderRef.Varint(),
			Varint2:     d.decoderRef.Varint2(),
			Name:        d.decoderRef.Name(),
			Value:       d.decoderRef.Value(),
		})
	}
	if d.onDecodedCallback != nil {
		return d.onDecodedCallback(instruction, d.decoderRef)
	}
	return true
}

func (d *adversarialTestDelegate) OnInstructionDecodingError(
	errorCode InstructionDecoderErrorCode,
	errorMessage string,
) {
	d.errorCalls = append(d.errorCalls, decoderErrorRecord{
		ErrorCode: errorCode,
		Message:   errorMessage,
	})
	if d.onErrorCallback != nil {
		d.onErrorCallback(errorCode, errorMessage)
	}
}

// -----------------------------------------------------------------------------
// 1. Random Chunk Fragmentation
// -----------------------------------------------------------------------------

func TestM2Adversarial_RandomChunkFragmentation(t *testing.T) {
	encoder := NewInstructionEncoder(HuffmanEncodingEnabled)

	// Define a diverse sequence of instructions across languages.
	type instructionCase struct {
		name     string
		language *Language
		create   func() *InstructionWithValues
	}

	testCases := []instructionCase{
		{
			name:     "InsertWithNameReference_Static",
			language: EncoderStreamLanguage(),
			create: func() *InstructionWithValues {
				return InstructionWithValuesInsertWithNameReference(true, 42, "custom-header-value")
			},
		},
		{
			name:     "InsertWithNameReference_Dynamic",
			language: EncoderStreamLanguage(),
			create: func() *InstructionWithValues {
				return InstructionWithValuesInsertWithNameReference(false, 1024, "dynamic-val-xyz")
			},
		},
		{
			name:     "InsertWithoutNameReference_Huffman",
			language: EncoderStreamLanguage(),
			create: func() *InstructionWithValues {
				return InstructionWithValuesInsertWithoutNameReference("custom-header-name", "custom-header-val")
			},
		},
		{
			name:     "Duplicate",
			language: EncoderStreamLanguage(),
			create: func() *InstructionWithValues {
				return InstructionWithValuesDuplicate(9999)
			},
		},
		{
			name:     "SetDynamicTableCapacity",
			language: EncoderStreamLanguage(),
			create: func() *InstructionWithValues {
				return InstructionWithValuesSetDynamicTableCapacity(65536)
			},
		},
		{
			name:     "HeaderAcknowledgement",
			language: DecoderStreamLanguage(),
			create: func() *InstructionWithValues {
				return InstructionWithValuesHeaderAcknowledgement(123456)
			},
		},
		{
			name:     "StreamCancellation",
			language: DecoderStreamLanguage(),
			create: func() *InstructionWithValues {
				return InstructionWithValuesStreamCancellation(789)
			},
		},
		{
			name:     "InsertCountIncrement",
			language: DecoderStreamLanguage(),
			create: func() *InstructionWithValues {
				return InstructionWithValuesInsertCountIncrement(42)
			},
		},
		{
			name:     "Prefix_WithSBitTrue",
			language: PrefixLanguage(),
			create: func() *InstructionWithValues {
				inst := InstructionWithValuesPrefix(500)
				inst.SetSBit(true)
				inst.SetVarint2(12)
				return inst
			},
		},
		{
			name:     "Prefix_WithSBitFalse",
			language: PrefixLanguage(),
			create: func() *InstructionWithValues {
				inst := InstructionWithValuesPrefix(10000)
				inst.SetSBit(false)
				inst.SetVarint2(99)
				return inst
			},
		},
		{
			name:     "IndexedHeaderField",
			language: RequestStreamLanguage(),
			create: func() *InstructionWithValues {
				return InstructionWithValuesIndexedHeaderField(true, 55)
			},
		},
		{
			name:     "LiteralHeaderFieldNameReference",
			language: RequestStreamLanguage(),
			create: func() *InstructionWithValues {
				return InstructionWithValuesLiteralHeaderFieldNameReference(false, 30, "referenced-name-value")
			},
		},
		{
			name:     "LiteralHeaderField",
			language: RequestStreamLanguage(),
			create: func() *InstructionWithValues {
				return InstructionWithValuesLiteralHeaderField(":authority", "example.com:443")
			},
		},
		{
			name:     "IndexedHeaderFieldPostBase",
			language: RequestStreamLanguage(),
			create: func() *InstructionWithValues {
				return InstructionWithValuesIndexedHeaderFieldPostBase(77)
			},
		},
		{
			name:     "LiteralHeaderFieldPostBaseNameReference",
			language: RequestStreamLanguage(),
			create: func() *InstructionWithValues {
				return InstructionWithValuesLiteralHeaderFieldPostBaseNameReference(15, "post-base-name-val")
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			instWithValues := tc.create()
			encoded := encoder.Encode(instWithValues, nil)
			require.True(t, len(encoded) > 0)

			// Test 1: Byte-by-byte (1-byte chunk) fragmentation with zero-byte chunks injected
			t.Run("OctetByOctetWithZeroBytes", func(t *testing.T) {
				delegate := newAdversarialTestDelegate()
				decoder := NewInstructionDecoder(tc.language, delegate)
				delegate.decoderRef = decoder

				for i := 0; i < len(encoded); i++ {
					// Inject empty chunks before and after real byte
					assert.True(t, decoder.Decode([]byte{}))
					assert.True(t, decoder.Decode(nil))
					success := decoder.Decode(encoded[i : i+1])
					require.True(t, success)
				}
				decoder.EndDecoding()

				assert.False(t, decoder.HasError())
				assert.True(t, decoder.AtInstructionBoundary())
				require.Equal(t, 1, len(delegate.decodedInstructions))
				require.Equal(t, 0, len(delegate.errorCalls))

				// Verify decoded values match
				rec := delegate.decodedValues[0]
				assert.Equal(t, instWithValues.SBit(), rec.SBit)
				assert.Equal(t, instWithValues.Varint(), rec.Varint)
				assert.Equal(t, instWithValues.Varint2(), rec.Varint2)
				assert.Equal(t, instWithValues.Name(), rec.Name)
				assert.Equal(t, instWithValues.Value(), rec.Value)
			})

			// Test 2: Randomized chunk sizes across multiple PRNG seeds
			for seed := int64(1); seed <= 5; seed++ {
				t.Run(fmt.Sprintf("RandomChunkSeed_%d", seed), func(t *testing.T) {
					rng := rand.New(rand.NewSource(seed))
					delegate := newAdversarialTestDelegate()
					decoder := NewInstructionDecoder(tc.language, delegate)
					delegate.decoderRef = decoder

					offset := 0
					for offset < len(encoded) {
						// Random chunk size from 1 to 4 bytes
						chunkSize := rng.Intn(4) + 1
						if offset+chunkSize > len(encoded) {
							chunkSize = len(encoded) - offset
						}

						// Occasionally inject empty slice
						if rng.Intn(3) == 0 {
							require.True(t, decoder.Decode([]byte{}))
						}

						chunk := encoded[offset : offset+chunkSize]
						offset += chunkSize

						success := decoder.Decode(chunk)
						require.True(t, success)
					}
					decoder.EndDecoding()

					assert.False(t, decoder.HasError())
					assert.True(t, decoder.AtInstructionBoundary())
					require.Equal(t, 1, len(delegate.decodedInstructions))
					require.Equal(t, 0, len(delegate.errorCalls))

					rec := delegate.decodedValues[0]
					assert.Equal(t, instWithValues.SBit(), rec.SBit)
					assert.Equal(t, instWithValues.Varint(), rec.Varint)
					assert.Equal(t, instWithValues.Varint2(), rec.Varint2)
					assert.Equal(t, instWithValues.Name(), rec.Name)
					assert.Equal(t, instWithValues.Value(), rec.Value)
				})
			}
		})
	}

	// Test 3: Multiple back-to-back instructions in a continuous stream with random fragmentation
	t.Run("MultiInstructionContinuousStream", func(t *testing.T) {
		var combinedStream []byte
		var expectedInstructions []*InstructionWithValues

		// Build a stream of 60 instructions on the EncoderStream
		for i := 0; i < 20; i++ {
			inst1 := InstructionWithValuesInsertWithNameReference(i%2 == 0, uint64(i*10), fmt.Sprintf("val-%d", i))
			inst2 := InstructionWithValuesInsertWithoutNameReference(
				fmt.Sprintf("key-%d", i),
				fmt.Sprintf("val-without-ref-%d", i),
			)
			inst3 := InstructionWithValuesDuplicate(uint64(i + 1))

			combinedStream = encoder.Encode(inst1, combinedStream)
			combinedStream = encoder.Encode(inst2, combinedStream)
			combinedStream = encoder.Encode(inst3, combinedStream)

			expectedInstructions = append(expectedInstructions, inst1, inst2, inst3)
		}

		for seed := int64(100); seed <= 103; seed++ {
			rng := rand.New(rand.NewSource(seed))
			delegate := newAdversarialTestDelegate()
			decoder := NewInstructionDecoder(EncoderStreamLanguage(), delegate)
			delegate.decoderRef = decoder

			offset := 0
			for offset < len(combinedStream) {
				chunkSize := rng.Intn(7) + 1
				if offset+chunkSize > len(combinedStream) {
					chunkSize = len(combinedStream) - offset
				}

				if rng.Intn(4) == 0 {
					require.True(t, decoder.Decode([]byte{}))
				}

				chunk := combinedStream[offset : offset+chunkSize]
				offset += chunkSize

				success := decoder.Decode(chunk)
				require.True(t, success)
			}
			decoder.EndDecoding()

			assert.False(t, decoder.HasError())
			assert.True(t, decoder.AtInstructionBoundary())
			require.Equal(t, len(expectedInstructions), len(delegate.decodedInstructions))
			require.Equal(t, 0, len(delegate.errorCalls))

			for idx, exp := range expectedInstructions {
				got := delegate.decodedValues[idx]
				assert.Equal(t, exp.SBit(), got.SBit)
				assert.Equal(t, exp.Varint(), got.Varint)
				assert.Equal(t, exp.Name(), got.Name)
				assert.Equal(t, exp.Value(), got.Value)
			}
		}
	})
}

// -----------------------------------------------------------------------------
// 2. Malformed Varints
// -----------------------------------------------------------------------------

func TestM2Adversarial_MalformedVarints(t *testing.T) {
	t.Run("Varint11ExtensionBytes0xFF", func(t *testing.T) {
		// Varint with 11 extension bytes of 0xff (exceeds 10-byte continuation limit).
		// Prefix 5: Duplicate instruction (opcode 0x00, mask 0xe0, param 5)
		// Byte 0: 0x1f (prefix = 31), followed by 11 bytes of 0xff
		malformedData := append([]byte{0x1f}, bytes.Repeat([]byte{0xff}, 11)...)

		delegate := newAdversarialTestDelegate()
		decoder := NewInstructionDecoder(EncoderStreamLanguage(), delegate)

		success := decoder.Decode(malformedData)
		assert.False(t, success)
		assert.True(t, decoder.HasError())
		require.Equal(t, 1, len(delegate.errorCalls))
		assert.Equal(t, InstructionDecoderIntegerTooLarge, delegate.errorCalls[0].ErrorCode)
		assert.Equal(t, "Encoded integer too large.", delegate.errorCalls[0].Message)

		// Subsequent decode calls must fail immediately
		assert.False(t, decoder.Decode([]byte{0x00}))
	})

	t.Run("Varint10thByteContinuationBitSet", func(t *testing.T) {
		// 9 continuation bytes with 0x80 bit set, and 10th continuation byte has 0x80 bit set
		// Prefix 6: HeaderAcknowledgement (opcode 0x80, mask 0x80, param 7)
		data := append([]byte{0xff}, bytes.Repeat([]byte{0x80}, 10)...)

		delegate := newAdversarialTestDelegate()
		decoder := NewInstructionDecoder(DecoderStreamLanguage(), delegate)

		success := decoder.Decode(data)
		assert.False(t, success)
		assert.True(t, decoder.HasError())
		require.Equal(t, 1, len(delegate.errorCalls))
		assert.Equal(t, InstructionDecoderIntegerTooLarge, delegate.errorCalls[0].ErrorCode)
	})

	t.Run("Varint64BitIntegerOverflowIn10thByte", func(t *testing.T) {
		// Prefix 8 (Prefix instruction, opcode 0x00, mask 0x00, param 8)
		// Prefix byte: 0xff (255)
		// 9 continuation bytes of 0xff (each contributes 127 << (7 * i))
		// The 10th extension byte has summand = 1 (summand << 63 = 0x8000000000000000).
		// Prefix 255 + (2^63 - 1) + 2^63 overflows uint64.
		data := append([]byte{0xff}, bytes.Repeat([]byte{0xff}, 9)...)
		data = append(data, 0x01) // 10th extension byte with bit 63 set

		delegate := newAdversarialTestDelegate()
		decoder := NewInstructionDecoder(PrefixLanguage(), delegate)

		success := decoder.Decode(data)
		assert.False(t, success)
		assert.True(t, decoder.HasError())
		require.Equal(t, 1, len(delegate.errorCalls))
		assert.Equal(t, InstructionDecoderIntegerTooLarge, delegate.errorCalls[0].ErrorCode)
		assert.Equal(t, "Encoded integer too large.", delegate.errorCalls[0].Message)
	})

	t.Run("Varint10thByteSummandGreaterThan1", func(t *testing.T) {
		// 10th extension byte must have summand <= 1. If summand is 2 (e.g. 0x02), must fail.
		data := append([]byte{0xff}, bytes.Repeat([]byte{0xff}, 9)...)
		data = append(data, 0x02) // summand = 2 > 1

		delegate := newAdversarialTestDelegate()
		decoder := NewInstructionDecoder(PrefixLanguage(), delegate)

		success := decoder.Decode(data)
		assert.False(t, success)
		assert.True(t, decoder.HasError())
		require.Equal(t, 1, len(delegate.errorCalls))
		assert.Equal(t, InstructionDecoderIntegerTooLarge, delegate.errorCalls[0].ErrorCode)
	})

	t.Run("PartialVarintTruncationAtPrefix", func(t *testing.T) {
		// Byte 0 indicates varint follows (all prefix bits 1), but no extension bytes follow
		delegate := newAdversarialTestDelegate()
		decoder := NewInstructionDecoder(EncoderStreamLanguage(), delegate)

		// 0x1f = Duplicate instruction with prefix 5 all 1s (needs extension byte)
		success := decoder.Decode([]byte{0x1f})
		assert.True(t, success) // Still in progress
		assert.False(t, decoder.AtInstructionBoundary())
		assert.False(t, decoder.HasError())

		// Premature EndDecoding must report truncated instruction
		decoder.EndDecoding()
		assert.True(t, decoder.HasError())
		require.Equal(t, 1, len(delegate.errorCalls))
		assert.Equal(t, InstructionDecoderIntegerTooLarge, delegate.errorCalls[0].ErrorCode)
		assert.Equal(t, "Truncated instruction.", delegate.errorCalls[0].Message)
	})

	t.Run("PartialVarintTruncationMidContinuation", func(t *testing.T) {
		// Varint with continuation bytes that never terminate with (b & 0x80) == 0
		delegate := newAdversarialTestDelegate()
		decoder := NewInstructionDecoder(DecoderStreamLanguage(), delegate)

		// 0xff (HeaderAcknowledgement, param 7 = 127), then 3 continuation bytes with 0x80 bit set
		success := decoder.Decode([]byte{0xff, 0x80, 0x81, 0x82})
		assert.True(t, success)
		assert.False(t, decoder.AtInstructionBoundary())

		decoder.EndDecoding()
		assert.True(t, decoder.HasError())
		require.Equal(t, 1, len(delegate.errorCalls))
		assert.Equal(t, InstructionDecoderIntegerTooLarge, delegate.errorCalls[0].ErrorCode)
		assert.Equal(t, "Truncated instruction.", delegate.errorCalls[0].Message)
	})

	t.Run("MaxValidUint64RoundTrip", func(t *testing.T) {
		// Ensure that the maximum valid uint64 does NOT overflow and decodes accurately
		encoder := NewInstructionEncoder(HuffmanEncodingDisabled)
		inst := InstructionWithValuesDuplicate(math.MaxUint64)
		encoded := encoder.Encode(inst, nil)

		delegate := newAdversarialTestDelegate()
		decoder := NewInstructionDecoder(EncoderStreamLanguage(), delegate)
		delegate.decoderRef = decoder

		success := decoder.Decode(encoded)
		assert.True(t, success)
		decoder.EndDecoding()
		assert.False(t, decoder.HasError())
		require.Equal(t, 1, len(delegate.decodedValues))
		assert.Equal(t, uint64(math.MaxUint64), delegate.decodedValues[0].Varint)
	})
}

// -----------------------------------------------------------------------------
// 3. Malformed Huffman Padding & Sequences
// -----------------------------------------------------------------------------

func TestM2Adversarial_MalformedHuffman(t *testing.T) {
	t.Run("PaddingGreaterThan7Bits", func(t *testing.T) {
		// InsertWithNameReference (opcode 0x80, mask 0x80)
		// Byte 0: 0xc0 (S-bit 1, index 0)
		// Value field: H-bit 1 (0x80), length 2 (0x02) -> 0x82
		// Payload: "c1ff" (Chromium test vector: 'a' is 0x18 / 0x60, padding has 8+ 1s)
		data, err := hex.DecodeString("c082c1ff")
		require.NoError(t, err)

		delegate := newAdversarialTestDelegate()
		decoder := NewInstructionDecoder(EncoderStreamLanguage(), delegate)

		success := decoder.Decode(data)
		assert.False(t, success)
		assert.True(t, decoder.HasError())
		require.Equal(t, 1, len(delegate.errorCalls))
		assert.Equal(t, InstructionDecoderHuffmanEncodingError, delegate.errorCalls[0].ErrorCode)
		assert.Equal(t, "Error in Huffman-encoded string.", delegate.errorCalls[0].Message)
	})

	t.Run("CorruptedPaddingBitsNotAllOnes", func(t *testing.T) {
		// Encode a valid Huffman string, then flip the lowest padding bit to 0
		// "a" has Huffman code: 00011 (5 bits). Remaining 3 bits of byte 0 are padding (must be 111 = 0x07).
		// Byte is 00011111 = 0x1f.

		// Find the encoded payload and corrupt the padding bits
		// Instead of valid padding, inject an invalid byte where padding bits are 0
		// For InsertWithoutNameReference: opcode 0x40 | H-bit 0x20 | len 1 = 0x61, byte = 0x18 (00011000 - padding is 000 instead of 111)
		corrupted := []byte{
			0x61, // H=1, len=1
			0x18, // 00011 000 (padding has zeroes!)
			0x81, // Value: H=1, len=1
			0x1f, // valid 'a'
		}

		delegate := newAdversarialTestDelegate()
		decoder := NewInstructionDecoder(EncoderStreamLanguage(), delegate)

		success := decoder.Decode(corrupted)
		assert.False(t, success)
		assert.True(t, decoder.HasError())
		require.Equal(t, 1, len(delegate.errorCalls))
		assert.Equal(t, InstructionDecoderHuffmanEncodingError, delegate.errorCalls[0].ErrorCode)
		assert.Equal(t, "Error in Huffman-encoded string.", delegate.errorCalls[0].Message)
	})

	t.Run("ExplicitEOSSequenceInPayload", func(t *testing.T) {
		// RFC 7541 §5.2: An explicit EOS symbol (30 consecutive 1s) MUST be treated as decoding error.
		// Construct an instruction containing 30 consecutive 1-bits (4 bytes: 0xff, 0xff, 0xff, 0xfc)
		data := []byte{
			0x64,                   // InsertWithoutNameReference: H=1, len=4
			0xff, 0xff, 0xff, 0xfc, // 30 bits of EOS
			0x81, 0x1f, // Value
		}

		delegate := newAdversarialTestDelegate()
		decoder := NewInstructionDecoder(EncoderStreamLanguage(), delegate)

		success := decoder.Decode(data)
		assert.False(t, success)
		assert.True(t, decoder.HasError())
		require.Equal(t, 1, len(delegate.errorCalls))
		assert.Equal(t, InstructionDecoderHuffmanEncodingError, delegate.errorCalls[0].ErrorCode)
	})

	t.Run("TruncatedHuffmanPayload", func(t *testing.T) {
		// String length specifies 5 bytes, but only 2 bytes arrive before EndDecoding
		data := []byte{
			0x65,       // H=1, len=5
			0x1f, 0x1f, // only 2 bytes
		}

		delegate := newAdversarialTestDelegate()
		decoder := NewInstructionDecoder(EncoderStreamLanguage(), delegate)

		success := decoder.Decode(data)
		assert.True(t, success) // in progress
		assert.False(t, decoder.AtInstructionBoundary())

		decoder.EndDecoding()
		assert.True(t, decoder.HasError())
		require.Equal(t, 1, len(delegate.errorCalls))
		assert.Equal(t, InstructionDecoderIntegerTooLarge, delegate.errorCalls[0].ErrorCode)
		assert.Equal(t, "Truncated instruction.", delegate.errorCalls[0].Message)
	})
}

// -----------------------------------------------------------------------------
// 4. Excessively Long Strings (> 1 MB Cap) & Boundary Lengths
// -----------------------------------------------------------------------------

func TestM2Adversarial_StringLengthLimits(t *testing.T) {
	t.Run("Length1MBPlus1Byte_Rejected", func(t *testing.T) {
		// 1 MB = 1048576 bytes. Limit is 1 MB. 1048577 must be rejected.
		// InsertWithoutNameReference (opcode 0x40, mask 0xc0, Name param 5)
		// Varint for 1,048,577:
		// Prefix 5: 31 (0x1f). Remainder = 1048577 - 31 = 1048546.
		// 1048546 = 0x80|98 (0xe2), 0x80|127 (0xff), 0x3f.
		data := []byte{
			0x5f, 0xe2, 0xff, 0x3f, // Name len = 1048577, H=0
		}

		delegate := newAdversarialTestDelegate()
		decoder := NewInstructionDecoder(EncoderStreamLanguage(), delegate)

		success := decoder.Decode(data)
		assert.False(t, success)
		assert.True(t, decoder.HasError())
		require.Equal(t, 1, len(delegate.errorCalls))
		assert.Equal(t, InstructionDecoderStringLiteralTooLong, delegate.errorCalls[0].ErrorCode)
		assert.Equal(t, "String literal too long.", delegate.errorCalls[0].Message)
	})

	t.Run("Length2MB_Rejected", func(t *testing.T) {
		// Chromium test vector: "bfffff7f" on testInstruction2 (opcode 0x80, Name param 6)
		dataChromium, err := hex.DecodeString("bfffff7f")
		require.NoError(t, err)

		delegateChromium := newAdversarialTestDelegate()
		decoderChromium := NewInstructionDecoder(testLanguage(), delegateChromium)

		success := decoderChromium.Decode(dataChromium)
		assert.False(t, success)
		assert.True(t, decoderChromium.HasError())
		require.Equal(t, 1, len(delegateChromium.errorCalls))
		assert.Equal(t, InstructionDecoderStringLiteralTooLong, delegateChromium.errorCalls[0].ErrorCode)
		assert.Equal(t, "String literal too long.", delegateChromium.errorCalls[0].Message)

		// Also test on EncoderStreamLanguage using InsertWithoutNameReference (opcode 0x40, Name param 5)
		var dataEncoderStream []byte
		qpackEncodeVarint(0x40, 5, 2*1024*1024, &dataEncoderStream)

		delegateEncoder := newAdversarialTestDelegate()
		decoderEncoder := NewInstructionDecoder(EncoderStreamLanguage(), delegateEncoder)

		success = decoderEncoder.Decode(dataEncoderStream)
		assert.False(t, success)
		assert.True(t, decoderEncoder.HasError())
		require.Equal(t, 1, len(delegateEncoder.errorCalls))
		assert.Equal(t, InstructionDecoderStringLiteralTooLong, delegateEncoder.errorCalls[0].ErrorCode)
		assert.Equal(t, "String literal too long.", delegateEncoder.errorCalls[0].Message)
	})

	t.Run("LengthMaxUint64_Rejected", func(t *testing.T) {
		// Name length varint encoded as math.MaxUint64
		var buf []byte
		// Encode varint math.MaxUint64 with prefix 5
		qpackEncodeVarint(0x40, 5, math.MaxUint64, &buf)

		delegate := newAdversarialTestDelegate()
		decoder := NewInstructionDecoder(EncoderStreamLanguage(), delegate)

		success := decoder.Decode(buf)
		assert.False(t, success)
		assert.True(t, decoder.HasError())
		require.Equal(t, 1, len(delegate.errorCalls))
		assert.Equal(t, InstructionDecoderStringLiteralTooLong, delegate.errorCalls[0].ErrorCode)
	})

	t.Run("Exact1MBLiteralString_Accepted", func(t *testing.T) {
		// Exactly 1024 * 1024 bytes string literal is within the limit and must succeed
		oneMBString := strings.Repeat("A", 1024*1024)
		encoder := NewInstructionEncoder(HuffmanEncodingDisabled)
		inst := InstructionWithValuesInsertWithoutNameReference("test-name", oneMBString)
		encoded := encoder.Encode(inst, nil)

		delegate := newAdversarialTestDelegate()
		decoder := NewInstructionDecoder(EncoderStreamLanguage(), delegate)
		delegate.decoderRef = decoder

		// Decode in 64KB chunks
		offset := 0
		for offset < len(encoded) {
			chunkSize := 64 * 1024
			if offset+chunkSize > len(encoded) {
				chunkSize = len(encoded) - offset
			}
			chunk := encoded[offset : offset+chunkSize]
			offset += chunkSize

			success := decoder.Decode(chunk)
			require.True(t, success)
		}
		decoder.EndDecoding()

		assert.False(t, decoder.HasError())
		assert.True(t, decoder.AtInstructionBoundary())
		require.Equal(t, 1, len(delegate.decodedValues))
		assert.Equal(t, "test-name", delegate.decodedValues[0].Name)
		assert.Equal(t, len(oneMBString), len(delegate.decodedValues[0].Value))
	})

	t.Run("ZeroLengthStrings_Accepted", func(t *testing.T) {
		// Empty Name and empty Value strings
		encoder := NewInstructionEncoder(HuffmanEncodingDisabled)
		inst := InstructionWithValuesInsertWithoutNameReference("", "")
		encoded := encoder.Encode(inst, nil)

		delegate := newAdversarialTestDelegate()
		decoder := NewInstructionDecoder(EncoderStreamLanguage(), delegate)
		delegate.decoderRef = decoder

		success := decoder.Decode(encoded)
		assert.True(t, success)
		decoder.EndDecoding()

		assert.False(t, decoder.HasError())
		assert.True(t, decoder.AtInstructionBoundary())
		require.Equal(t, 1, len(delegate.decodedValues))
		assert.Equal(t, "", delegate.decodedValues[0].Name)
		assert.Equal(t, "", delegate.decodedValues[0].Value)
	})
}

// -----------------------------------------------------------------------------
// 5. Reentrancy and Zero-Byte Inputs
// -----------------------------------------------------------------------------

func TestM2Adversarial_ZeroByteInputsAndReentrancy(t *testing.T) {
	encoder := NewInstructionEncoder(HuffmanEncodingDisabled)

	t.Run("ConsecutiveZeroByteChunksAtEveryState", func(t *testing.T) {
		inst := InstructionWithValuesInsertWithoutNameReference("header-name", "header-value")
		encoded := encoder.Encode(inst, nil)

		delegate := newAdversarialTestDelegate()
		decoder := NewInstructionDecoder(EncoderStreamLanguage(), delegate)
		delegate.decoderRef = decoder

		// 10 zero-byte calls before data
		for i := 0; i < 10; i++ {
			assert.True(t, decoder.Decode([]byte{}))
			assert.True(t, decoder.Decode(nil))
		}

		// Inject 3 zero-byte calls between every single byte of encoded data
		for i := 0; i < len(encoded); i++ {
			assert.True(t, decoder.Decode([]byte{}))
			assert.True(t, decoder.Decode(nil))
			assert.True(t, decoder.Decode(encoded[i:i+1]))
			assert.True(t, decoder.Decode([]byte{}))
		}

		// 10 zero-byte calls after data
		for i := 0; i < 10; i++ {
			assert.True(t, decoder.Decode([]byte{}))
			assert.True(t, decoder.Decode(nil))
		}

		decoder.EndDecoding()
		assert.False(t, decoder.HasError())
		assert.True(t, decoder.AtInstructionBoundary())
		require.Equal(t, 1, len(delegate.decodedValues))
		assert.Equal(t, "header-name", delegate.decodedValues[0].Name)
		assert.Equal(t, "header-value", delegate.decodedValues[0].Value)
	})

	t.Run("ReentrantDecodeDuringOnInstructionDecoded", func(t *testing.T) {
		// When instruction 1 is decoded, delegate reentrantly feeds instruction 2
		inst1 := InstructionWithValuesDuplicate(10)
		inst2 := InstructionWithValuesDuplicate(20)

		encoded1 := encoder.Encode(inst1, nil)
		encoded2 := encoder.Encode(inst2, nil)

		reentrancyCount := 0
		delegate := newAdversarialTestDelegate()
		decoder := NewInstructionDecoder(EncoderStreamLanguage(), delegate)
		delegate.decoderRef = decoder

		delegate.onDecodedCallback = func(instruction *Instruction, dec *InstructionDecoder) bool {
			reentrancyCount++
			if reentrancyCount == 1 {
				// Reentrantly decode inst2
				ok := dec.Decode(encoded2)
				assert.True(t, ok)
			}
			return true
		}

		// Decode inst1
		success := decoder.Decode(encoded1)
		assert.True(t, success)
		decoder.EndDecoding()

		assert.False(t, decoder.HasError())
		assert.True(t, decoder.AtInstructionBoundary())
		require.Equal(t, 2, len(delegate.decodedValues))
		assert.Equal(t, uint64(10), delegate.decodedValues[0].Varint) // inst1 recorded first upon completion
		assert.Equal(t, uint64(20), delegate.decodedValues[1].Varint) // inst2 recorded during reentrant callback
	})

	t.Run("ReentrantEndDecodingDuringOnInstructionDecoded", func(t *testing.T) {
		inst := InstructionWithValuesDuplicate(42)
		encoded := encoder.Encode(inst, nil)

		endDecodingCalled := false
		delegate := newAdversarialTestDelegate()
		decoder := NewInstructionDecoder(EncoderStreamLanguage(), delegate)
		delegate.decoderRef = decoder

		delegate.onDecodedCallback = func(instruction *Instruction, dec *InstructionDecoder) bool {
			// At instruction completion, decoder is at boundary
			assert.True(t, dec.AtInstructionBoundary())
			dec.EndDecoding()
			assert.False(t, dec.HasError())
			endDecodingCalled = true
			return true
		}

		success := decoder.Decode(encoded)
		assert.True(t, success)
		assert.True(t, endDecodingCalled)
		assert.False(t, decoder.HasError())
		require.Equal(t, 1, len(delegate.decodedValues))
	})

	t.Run("ReentrantInvalidDecodeDuringOnInstructionDecoded", func(t *testing.T) {
		inst := InstructionWithValuesDuplicate(100)
		encoded := encoder.Encode(inst, nil)

		delegate := newAdversarialTestDelegate()
		decoder := NewInstructionDecoder(EncoderStreamLanguage(), delegate)
		delegate.decoderRef = decoder

		delegate.onDecodedCallback = func(instruction *Instruction, dec *InstructionDecoder) bool {
			// Feed malformed varint with 11 0xff bytes
			malformed := append([]byte{0x1f}, bytes.Repeat([]byte{0xff}, 11)...)
			ok := dec.Decode(malformed)
			assert.False(t, ok)
			return true
		}

		success := decoder.Decode(encoded)
		assert.False(t, success)
		assert.True(t, decoder.HasError())
		require.Equal(t, 1, len(delegate.errorCalls))
		assert.Equal(t, InstructionDecoderIntegerTooLarge, delegate.errorCalls[0].ErrorCode)
	})

	t.Run("ReentrantCallDuringOnErrorCallback", func(t *testing.T) {
		delegate := newAdversarialTestDelegate()
		decoder := NewInstructionDecoder(EncoderStreamLanguage(), delegate)
		delegate.decoderRef = decoder

		reentrantErrorCallHandled := false
		delegate.onErrorCallback = func(errorCode InstructionDecoderErrorCode, message string) {
			// Attempt to call Decode from within error handler
			ok := decoder.Decode([]byte{0x00})
			assert.False(t, ok) // Must immediately reject
			reentrantErrorCallHandled = true
		}

		// Trigger error with malformed varint
		malformed := append([]byte{0x1f}, bytes.Repeat([]byte{0xff}, 11)...)
		success := decoder.Decode(malformed)
		assert.False(t, success)
		assert.True(t, reentrantErrorCallHandled)
		assert.True(t, decoder.HasError())
	})
}

// -----------------------------------------------------------------------------
// 6. Robustness Under Fuzzing and Bit Corruption
// -----------------------------------------------------------------------------

func TestM2Adversarial_FuzzAndRandomBitCorruption(t *testing.T) {
	encoder := NewInstructionEncoder(HuffmanEncodingEnabled)

	t.Run("BitCorruptionDoesNotPanicOrHang", func(t *testing.T) {
		// Valid instruction to corrupt
		inst := InstructionWithValuesInsertWithoutNameReference("content-encoding", "gzip, deflate, br")
		validEncoded := encoder.Encode(inst, nil)

		rng := rand.New(rand.NewSource(42))

		// Run 500 iterations of bit corruptions
		for iter := 0; iter < 500; iter++ {
			corrupted := make([]byte, len(validEncoded))
			copy(corrupted, validEncoded)

			// Flip 1 to 3 random bits
			flips := rng.Intn(3) + 1
			for f := 0; f < flips; f++ {
				byteIdx := rng.Intn(len(corrupted))
				bitIdx := uint(rng.Intn(8))
				corrupted[byteIdx] ^= (1 << bitIdx)
			}

			delegate := newAdversarialTestDelegate()
			decoder := NewInstructionDecoder(EncoderStreamLanguage(), delegate)

			// Feed in random chunks (1..3 bytes)
			offset := 0
			for offset < len(corrupted) {
				chunkSize := rng.Intn(3) + 1
				if offset+chunkSize > len(corrupted) {
					chunkSize = len(corrupted) - offset
				}
				chunk := corrupted[offset : offset+chunkSize]
				offset += chunkSize

				if !decoder.Decode(chunk) {
					break
				}
			}
			decoder.EndDecoding()

			// Invariant: Must never panic or deadlock.
			// Either succeeded or reported an error.
			if decoder.HasError() {
				assert.True(t, len(delegate.errorCalls) > 0)
			}
		}
	})

	t.Run("PureRandomGarbageDoesNotPanicOrHang", func(t *testing.T) {
		languages := []*Language{
			EncoderStreamLanguage(),
			DecoderStreamLanguage(),
			PrefixLanguage(),
			RequestStreamLanguage(),
		}

		rng := rand.New(rand.NewSource(999))

		for iter := 0; iter < 500; iter++ {
			length := rng.Intn(200) + 1
			garbage := make([]byte, length)
			rng.Read(garbage)

			lang := languages[rng.Intn(len(languages))]
			delegate := newAdversarialTestDelegate()
			decoder := NewInstructionDecoder(lang, delegate)

			offset := 0
			for offset < len(garbage) {
				chunkSize := rng.Intn(5) + 1
				if offset+chunkSize > len(garbage) {
					chunkSize = len(garbage) - offset
				}
				chunk := garbage[offset : offset+chunkSize]
				offset += chunkSize

				if !decoder.Decode(chunk) {
					break
				}
			}
			decoder.EndDecoding()

			// Invariant: Completed without panic.
		}
	})
}
