// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package qpack

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/lemon4ksan/foundation/testing/assert"
	"github.com/lemon4ksan/foundation/testing/require"
)

// Test fixture tracking incremental output segments across Encode() calls.
// Direct translation of Chromium's InstructionEncoderTest.
type instructionEncoderTestFixture struct {
	t                *testing.T
	huffman          HuffmanEncoding
	encoder          *InstructionEncoder
	output           []byte
	verifiedPosition int
}

func newInstructionEncoderTestFixture(t *testing.T, huffman HuffmanEncoding) *instructionEncoderTestFixture {
	return &instructionEncoderTestFixture{
		t:       t,
		huffman: huffman,
		encoder: NewInstructionEncoder(huffman),
	}
}

func (f *instructionEncoderTestFixture) disableHuffmanEncoding() bool {
	return f.huffman == HuffmanEncodingDisabled
}

func (f *instructionEncoderTestFixture) encodeInstruction(inst *InstructionWithValues) {
	f.output = f.encoder.Encode(inst, f.output)
}

func (f *instructionEncoderTestFixture) encodedSegmentMatches(hexExpected string) bool {
	expected, err := hex.DecodeString(hexExpected)
	require.NoError(f.t, err)

	recentlyEncoded := f.output[f.verifiedPosition:]
	f.verifiedPosition = len(f.output)
	return bytes.Equal(recentlyEncoded, expected)
}

func runInstructionEncoderTests(t *testing.T, testFunc func(t *testing.T, f *instructionEncoderTestFixture)) {
	modes := []HuffmanEncoding{HuffmanEncodingEnabled, HuffmanEncodingDisabled}
	for _, mode := range modes {
		t.Run(mode.String(), func(t *testing.T) {
			f := newInstructionEncoderTestFixture(t, mode)
			testFunc(t, f)
		})
	}
}

// -----------------------------------------------------------------------------
// 6 Ported Encoder Unit Tests from Chromium's qpack_instruction_encoder_test.cc
// -----------------------------------------------------------------------------

// TEST_P(InstructionEncoderTest, Varint)
func TestInstructionEncoder_Varint(t *testing.T) {
	runInstructionEncoderTests(t, func(t *testing.T, f *instructionEncoderTestFixture) {
		instruction := &Instruction{
			Opcode: InstructionOpcode{Value: 0x00, Mask: 0x80},
			Fields: []InstructionField{
				{Type: InstructionFieldTypeVarint, Param: 7},
			},
		}

		instWithValues := NewInstructionWithValues(instruction)
		instWithValues.SetVarint(5)
		f.encodeInstruction(instWithValues)
		assert.True(t, f.encodedSegmentMatches("05"))

		instWithValues.SetVarint(127)
		f.encodeInstruction(instWithValues)
		assert.True(t, f.encodedSegmentMatches("7f00"))
	})
}

// TEST_P(InstructionEncoderTest, SBitAndTwoVarint2)
func TestInstructionEncoder_SBitAndTwoVarint2(t *testing.T) {
	runInstructionEncoderTests(t, func(t *testing.T, f *instructionEncoderTestFixture) {
		instruction := &Instruction{
			Opcode: InstructionOpcode{Value: 0x80, Mask: 0xc0},
			Fields: []InstructionField{
				{Type: InstructionFieldTypeSbit, Param: 0x20},
				{Type: InstructionFieldTypeVarint, Param: 5},
				{Type: InstructionFieldTypeVarint2, Param: 8},
			},
		}

		instWithValues := NewInstructionWithValues(instruction)
		instWithValues.SetSBit(true)
		instWithValues.SetVarint(5)
		instWithValues.SetVarint2(200)
		f.encodeInstruction(instWithValues)
		assert.True(t, f.encodedSegmentMatches("a5c8"))

		instWithValues.SetSBit(false)
		instWithValues.SetVarint(31)
		instWithValues.SetVarint2(356)
		f.encodeInstruction(instWithValues)
		assert.True(t, f.encodedSegmentMatches("9f00ff65"))
	})
}

// TEST_P(InstructionEncoderTest, SBitAndVarintAndValue)
func TestInstructionEncoder_SBitAndVarintAndValue(t *testing.T) {
	runInstructionEncoderTests(t, func(t *testing.T, f *instructionEncoderTestFixture) {
		instruction := &Instruction{
			Opcode: InstructionOpcode{Value: 0xc0, Mask: 0xc0},
			Fields: []InstructionField{
				{Type: InstructionFieldTypeSbit, Param: 0x20},
				{Type: InstructionFieldTypeVarint, Param: 5},
				{Type: InstructionFieldTypeValue, Param: 7},
			},
		}

		instWithValues := NewInstructionWithValues(instruction)
		instWithValues.SetSBit(true)
		instWithValues.SetVarint(100)
		instWithValues.SetValue("foo")
		f.encodeInstruction(instWithValues)

		if f.disableHuffmanEncoding() {
			assert.True(t, f.encodedSegmentMatches("ff4503666f6f"))
		} else {
			assert.True(t, f.encodedSegmentMatches("ff458294e7"))
		}

		instWithValues.SetSBit(false)
		instWithValues.SetVarint(3)
		instWithValues.SetValue("bar")
		f.encodeInstruction(instWithValues)
		// "bar" Huffman size is 3, which is not strictly less than 3, so literal is used in both modes.
		assert.True(t, f.encodedSegmentMatches("c303626172"))
	})
}

// TEST_P(InstructionEncoderTest, Name)
func TestInstructionEncoder_Name(t *testing.T) {
	runInstructionEncoderTests(t, func(t *testing.T, f *instructionEncoderTestFixture) {
		instruction := &Instruction{
			Opcode: InstructionOpcode{Value: 0xe0, Mask: 0xe0},
			Fields: []InstructionField{
				{Type: InstructionFieldTypeName, Param: 4},
			},
		}

		instWithValues := NewInstructionWithValues(instruction)
		instWithValues.SetName("")
		f.encodeInstruction(instWithValues)
		assert.True(t, f.encodedSegmentMatches("e0"))

		instWithValues.SetName("foo")
		f.encodeInstruction(instWithValues)
		if f.disableHuffmanEncoding() {
			assert.True(t, f.encodedSegmentMatches("e3666f6f"))
		} else {
			assert.True(t, f.encodedSegmentMatches("f294e7"))
		}

		instWithValues.SetName("bar")
		f.encodeInstruction(instWithValues)
		assert.True(t, f.encodedSegmentMatches("e3626172"))
	})
}

// TEST_P(InstructionEncoderTest, Value)
func TestInstructionEncoder_Value(t *testing.T) {
	runInstructionEncoderTests(t, func(t *testing.T, f *instructionEncoderTestFixture) {
		instruction := &Instruction{
			Opcode: InstructionOpcode{Value: 0xf0, Mask: 0xf0},
			Fields: []InstructionField{
				{Type: InstructionFieldTypeValue, Param: 3},
			},
		}

		instWithValues := NewInstructionWithValues(instruction)
		instWithValues.SetValue("")
		f.encodeInstruction(instWithValues)
		assert.True(t, f.encodedSegmentMatches("f0"))

		instWithValues.SetValue("foo")
		f.encodeInstruction(instWithValues)
		if f.disableHuffmanEncoding() {
			assert.True(t, f.encodedSegmentMatches("f3666f6f"))
		} else {
			assert.True(t, f.encodedSegmentMatches("fa94e7"))
		}

		instWithValues.SetValue("bar")
		f.encodeInstruction(instWithValues)
		assert.True(t, f.encodedSegmentMatches("f3626172"))
	})
}

// TEST_P(InstructionEncoderTest, SBitAndNameAndValue)
func TestInstructionEncoder_SBitAndNameAndValue(t *testing.T) {
	runInstructionEncoderTests(t, func(t *testing.T, f *instructionEncoderTestFixture) {
		instruction := &Instruction{
			Opcode: InstructionOpcode{Value: 0xf0, Mask: 0xf0},
			Fields: []InstructionField{
				{Type: InstructionFieldTypeSbit, Param: 0x08},
				{Type: InstructionFieldTypeName, Param: 2},
				{Type: InstructionFieldTypeValue, Param: 7},
			},
		}

		instWithValues := NewInstructionWithValues(instruction)
		instWithValues.SetSBit(false)
		instWithValues.SetName("")
		instWithValues.SetValue("")
		f.encodeInstruction(instWithValues)
		assert.True(t, f.encodedSegmentMatches("f000"))

		instWithValues.SetSBit(true)
		instWithValues.SetName("foo")
		instWithValues.SetValue("bar")
		f.encodeInstruction(instWithValues)
		if f.disableHuffmanEncoding() {
			assert.True(t, f.encodedSegmentMatches("fb00666f6f03626172"))
		} else {
			assert.True(t, f.encodedSegmentMatches("fe94e703626172"))
		}
	})
}
