// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package qpack

import "sync"

// Backward-compatible aliases for RFC 9204 and QUIC error codes.
const (
	QPACK_DECOMPRESSION_FAILED = uint64(ErrCodeDecompressionFailed)
	QPACK_ENCODER_STREAM_ERROR = uint64(ErrCodeEncoderStreamError)
	QPACK_DECODER_STREAM_ERROR = uint64(ErrCodeDecoderStreamError)

	QUIC_NO_ERROR       = uint64(ErrCodeNoError)
	QUIC_INTERNAL_ERROR = uint64(ErrCodeInternalError)

	QUIC_QPACK_DECOMPRESSION_FAILED = uint64(ErrCodeDecompressionFailed)

	QUIC_QPACK_ENCODER_STREAM_INTEGER_TOO_LARGE                 = uint64(ErrCodeEncoderIntegerTooLarge)
	QUIC_QPACK_ENCODER_STREAM_STRING_LITERAL_TOO_LONG           = uint64(ErrCodeEncoderStringLiteralTooLong)
	QUIC_QPACK_ENCODER_STREAM_HUFFMAN_ENCODING_ERROR            = uint64(ErrCodeEncoderHuffmanEncodingError)
	QUIC_QPACK_ENCODER_STREAM_INVALID_STATIC_ENTRY              = uint64(ErrCodeEncoderInvalidStaticEntry)
	QUIC_QPACK_ENCODER_STREAM_ERROR_INSERTING_STATIC            = uint64(ErrCodeEncoderErrorInsertingStatic)
	QUIC_QPACK_ENCODER_STREAM_INSERTION_INVALID_RELATIVE_INDEX  = uint64(ErrCodeEncoderInsertionInvalidRelativeIndex)
	QUIC_QPACK_ENCODER_STREAM_INSERTION_DYNAMIC_ENTRY_NOT_FOUND = uint64(ErrCodeEncoderInsertionDynamicEntryNotFound)
	QUIC_QPACK_ENCODER_STREAM_ERROR_INSERTING_DYNAMIC           = uint64(ErrCodeEncoderErrorInsertingDynamic)
	QUIC_QPACK_ENCODER_STREAM_ERROR_INSERTING_LITERAL           = uint64(ErrCodeEncoderErrorInsertingLiteral)
	QUIC_QPACK_ENCODER_STREAM_DUPLICATE_INVALID_RELATIVE_INDEX  = uint64(ErrCodeEncoderDuplicateInvalidRelativeIndex)
	QUIC_QPACK_ENCODER_STREAM_DUPLICATE_DYNAMIC_ENTRY_NOT_FOUND = uint64(ErrCodeEncoderDuplicateDynamicEntryNotFound)
	QUIC_QPACK_ENCODER_STREAM_SET_DYNAMIC_TABLE_CAPACITY        = uint64(ErrCodeEncoderSetDynamicTableCapacity)
	QUIC_QPACK_DECODER_STREAM_INTEGER_TOO_LARGE                 = uint64(ErrCodeDecoderIntegerTooLarge)
	QUIC_QPACK_DECODER_STREAM_INVALID_ZERO_INCREMENT            = uint64(ErrCodeDecoderInvalidZeroIncrement)
	QUIC_QPACK_DECODER_STREAM_INCREMENT_OVERFLOW                = uint64(ErrCodeDecoderIncrementOverflow)
	QUIC_QPACK_DECODER_STREAM_IMPOSSIBLE_INSERT_COUNT           = uint64(ErrCodeDecoderImpossibleInsertCount)
	QUIC_QPACK_DECODER_STREAM_INCORRECT_ACKNOWLEDGEMENT         = uint64(ErrCodeDecoderIncorrectAcknowledgement)
)

// InstructionOpcode represents the bitmask and value identifying an instruction in the first byte.
// |Mask| determines which bits are part of the opcode.
// |Value| is the expected value of those bits (all other bits must be zero).
// Follows RFC 9204 instruction wire format definitions.
type InstructionOpcode struct {
	Value byte
	Mask  byte
}

func (o InstructionOpcode) Equal(other InstructionOpcode) bool {
	return o.Value == other.Value && o.Mask == other.Mask
}

// InstructionFieldType identifies the type of an instruction field.
// Follows RFC 9204 stream instruction field definitions.
type InstructionFieldType int

const (
	// InstructionFieldTypeSbit: A single bit indicating static table reference or sign of Delta Base.
	InstructionFieldTypeSbit InstructionFieldType = iota
	// InstructionFieldTypeVarint: Variable-length integer (index, stream ID, max capacity, RIC).
	InstructionFieldTypeVarint
	// InstructionFieldTypeVarint2: Second variable-length integer (Delta Base).
	InstructionFieldTypeVarint2
	// InstructionFieldTypeName: Header name (Huffman flag + length prefix varint + string literal).
	InstructionFieldTypeName
	// InstructionFieldTypeValue: Header value (Huffman flag + length prefix varint + string literal).
	InstructionFieldTypeValue
)

// InstructionField describes a single field following an opcode.
// For kSbit: param is mask with exactly one bit set.
// For kVarint / kVarint2: param is prefix length in bits (1..8).
// For kName / kValue: param is prefix length of string length varint.
// The bit immediately preceding the prefix is interpreted as the Huffman flag.
type InstructionField struct {
	Type  InstructionFieldType
	Param uint8
}

// Instruction consists of an opcode followed by an ordered list of fields.
// Follows RFC 9204 wire format specification.
type Instruction struct {
	Opcode InstructionOpcode
	Fields []InstructionField
}

// Language represents a complete stream grammar.
// In a valid language, every byte from 0 to 255 matches exactly one instruction opcode.
type Language []*Instruction

// Instructions returns the underlying slice of instructions.
func (l *Language) Instructions() []*Instruction {
	if l == nil {
		return nil
	}

	return *l
}

// -----------------------------------------------------------------------------
// Singleton Instructions (13 Total per RFC 9204)
// -----------------------------------------------------------------------------

var (
	onceInstructions sync.Once

	// Encoder stream instructions (RFC 9204 §4.3)
	instInsertWithNameReference    *Instruction
	instInsertWithoutNameReference *Instruction
	instDuplicate                  *Instruction
	instSetDynamicTableCapacity    *Instruction

	// Decoder stream instructions (RFC 9204 §4.4)
	instHeaderAcknowledgement *Instruction
	instStreamCancellation    *Instruction
	instInsertCountIncrement  *Instruction

	// Prefix instruction (RFC 9204 §4.5.1)
	instPrefix *Instruction

	// Request / Push stream field line instructions (RFC 9204 §4.5.2 - §4.5.6)
	instIndexedHeaderField                      *Instruction
	instLiteralHeaderFieldNameReference         *Instruction
	instLiteralHeaderField                      *Instruction
	instIndexedHeaderFieldPostBase              *Instruction
	instLiteralHeaderFieldPostBaseNameReference *Instruction

	// 4 Languages
	langEncoderStream *Language
	langDecoderStream *Language
	langPrefix        *Language
	langRequestStream *Language
)

func initInstructions() {
	// 1. Insert With Name Reference (RFC 9204 §4.3.1): [1 T NNNNNN] [H VVVVVVV]
	instInsertWithNameReference = &Instruction{
		Opcode: InstructionOpcode{Value: 0x80, Mask: 0x80},
		Fields: []InstructionField{
			{Type: InstructionFieldTypeSbit, Param: 0x40},
			{Type: InstructionFieldTypeVarint, Param: 6},
			{Type: InstructionFieldTypeValue, Param: 7},
		},
	}

	// 2. Insert Without Name Reference (RFC 9204 §4.3.2): [01 H NNNNN] [H VVVVVVV]
	instInsertWithoutNameReference = &Instruction{
		Opcode: InstructionOpcode{Value: 0x40, Mask: 0xc0},
		Fields: []InstructionField{
			{Type: InstructionFieldTypeName, Param: 5},
			{Type: InstructionFieldTypeValue, Param: 7},
		},
	}

	// 3. Duplicate (RFC 9204 §4.3.3): [000 NNNNN]
	instDuplicate = &Instruction{
		Opcode: InstructionOpcode{Value: 0x00, Mask: 0xe0},
		Fields: []InstructionField{
			{Type: InstructionFieldTypeVarint, Param: 5},
		},
	}

	// 4. Set Dynamic Table Capacity (RFC 9204 §4.3.4): [001 CCCCC]
	instSetDynamicTableCapacity = &Instruction{
		Opcode: InstructionOpcode{Value: 0x20, Mask: 0xe0},
		Fields: []InstructionField{
			{Type: InstructionFieldTypeVarint, Param: 5},
		},
	}

	// 5. Header Acknowledgement (RFC 9204 §4.4.1): [1 SSSSSSS]
	instHeaderAcknowledgement = &Instruction{
		Opcode: InstructionOpcode{Value: 0x80, Mask: 0x80},
		Fields: []InstructionField{
			{Type: InstructionFieldTypeVarint, Param: 7},
		},
	}

	// 6. Stream Cancellation (RFC 9204 §4.4.2): [01 SSSSSS]
	instStreamCancellation = &Instruction{
		Opcode: InstructionOpcode{Value: 0x40, Mask: 0xc0},
		Fields: []InstructionField{
			{Type: InstructionFieldTypeVarint, Param: 6},
		},
	}

	// 7. Insert Count Increment (RFC 9204 §4.4.3): [00 IIIIII]
	instInsertCountIncrement = &Instruction{
		Opcode: InstructionOpcode{Value: 0x00, Mask: 0xc0},
		Fields: []InstructionField{
			{Type: InstructionFieldTypeVarint, Param: 6},
		},
	}

	// 8. Prefix (RFC 9204 §4.5.1): [RRRRRRRR] [S DDDDDDD]
	instPrefix = &Instruction{
		Opcode: InstructionOpcode{Value: 0x00, Mask: 0x00},
		Fields: []InstructionField{
			{Type: InstructionFieldTypeVarint, Param: 8},
			{Type: InstructionFieldTypeSbit, Param: 0x80},
			{Type: InstructionFieldTypeVarint2, Param: 7},
		},
	}

	// 9. Indexed Header Field (RFC 9204 §4.5.2): [1 T NNNNNN]
	instIndexedHeaderField = &Instruction{
		Opcode: InstructionOpcode{Value: 0x80, Mask: 0x80},
		Fields: []InstructionField{
			{Type: InstructionFieldTypeSbit, Param: 0x40},
			{Type: InstructionFieldTypeVarint, Param: 6},
		},
	}

	// 10. Literal Header Field With Name Reference (RFC 9204 §4.5.4): [0100/0101 T NNNN] [H VVVVVVV]
	instLiteralHeaderFieldNameReference = &Instruction{
		Opcode: InstructionOpcode{Value: 0x40, Mask: 0xc0},
		Fields: []InstructionField{
			{Type: InstructionFieldTypeSbit, Param: 0x10},
			{Type: InstructionFieldTypeVarint, Param: 4},
			{Type: InstructionFieldTypeValue, Param: 7},
		},
	}

	// 11. Literal Header Field Without Name Reference (RFC 9204 §4.5.6): [001 H NNN] [H VVVVVVV]
	instLiteralHeaderField = &Instruction{
		Opcode: InstructionOpcode{Value: 0x20, Mask: 0xe0},
		Fields: []InstructionField{
			{Type: InstructionFieldTypeName, Param: 3},
			{Type: InstructionFieldTypeValue, Param: 7},
		},
	}

	// 12. Indexed Header Field With Post-Base Index (RFC 9204 §4.5.3): [0001 NNNN]
	instIndexedHeaderFieldPostBase = &Instruction{
		Opcode: InstructionOpcode{Value: 0x10, Mask: 0xf0},
		Fields: []InstructionField{
			{Type: InstructionFieldTypeVarint, Param: 4},
		},
	}

	// 13. Literal Header Field With Post-Base Name Reference (RFC 9204 §4.5.5): [0000 NNN] [H VVVVVVV]
	instLiteralHeaderFieldPostBaseNameReference = &Instruction{
		Opcode: InstructionOpcode{Value: 0x00, Mask: 0xf0},
		Fields: []InstructionField{
			{Type: InstructionFieldTypeVarint, Param: 3},
			{Type: InstructionFieldTypeValue, Param: 7},
		},
	}

	// Languages
	langEncoderStream = &Language{
		instInsertWithNameReference,
		instInsertWithoutNameReference,
		instDuplicate,
		instSetDynamicTableCapacity,
	}

	langDecoderStream = &Language{
		instHeaderAcknowledgement,
		instStreamCancellation,
		instInsertCountIncrement,
	}

	langPrefix = &Language{
		instPrefix,
	}

	langRequestStream = &Language{
		instIndexedHeaderField,
		instLiteralHeaderFieldNameReference,
		instLiteralHeaderField,
		instIndexedHeaderFieldPostBase,
		instLiteralHeaderFieldPostBaseNameReference,
	}
}

func ensureInstructions() {
	onceInstructions.Do(initInstructions)
}

// 13 Singleton Instruction Accessors
func InsertWithNameReferenceInstruction() *Instruction {
	ensureInstructions()
	return instInsertWithNameReference
}

func InsertWithoutNameReferenceInstruction() *Instruction {
	ensureInstructions()
	return instInsertWithoutNameReference
}

func DuplicateInstruction() *Instruction {
	ensureInstructions()
	return instDuplicate
}

func SetDynamicTableCapacityInstruction() *Instruction {
	ensureInstructions()
	return instSetDynamicTableCapacity
}

func HeaderAcknowledgementInstruction() *Instruction {
	ensureInstructions()
	return instHeaderAcknowledgement
}

func StreamCancellationInstruction() *Instruction {
	ensureInstructions()
	return instStreamCancellation
}

func InsertCountIncrementInstruction() *Instruction {
	ensureInstructions()
	return instInsertCountIncrement
}

func PrefixInstruction() *Instruction {
	ensureInstructions()
	return instPrefix
}

func IndexedHeaderFieldInstruction() *Instruction {
	ensureInstructions()
	return instIndexedHeaderField
}

func LiteralHeaderFieldNameReferenceInstruction() *Instruction {
	ensureInstructions()
	return instLiteralHeaderFieldNameReference
}

func LiteralHeaderFieldInstruction() *Instruction {
	ensureInstructions()
	return instLiteralHeaderField
}

func IndexedHeaderFieldPostBaseInstruction() *Instruction {
	ensureInstructions()
	return instIndexedHeaderFieldPostBase
}

func LiteralHeaderFieldPostBaseNameReferenceInstruction() *Instruction {
	ensureInstructions()
	return instLiteralHeaderFieldPostBaseNameReference
}

// 4 Language Accessors
func EncoderStreamLanguage() *Language {
	ensureInstructions()
	return langEncoderStream
}

func DecoderStreamLanguage() *Language {
	ensureInstructions()
	return langDecoderStream
}

func PrefixLanguage() *Language {
	ensureInstructions()
	return langPrefix
}

func RequestStreamLanguage() *Language {
	ensureInstructions()
	return langRequestStream
}

// -----------------------------------------------------------------------------
// InstructionWithValues
// -----------------------------------------------------------------------------

// InstructionWithValues holds an instruction grammar reference and bound field values.
// Follows RFC 9204 instruction definitions.
type InstructionWithValues struct {
	instruction *Instruction
	sBit        bool
	varint      uint64
	varint2     uint64
	name        string
	value       string
}

func NewInstructionWithValues(instruction *Instruction) *InstructionWithValues {
	return &InstructionWithValues{instruction: instruction}
}

// Accessors
func (v *InstructionWithValues) Instruction() *Instruction { return v.instruction }
func (v *InstructionWithValues) SBit() bool                { return v.sBit }
func (v *InstructionWithValues) Varint() uint64            { return v.varint }
func (v *InstructionWithValues) Varint2() uint64           { return v.varint2 }
func (v *InstructionWithValues) Name() string              { return v.name }
func (v *InstructionWithValues) Value() string             { return v.value }

// Setters
func (v *InstructionWithValues) SetInstruction(inst *Instruction) { v.instruction = inst }
func (v *InstructionWithValues) SetSBit(sBit bool)                { v.sBit = sBit }
func (v *InstructionWithValues) SetVarint(varint uint64)          { v.varint = varint }
func (v *InstructionWithValues) SetVarint2(varint2 uint64)        { v.varint2 = varint2 }
func (v *InstructionWithValues) SetName(name string)              { v.name = name }
func (v *InstructionWithValues) SetValue(value string)            { v.value = value }

// Factory functions for creating bound instructions
func InstructionWithValuesInsertWithNameReference(
	isStatic bool,
	nameIndex uint64,
	value string,
) *InstructionWithValues {
	return &InstructionWithValues{
		instruction: InsertWithNameReferenceInstruction(),
		sBit:        isStatic,
		varint:      nameIndex,
		value:       value,
	}
}

func InstructionWithValuesInsertWithoutNameReference(name, value string) *InstructionWithValues {
	return &InstructionWithValues{
		instruction: InsertWithoutNameReferenceInstruction(),
		name:        name,
		value:       value,
	}
}

func InstructionWithValuesDuplicate(index uint64) *InstructionWithValues {
	return &InstructionWithValues{
		instruction: DuplicateInstruction(),
		varint:      index,
	}
}

func InstructionWithValuesSetDynamicTableCapacity(capacity uint64) *InstructionWithValues {
	return &InstructionWithValues{
		instruction: SetDynamicTableCapacityInstruction(),
		varint:      capacity,
	}
}

func InstructionWithValuesInsertCountIncrement(increment uint64) *InstructionWithValues {
	return &InstructionWithValues{
		instruction: InsertCountIncrementInstruction(),
		varint:      increment,
	}
}

func InstructionWithValuesHeaderAcknowledgement(streamID uint64) *InstructionWithValues {
	return &InstructionWithValues{
		instruction: HeaderAcknowledgementInstruction(),
		varint:      streamID,
	}
}

func InstructionWithValuesStreamCancellation(streamID uint64) *InstructionWithValues {
	return &InstructionWithValues{
		instruction: StreamCancellationInstruction(),
		varint:      streamID,
	}
}

func InstructionWithValuesPrefix(requiredInsertCount uint64) *InstructionWithValues {
	return &InstructionWithValues{
		instruction: PrefixInstruction(),
		varint:      requiredInsertCount,
		varint2:     0,
		sBit:        false,
	}
}

func InstructionWithValuesIndexedHeaderField(isStatic bool, index uint64) *InstructionWithValues {
	return &InstructionWithValues{
		instruction: IndexedHeaderFieldInstruction(),
		sBit:        isStatic,
		varint:      index,
	}
}

func InstructionWithValuesLiteralHeaderFieldNameReference(
	isStatic bool,
	index uint64,
	value string,
) *InstructionWithValues {
	return &InstructionWithValues{
		instruction: LiteralHeaderFieldNameReferenceInstruction(),
		sBit:        isStatic,
		varint:      index,
		value:       value,
	}
}

func InstructionWithValuesLiteralHeaderField(name, value string) *InstructionWithValues {
	return &InstructionWithValues{
		instruction: LiteralHeaderFieldInstruction(),
		name:        name,
		value:       value,
	}
}

func InstructionWithValuesIndexedHeaderFieldPostBase(postBaseIndex uint64) *InstructionWithValues {
	return &InstructionWithValues{
		instruction: IndexedHeaderFieldPostBaseInstruction(),
		varint:      postBaseIndex,
	}
}

func InstructionWithValuesLiteralHeaderFieldPostBaseNameReference(
	postBaseNameIndex uint64,
	value string,
) *InstructionWithValues {
	return &InstructionWithValues{
		instruction: LiteralHeaderFieldPostBaseNameReferenceInstruction(),
		varint:      postBaseNameIndex,
		value:       value,
	}
}
