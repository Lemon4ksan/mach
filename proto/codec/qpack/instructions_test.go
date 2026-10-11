// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package qpack

import (
	"fmt"
	"testing"

	"github.com/lemon4ksan/foundation/testing/assert"
	"github.com/lemon4ksan/foundation/testing/require"
)

// validateLanguage verifies that the language is well-formed:
// 1. Bits of Value that are zero in Mask must be zero in each instruction.
// 2. Every byte from 0 to 255 matches exactly one instruction opcode.
// Direct port of Chromium's ValidateLanguage from qpack_instructions.cc.
func validateLanguage(t *testing.T, language *Language) {
	require.NotNil(t, language)

	// 1. In each instruction, bits of Value that are zero in Mask must be zero.
	for _, inst := range *language {
		assert.Equal(
			t,
			uint8(0),
			inst.Opcode.Value&^inst.Opcode.Mask,
			fmt.Sprintf(
				"Instruction opcode value %08b has bits outside mask %08b",
				inst.Opcode.Value,
				inst.Opcode.Mask,
			),
		)
	}

	// 2. Every byte from 0 to 255 must match exactly one instruction opcode.
	for b := 0; b <= 255; b++ {
		byteVal := uint8(b)
		matchCount := 0
		for _, inst := range *language {
			if (byteVal & inst.Opcode.Mask) == inst.Opcode.Value {
				matchCount++
			}
		}
		assert.Equal(
			t,
			1,
			matchCount,
			fmt.Sprintf("Byte %08b (%d) must match exactly 1 instruction, matched %d", byteVal, b, matchCount),
		)
	}
}

func TestInstructions_LanguagesValid(t *testing.T) {
	t.Run("EncoderStreamLanguage", func(t *testing.T) {
		validateLanguage(t, EncoderStreamLanguage())
	})
	t.Run("DecoderStreamLanguage", func(t *testing.T) {
		validateLanguage(t, DecoderStreamLanguage())
	})
	t.Run("PrefixLanguage", func(t *testing.T) {
		validateLanguage(t, PrefixLanguage())
	})
	t.Run("RequestStreamLanguage", func(t *testing.T) {
		validateLanguage(t, RequestStreamLanguage())
	})
}

func TestInstructions_OpcodeEqual(t *testing.T) {
	op1 := InstructionOpcode{Value: 0x80, Mask: 0x80}
	op2 := InstructionOpcode{Value: 0x80, Mask: 0x80}
	op3 := InstructionOpcode{Value: 0x40, Mask: 0xc0}

	assert.True(t, op1.Equal(op2))
	assert.False(t, op1.Equal(op3))
}

func TestInstructions_LanguageInstructionsHelper(t *testing.T) {
	lang := EncoderStreamLanguage()
	instructions := lang.Instructions()
	assert.Equal(t, 4, len(instructions))

	var nilLang *Language
	assert.Nil(t, nilLang.Instructions())
}

func TestInstructionWithValues_FactoriesAndAccessors(t *testing.T) {
	// InsertWithNameReference
	v1 := InstructionWithValuesInsertWithNameReference(true, 42, "val1")
	assert.Equal(t, InsertWithNameReferenceInstruction(), v1.Instruction())
	assert.True(t, v1.SBit())
	assert.Equal(t, uint64(42), v1.Varint())
	assert.Equal(t, "val1", v1.Value())

	// InsertWithoutNameReference
	v2 := InstructionWithValuesInsertWithoutNameReference("name2", "val2")
	assert.Equal(t, InsertWithoutNameReferenceInstruction(), v2.Instruction())
	assert.Equal(t, "name2", v2.Name())
	assert.Equal(t, "val2", v2.Value())

	// Duplicate
	v3 := InstructionWithValuesDuplicate(15)
	assert.Equal(t, DuplicateInstruction(), v3.Instruction())
	assert.Equal(t, uint64(15), v3.Varint())

	// SetDynamicTableCapacity
	v4 := InstructionWithValuesSetDynamicTableCapacity(4096)
	assert.Equal(t, SetDynamicTableCapacityInstruction(), v4.Instruction())
	assert.Equal(t, uint64(4096), v4.Varint())

	// InsertCountIncrement
	v5 := InstructionWithValuesInsertCountIncrement(10)
	assert.Equal(t, InsertCountIncrementInstruction(), v5.Instruction())
	assert.Equal(t, uint64(10), v5.Varint())

	// HeaderAcknowledgement
	v6 := InstructionWithValuesHeaderAcknowledgement(7)
	assert.Equal(t, HeaderAcknowledgementInstruction(), v6.Instruction())
	assert.Equal(t, uint64(7), v6.Varint())

	// StreamCancellation
	v7 := InstructionWithValuesStreamCancellation(9)
	assert.Equal(t, StreamCancellationInstruction(), v7.Instruction())
	assert.Equal(t, uint64(9), v7.Varint())

	// Prefix
	v8 := InstructionWithValuesPrefix(100)
	assert.Equal(t, PrefixInstruction(), v8.Instruction())
	assert.Equal(t, uint64(100), v8.Varint())
	assert.Equal(t, uint64(0), v8.Varint2())
	assert.False(t, v8.SBit())

	// IndexedHeaderField
	v9 := InstructionWithValuesIndexedHeaderField(false, 3)
	assert.Equal(t, IndexedHeaderFieldInstruction(), v9.Instruction())
	assert.False(t, v9.SBit())
	assert.Equal(t, uint64(3), v9.Varint())

	// LiteralHeaderFieldNameReference
	v10 := InstructionWithValuesLiteralHeaderFieldNameReference(true, 5, "hello")
	assert.Equal(t, LiteralHeaderFieldNameReferenceInstruction(), v10.Instruction())
	assert.True(t, v10.SBit())
	assert.Equal(t, uint64(5), v10.Varint())
	assert.Equal(t, "hello", v10.Value())

	// LiteralHeaderField
	v11 := InstructionWithValuesLiteralHeaderField("custom-name", "custom-val")
	assert.Equal(t, LiteralHeaderFieldInstruction(), v11.Instruction())
	assert.Equal(t, "custom-name", v11.Name())
	assert.Equal(t, "custom-val", v11.Value())

	// IndexedHeaderFieldPostBase
	v12 := InstructionWithValuesIndexedHeaderFieldPostBase(8)
	assert.Equal(t, IndexedHeaderFieldPostBaseInstruction(), v12.Instruction())
	assert.Equal(t, uint64(8), v12.Varint())

	// LiteralHeaderFieldPostBaseNameReference
	v13 := InstructionWithValuesLiteralHeaderFieldPostBaseNameReference(4, "post-val")
	assert.Equal(t, LiteralHeaderFieldPostBaseNameReferenceInstruction(), v13.Instruction())
	assert.Equal(t, uint64(4), v13.Varint())
	assert.Equal(t, "post-val", v13.Value())

	// Setters
	customInst := &Instruction{}
	v := NewInstructionWithValues(customInst)
	assert.Equal(t, customInst, v.Instruction())
	v.SetInstruction(instDuplicate)
	assert.Equal(t, instDuplicate, v.Instruction())
	v.SetSBit(true)
	assert.True(t, v.SBit())
	v.SetVarint(99)
	assert.Equal(t, uint64(99), v.Varint())
	v.SetVarint2(199)
	assert.Equal(t, uint64(199), v.Varint2())
	v.SetName("new-name")
	assert.Equal(t, "new-name", v.Name())
	v.SetValue("new-val")
	assert.Equal(t, "new-val", v.Value())
}
