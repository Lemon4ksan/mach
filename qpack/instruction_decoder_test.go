// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package qpack

import (
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/lemon4ksan/foundation/testing/assert"
	"github.com/lemon4ksan/foundation/testing/require"
)

// FragmentMode defines how input slices are partitioned into Decode() calls.
// Direct translation of Chromium's FragmentMode.
type FragmentMode int

const (
	FragmentModeSingleChunk FragmentMode = iota
	FragmentModeOctetByOctet
)

func (m FragmentMode) String() string {
	switch m {
	case FragmentModeSingleChunk:
		return "SingleChunk"
	case FragmentModeOctetByOctet:
		return "OctetByOctet"
	default:
		return fmt.Sprintf("FragmentMode(%d)", m)
	}
}

// TestInstruction1: Opcode {0x00, 0x80}, Fields: Sbit(0x40), Varint(6), Varint2(8)
func testInstruction1() *Instruction {
	return &Instruction{
		Opcode: InstructionOpcode{Value: 0x00, Mask: 0x80},
		Fields: []InstructionField{
			{Type: InstructionFieldTypeSbit, Param: 0x40},
			{Type: InstructionFieldTypeVarint, Param: 6},
			{Type: InstructionFieldTypeVarint2, Param: 8},
		},
	}
}

// TestInstruction2: Opcode {0x80, 0x80}, Fields: Name(6), Value(7)
func testInstruction2() *Instruction {
	return &Instruction{
		Opcode: InstructionOpcode{Value: 0x80, Mask: 0x80},
		Fields: []InstructionField{
			{Type: InstructionFieldTypeName, Param: 6},
			{Type: InstructionFieldTypeValue, Param: 7},
		},
	}
}

func testLanguage() *Language {
	return &Language{
		testInstruction1(),
		testInstruction2(),
	}
}

// MockDelegate captures callbacks and provides hook points for custom test logic.
type mockInstructionDecoderDelegate struct {
	onInstructionDecodedFunc func(instruction *Instruction) bool
	onErrorFunc              func(errorCode InstructionDecoderErrorCode, errorMessage string)

	decodedCount     int
	decodedList      []*Instruction
	errorCount       int
	lastErrorCode    InstructionDecoderErrorCode
	lastErrorMessage string
}

func newMockInstructionDecoderDelegate() *mockInstructionDecoderDelegate {
	return &mockInstructionDecoderDelegate{
		onInstructionDecodedFunc: func(instruction *Instruction) bool {
			return true
		},
	}
}

func (d *mockInstructionDecoderDelegate) OnInstructionDecoded(instruction *Instruction) bool {
	d.decodedCount++
	d.decodedList = append(d.decodedList, instruction)
	if d.onInstructionDecodedFunc != nil {
		return d.onInstructionDecodedFunc(instruction)
	}
	return true
}

func (d *mockInstructionDecoderDelegate) OnInstructionDecodingError(
	errorCode InstructionDecoderErrorCode,
	errorMessage string,
) {
	d.errorCount++
	d.lastErrorCode = errorCode
	d.lastErrorMessage = errorMessage
	if d.onErrorFunc != nil {
		d.onErrorFunc(errorCode, errorMessage)
	}
}

// Test fixture wrapping decoder, delegate, and fragment execution.
type instructionDecoderTestFixture struct {
	t            *testing.T
	fragmentMode FragmentMode
	delegate     *mockInstructionDecoderDelegate
	decoder      *InstructionDecoder
}

func newInstructionDecoderTestFixture(t *testing.T, mode FragmentMode) *instructionDecoderTestFixture {
	f := &instructionDecoderTestFixture{
		t:            t,
		fragmentMode: mode,
		delegate:     newMockInstructionDecoderDelegate(),
	}
	f.decoder = NewInstructionDecoder(testLanguage(), f.delegate)

	// In Chromium: ON_CALL(delegate_, OnInstructionDecodingError(_, _))
	//                  .WillByDefault([this]() { decoder_.reset(); });
	// Destroy InstructionDecoder on error to test that it does not crash (crbug.com/1025209).
	f.delegate.onErrorFunc = func(errorCode InstructionDecoderErrorCode, errorMessage string) {
		f.decoder = nil
	}

	return f
}

func (f *instructionDecoderTestFixture) decodeInstruction(data []byte) {
	require.NotNil(f.t, f.decoder)
	assert.True(f.t, f.decoder.AtInstructionBoundary())

	if f.fragmentMode == FragmentModeSingleChunk {
		success := f.decoder.Decode(data)
		if f.decoder == nil {
			assert.False(f.t, success)
			return
		}
		assert.True(f.t, success)
		assert.True(f.t, f.decoder.AtInstructionBoundary())
	} else {
		// OctetByOctet
		for i := 0; i < len(data); i++ {
			success := f.decoder.Decode(data[i : i+1])
			if f.decoder == nil {
				assert.False(f.t, success)
				return
			}
			assert.True(f.t, success)
			if i+1 < len(data) {
				assert.False(f.t, f.decoder.AtInstructionBoundary())
			}
		}
		assert.True(f.t, f.decoder.AtInstructionBoundary())
	}
}

func runInstructionDecoderTests(t *testing.T, testFunc func(t *testing.T, f *instructionDecoderTestFixture)) {
	modes := []FragmentMode{FragmentModeSingleChunk, FragmentModeOctetByOctet}
	for _, mode := range modes {
		t.Run(mode.String(), func(t *testing.T) {
			f := newInstructionDecoderTestFixture(t, mode)
			testFunc(t, f)
		})
	}
}

// -----------------------------------------------------------------------------
// 7 Ported Decoder Unit Tests from Chromium's qpack_instruction_decoder_test.cc
// -----------------------------------------------------------------------------

// TEST_P(InstructionDecoderTest, SBitAndVarint2)
func TestInstructionDecoder_SBitAndVarint2(t *testing.T) {
	runInstructionDecoderTests(t, func(t *testing.T, f *instructionDecoderTestFixture) {
		encodedData1, err := hex.DecodeString("7f01ff65")
		require.NoError(t, err)

		f.decodeInstruction(encodedData1)
		assert.Equal(t, 1, f.delegate.decodedCount)
		assert.True(t, f.decoder.SBit())
		assert.Equal(t, uint64(64), f.decoder.Varint())
		assert.Equal(t, uint64(356), f.decoder.Varint2())

		encodedData2, err := hex.DecodeString("05c8")
		require.NoError(t, err)

		f.decodeInstruction(encodedData2)
		assert.Equal(t, 2, f.delegate.decodedCount)
		assert.False(t, f.decoder.SBit())
		assert.Equal(t, uint64(5), f.decoder.Varint())
		assert.Equal(t, uint64(200), f.decoder.Varint2())
	})
}

// TEST_P(InstructionDecoderTest, NameAndValue)
func TestInstructionDecoder_NameAndValue(t *testing.T) {
	runInstructionDecoderTests(t, func(t *testing.T, f *instructionDecoderTestFixture) {
		encodedData1, err := hex.DecodeString("83666f6f03626172")
		require.NoError(t, err)

		f.decodeInstruction(encodedData1)
		assert.Equal(t, 1, f.delegate.decodedCount)
		assert.Equal(t, "foo", f.decoder.Name())
		assert.Equal(t, "bar", f.decoder.Value())

		encodedData2, err := hex.DecodeString("8000")
		require.NoError(t, err)

		f.decodeInstruction(encodedData2)
		assert.Equal(t, 2, f.delegate.decodedCount)
		assert.Equal(t, "", f.decoder.Name())
		assert.Equal(t, "", f.decoder.Value())

		encodedData3, err := hex.DecodeString("c294e7838c767f")
		require.NoError(t, err)

		f.decodeInstruction(encodedData3)
		assert.Equal(t, 3, f.delegate.decodedCount)
		assert.Equal(t, "foo", f.decoder.Name())
		assert.Equal(t, "bar", f.decoder.Value())
	})
}

// TEST_P(InstructionDecoderTest, InvalidHuffmanEncoding)
func TestInstructionDecoder_InvalidHuffmanEncoding(t *testing.T) {
	runInstructionDecoderTests(t, func(t *testing.T, f *instructionDecoderTestFixture) {
		encodedData, err := hex.DecodeString("c1ff")
		require.NoError(t, err)

		var capturedErrorCode InstructionDecoderErrorCode
		var capturedMessage string
		f.delegate.onErrorFunc = func(errorCode InstructionDecoderErrorCode, errorMessage string) {
			capturedErrorCode = errorCode
			capturedMessage = errorMessage
			f.decoder = nil
		}

		f.decodeInstruction(encodedData)
		assert.Equal(t, 1, f.delegate.errorCount)
		assert.Equal(t, InstructionDecoderHuffmanEncodingError, capturedErrorCode)
		assert.Equal(t, "Error in Huffman-encoded string.", capturedMessage)
		assert.Nil(t, f.decoder)
	})
}

// TEST_P(InstructionDecoderTest, InvalidVarintEncoding)
func TestInstructionDecoder_InvalidVarintEncoding(t *testing.T) {
	runInstructionDecoderTests(t, func(t *testing.T, f *instructionDecoderTestFixture) {
		encodedData, err := hex.DecodeString("ffffffffffffffffffffff")
		require.NoError(t, err)

		var capturedErrorCode InstructionDecoderErrorCode
		var capturedMessage string
		f.delegate.onErrorFunc = func(errorCode InstructionDecoderErrorCode, errorMessage string) {
			capturedErrorCode = errorCode
			capturedMessage = errorMessage
			f.decoder = nil
		}

		f.decodeInstruction(encodedData)
		assert.Equal(t, 1, f.delegate.errorCount)
		assert.Equal(t, InstructionDecoderIntegerTooLarge, capturedErrorCode)
		assert.Equal(t, "Encoded integer too large.", capturedMessage)
		assert.Nil(t, f.decoder)
	})
}

// TEST_P(InstructionDecoderTest, StringLiteralTooLong)
func TestInstructionDecoder_StringLiteralTooLong(t *testing.T) {
	runInstructionDecoderTests(t, func(t *testing.T, f *instructionDecoderTestFixture) {
		encodedData, err := hex.DecodeString("bfffff7f")
		require.NoError(t, err)

		var capturedErrorCode InstructionDecoderErrorCode
		var capturedMessage string
		f.delegate.onErrorFunc = func(errorCode InstructionDecoderErrorCode, errorMessage string) {
			capturedErrorCode = errorCode
			capturedMessage = errorMessage
			f.decoder = nil
		}

		f.decodeInstruction(encodedData)
		assert.Equal(t, 1, f.delegate.errorCount)
		assert.Equal(t, InstructionDecoderStringLiteralTooLong, capturedErrorCode)
		assert.Equal(t, "String literal too long.", capturedMessage)
		assert.Nil(t, f.decoder)
	})
}

// TEST_P(InstructionDecoderTest, DelegateSignalsError)
func TestInstructionDecoder_DelegateSignalsError(t *testing.T) {
	runInstructionDecoderTests(t, func(t *testing.T, f *instructionDecoderTestFixture) {
		callCount := 0
		f.delegate.onInstructionDecodedFunc = func(instruction *Instruction) bool {
			callCount++
			if callCount == 1 {
				assert.Equal(t, uint64(1), f.decoder.Varint())
				return true
			}
			assert.Equal(t, uint64(2), f.decoder.Varint())
			return false
		}

		encodedData, err := hex.DecodeString("01000200030004000500")
		require.NoError(t, err)

		if f.fragmentMode == FragmentModeSingleChunk {
			success := f.decoder.Decode(encodedData)
			assert.False(t, success)
			assert.Equal(t, 2, callCount)
		} else {
			var success bool
			for i := 0; i < len(encodedData); i++ {
				success = f.decoder.Decode(encodedData[i : i+1])
				if !success {
					break
				}
			}
			assert.False(t, success)
			assert.Equal(t, 2, callCount)
		}
	})
}

// TEST_P(InstructionDecoderTest, DelegateSignalsErrorAndDestroysDecoder)
func TestInstructionDecoder_DelegateSignalsErrorAndDestroysDecoder(t *testing.T) {
	runInstructionDecoderTests(t, func(t *testing.T, f *instructionDecoderTestFixture) {
		f.delegate.onInstructionDecodedFunc = func(instruction *Instruction) bool {
			assert.Equal(t, uint64(1), f.decoder.Varint())
			f.decoder = nil
			return false
		}

		encodedData, err := hex.DecodeString("0100")
		require.NoError(t, err)

		f.decodeInstruction(encodedData)
		assert.Nil(t, f.decoder)
		assert.Equal(t, 1, f.delegate.decodedCount)
	})
}

func TestInstructionDecoder_EndDecodingAndHasError(t *testing.T) {
	delegate := newMockInstructionDecoderDelegate()
	decoder := NewInstructionDecoder(testLanguage(), delegate)

	assert.False(t, decoder.HasError())
	assert.True(t, decoder.AtInstructionBoundary())

	// Calling EndDecoding when at instruction boundary does not cause error
	decoder.EndDecoding()
	assert.False(t, decoder.HasError())

	// Decode incomplete instruction
	partial, _ := hex.DecodeString("7f")
	success := decoder.Decode(partial)
	assert.True(t, success)
	assert.False(t, decoder.AtInstructionBoundary())
	assert.False(t, decoder.HasError())

	// Calling EndDecoding when NOT at boundary reports error
	decoder.EndDecoding()
	assert.True(t, decoder.HasError())
	assert.Equal(t, 1, delegate.errorCount)
	assert.Equal(t, InstructionDecoderIntegerTooLarge, delegate.lastErrorCode)
	assert.Equal(t, "Truncated instruction.", delegate.lastErrorMessage)
}

func TestInstruction_CodecRoundTripAllInstructions(t *testing.T) {
	testCases := []struct {
		name     string
		language *Language
		create   func() *InstructionWithValues
	}{
		{
			name:     "InsertWithNameReference_Static",
			language: EncoderStreamLanguage(),
			create: func() *InstructionWithValues {
				return InstructionWithValuesInsertWithNameReference(true, 42, "custom-value")
			},
		},
		{
			name:     "InsertWithNameReference_Dynamic",
			language: EncoderStreamLanguage(),
			create: func() *InstructionWithValues {
				return InstructionWithValuesInsertWithNameReference(false, 105, "dynamic-value-foo-bar")
			},
		},
		{
			name:     "InsertWithoutNameReference",
			language: EncoderStreamLanguage(),
			create: func() *InstructionWithValues {
				return InstructionWithValuesInsertWithoutNameReference(":custom-key", "my-custom-value")
			},
		},
		{
			name:     "Duplicate",
			language: EncoderStreamLanguage(),
			create: func() *InstructionWithValues {
				return InstructionWithValuesDuplicate(99)
			},
		},
		{
			name:     "SetDynamicTableCapacity",
			language: EncoderStreamLanguage(),
			create: func() *InstructionWithValues {
				return InstructionWithValuesSetDynamicTableCapacity(16384)
			},
		},
		{
			name:     "HeaderAcknowledgement",
			language: DecoderStreamLanguage(),
			create: func() *InstructionWithValues {
				return InstructionWithValuesHeaderAcknowledgement(1024)
			},
		},
		{
			name:     "StreamCancellation",
			language: DecoderStreamLanguage(),
			create: func() *InstructionWithValues {
				return InstructionWithValuesStreamCancellation(512)
			},
		},
		{
			name:     "InsertCountIncrement",
			language: DecoderStreamLanguage(),
			create: func() *InstructionWithValues {
				return InstructionWithValuesInsertCountIncrement(25)
			},
		},
		{
			name:     "Prefix",
			language: PrefixLanguage(),
			create: func() *InstructionWithValues {
				inst := InstructionWithValuesPrefix(12345)
				inst.SetSBit(true)
				inst.SetVarint2(67)
				return inst
			},
		},
		{
			name:     "IndexedHeaderField",
			language: RequestStreamLanguage(),
			create: func() *InstructionWithValues {
				return InstructionWithValuesIndexedHeaderField(true, 17)
			},
		},
		{
			name:     "LiteralHeaderFieldNameReference",
			language: RequestStreamLanguage(),
			create: func() *InstructionWithValues {
				return InstructionWithValuesLiteralHeaderFieldNameReference(false, 33, "val-ref-test")
			},
		},
		{
			name:     "LiteralHeaderField",
			language: RequestStreamLanguage(),
			create: func() *InstructionWithValues {
				return InstructionWithValuesLiteralHeaderField("content-type", "application/json; charset=utf-8")
			},
		},
		{
			name:     "IndexedHeaderFieldPostBase",
			language: RequestStreamLanguage(),
			create: func() *InstructionWithValues {
				return InstructionWithValuesIndexedHeaderFieldPostBase(88)
			},
		},
		{
			name:     "LiteralHeaderFieldPostBaseNameReference",
			language: RequestStreamLanguage(),
			create: func() *InstructionWithValues {
				return InstructionWithValuesLiteralHeaderFieldPostBaseNameReference(12, "post-base-value-123")
			},
		},
	}

	huffmanModes := []HuffmanEncoding{HuffmanEncodingEnabled, HuffmanEncodingDisabled}
	fragmentModes := []FragmentMode{FragmentModeSingleChunk, FragmentModeOctetByOctet}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			for _, huff := range huffmanModes {
				t.Run(huff.String(), func(t *testing.T) {
					for _, frag := range fragmentModes {
						t.Run(frag.String(), func(t *testing.T) {
							inst := tc.create()
							encoder := NewInstructionEncoder(huff)
							encoded := encoder.Encode(inst, nil)
							require.NotEmpty(t, encoded)

							delegate := newMockInstructionDecoderDelegate()
							decoder := NewInstructionDecoder(tc.language, delegate)
							require.True(t, decoder.AtInstructionBoundary())

							if frag == FragmentModeSingleChunk {
								ok := decoder.Decode(encoded)
								require.True(t, ok)
							} else {
								for i := 0; i < len(encoded); i++ {
									ok := decoder.Decode(encoded[i : i+1])
									require.True(t, ok)
								}
							}

							require.True(t, decoder.AtInstructionBoundary())
							assert.False(t, decoder.HasError())
							assert.Equal(t, 1, delegate.decodedCount)

							// Verify decoded values match original
							assert.Equal(t, inst.Instruction(), delegate.decodedList[0])
							assert.Equal(t, inst.SBit(), decoder.SBit())
							assert.Equal(t, inst.Varint(), decoder.Varint())
							assert.Equal(t, inst.Varint2(), decoder.Varint2())
							assert.Equal(t, inst.Name(), decoder.Name())
							assert.Equal(t, inst.Value(), decoder.Value())
						})
					}
				})
			}
		})
	}
}
