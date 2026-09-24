// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package qpack

import (
	"fmt"

	"golang.org/x/net/http2/hpack"
)

// HuffmanEncoding specifies whether Huffman compression is attempted.
// Direct 1:1 translation of Chromium's quiche::InstructionEncoder::HuffmanEncoding.
type HuffmanEncoding int

const (
	HuffmanEncodingEnabled HuffmanEncoding = iota
	HuffmanEncodingDisabled
)

func (h HuffmanEncoding) String() string {
	switch h {
	case HuffmanEncodingEnabled:
		return "HuffmanEnabled"
	case HuffmanEncodingDisabled:
		return "HuffmanDisabled"
	default:
		return fmt.Sprintf("HuffmanEncoding(%d)", int(h))
	}
}

type qpackInstructionEncoderState int

const (
	encoderStateOpcode qpackInstructionEncoderState = iota
	encoderStateStartField
	encoderStateSbit
	encoderStateStartString
	encoderStateVarintEncode
	encoderStateWriteString
	encoderStateDone
)

// InstructionEncoder serializes QPACK instructions into wire bytes.
// Direct 1:1 structural translation of Chromium's quiche::InstructionEncoder.
type InstructionEncoder struct {
	huffman HuffmanEncoding

	state        qpackInstructionEncoderState
	instruction  *Instruction
	fieldIdx     int
	byte_        byte
	useHuffman   bool
	stringLength uint64
	stringVal    string
}

// NewInstructionEncoder creates a new encoder with the specified Huffman encoding policy.
func NewInstructionEncoder(huffman HuffmanEncoding) *InstructionEncoder {
	return &InstructionEncoder{
		huffman: huffman,
	}
}

// Encode encodes instructionWithValues and appends the result to output, returning the slice.
func (e *InstructionEncoder) Encode(instructionWithValues *InstructionWithValues, output []byte) []byte {
	e.EncodeTo(instructionWithValues, &output)
	return output
}

// EncodeTo encodes instructionWithValues directly into the provided slice pointer.
func (e *InstructionEncoder) EncodeTo(instructionWithValues *InstructionWithValues, output *[]byte) {
	e.state = encoderStateOpcode
	e.instruction = instructionWithValues.Instruction()
	e.fieldIdx = 0
	e.byte_ = 0

	for e.state != encoderStateDone {
		switch e.state {
		case encoderStateOpcode:
			e.doOpcode()
		case encoderStateStartField:
			e.doStartField()
		case encoderStateSbit:
			e.doSbit(instructionWithValues.SBit())
		case encoderStateStartString:
			e.doStartString(instructionWithValues)
		case encoderStateVarintEncode:
			e.doVarintEncode(instructionWithValues, output)
		case encoderStateWriteString:
			e.doWriteString(output)
		}
	}
}

func (e *InstructionEncoder) doOpcode() {
	e.byte_ = e.instruction.Opcode.Value
	e.state = encoderStateStartField
}

func (e *InstructionEncoder) doStartField() {
	if e.fieldIdx == len(e.instruction.Fields) {
		e.state = encoderStateDone
		return
	}

	field := e.instruction.Fields[e.fieldIdx]
	switch field.Type {
	case InstructionFieldTypeSbit:
		e.state = encoderStateSbit
	case InstructionFieldTypeVarint, InstructionFieldTypeVarint2:
		e.state = encoderStateVarintEncode
	case InstructionFieldTypeName, InstructionFieldTypeValue:
		e.state = encoderStateStartString
	}
}

func (e *InstructionEncoder) doSbit(sBit bool) {
	field := e.instruction.Fields[e.fieldIdx]
	if sBit {
		e.byte_ |= field.Param
	}
	e.fieldIdx++
	e.state = encoderStateStartField
}

func (e *InstructionEncoder) doStartString(instWithValues *InstructionWithValues) {
	field := e.instruction.Fields[e.fieldIdx]
	if field.Type == InstructionFieldTypeName {
		e.stringVal = instWithValues.Name()
	} else {
		e.stringVal = instWithValues.Value()
	}

	huffmanLen := hpack.HuffmanEncodeLength(e.stringVal)
	// Chromium Huffman decision rule: strictly less than unencoded size.
	if e.huffman == HuffmanEncodingEnabled && huffmanLen < uint64(len(e.stringVal)) {
		e.useHuffman = true
		e.byte_ |= (1 << field.Param)
		e.stringLength = huffmanLen
	} else {
		e.useHuffman = false
		e.stringLength = uint64(len(e.stringVal))
	}

	e.state = encoderStateVarintEncode
}

func (e *InstructionEncoder) doVarintEncode(instWithValues *InstructionWithValues, output *[]byte) {
	field := e.instruction.Fields[e.fieldIdx]

	var integerToEncode uint64
	switch field.Type {
	case InstructionFieldTypeVarint:
		integerToEncode = instWithValues.Varint()
	case InstructionFieldTypeVarint2:
		integerToEncode = instWithValues.Varint2()
	case InstructionFieldTypeName, InstructionFieldTypeValue:
		integerToEncode = e.stringLength
	}

	qpackEncodeVarint(e.byte_, field.Param, integerToEncode, output)
	e.byte_ = 0

	switch field.Type {
	case InstructionFieldTypeVarint, InstructionFieldTypeVarint2:
		e.fieldIdx++
		e.state = encoderStateStartField
	case InstructionFieldTypeName, InstructionFieldTypeValue:
		e.state = encoderStateWriteString
	}
}

func (e *InstructionEncoder) doWriteString(output *[]byte) {
	if e.useHuffman {
		*output = hpack.AppendHuffmanString(*output, e.stringVal)
	} else {
		*output = append(*output, e.stringVal...)
	}

	e.fieldIdx++
	e.state = encoderStateStartField
}

func qpackEncodeVarint(highBits, prefixLength uint8, varint uint64, output *[]byte) {
	prefixMask := uint8((1 << prefixLength) - 1)
	if varint < uint64(prefixMask) {
		*output = append(*output, highBits|uint8(varint))
		return
	}

	*output = append(*output, highBits|prefixMask)
	varint -= uint64(prefixMask)

	for varint >= 128 {
		*output = append(*output, uint8(0x80|(varint%128)))
		varint >>= 7
	}
	*output = append(*output, uint8(varint))
}
