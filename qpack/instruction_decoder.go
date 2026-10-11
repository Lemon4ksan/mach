// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package qpack

import (
	"math"

	"golang.org/x/net/http2/hpack"
)

// String literal length limit: 1 MB (1024 * 1024 bytes).
const qpackStringLiteralLengthLimit uint64 = 1024 * 1024

// InstructionDecoderErrorCode identifies error conditions encountered during instruction decoding.
type InstructionDecoderErrorCode = uint64

const (
	// InstructionDecoderIntegerTooLarge indicates a varint exceeds 64 bits or has more than 10 extension bytes.
	InstructionDecoderIntegerTooLarge InstructionDecoderErrorCode = iota
	// InstructionDecoderStringLiteralTooLong indicates a string literal exceeds the 1 MB size limit.
	InstructionDecoderStringLiteralTooLong
	// InstructionDecoderHuffmanEncodingError indicates malformed Huffman code, explicit EOS, or invalid padding.
	InstructionDecoderHuffmanEncodingError
)

// Aliases for parity across naming styles.
const (
	// InstructionDecoderErrorCodeIntegerTooLarge is an alias for [InstructionDecoderIntegerTooLarge].
	InstructionDecoderErrorCodeIntegerTooLarge = InstructionDecoderIntegerTooLarge
	// InstructionDecoderErrorCodeStringLiteralTooLong is an alias for [InstructionDecoderStringLiteralTooLong].
	InstructionDecoderErrorCodeStringLiteralTooLong = InstructionDecoderStringLiteralTooLong
	// InstructionDecoderErrorCodeHuffmanEncodingError is an alias for [InstructionDecoderHuffmanEncodingError].
	InstructionDecoderErrorCodeHuffmanEncodingError = InstructionDecoderHuffmanEncodingError
)

// InstructionDecoderDelegate provides callback methods for decoded instructions and decoding errors.
type InstructionDecoderDelegate interface {
	// OnInstructionDecoded is called when an [Instruction] and all its fields are fully decoded.
	// Returning false halts decoding immediately.
	OnInstructionDecoded(instruction *Instruction) bool

	// OnInstructionDecodingError is called when a decoding error occurs.
	OnInstructionDecodingError(errorCode InstructionDecoderErrorCode, errorMessage string)
}

// -----------------------------------------------------------------------------
// Streaming Varint Decoder Helper
// -----------------------------------------------------------------------------

type decodeStatus int

const (
	decodeStatusDone decodeStatus = iota
	decodeStatusInProgress
	decodeStatusError
)

// qpackVarintDecoder decodes variable-length integers progressively.
// Direct translation of Chromium's HpackVarintDecoder.
type qpackVarintDecoder struct {
	value         uint64
	offset        uint8
	done          bool
	errorDetected bool
}

func (v *qpackVarintDecoder) Reset() {
	v.value = 0
	v.offset = 0
	v.done = false
	v.errorDetected = false
}

func (v *qpackVarintDecoder) Value() uint64 { return v.value }
func (v *qpackVarintDecoder) Done() bool    { return v.done }
func (v *qpackVarintDecoder) Error() bool   { return v.errorDetected }

// Start begins decoding a varint from data, using prefixLength bits from data[0].
// Returns the number of bytes consumed from data and status.
func (v *qpackVarintDecoder) Start(prefixLength uint8, data []byte) (int, decodeStatus) {
	v.Reset()

	if len(data) == 0 {
		return 0, decodeStatusInProgress
	}

	prefixMask := uint8((1 << prefixLength) - 1)
	v.value = uint64(data[0] & prefixMask)

	if v.value < uint64(prefixMask) {
		v.done = true
		return 1, decodeStatusDone
	}

	v.offset = 0
	consumed, status := v.Resume(data[1:])

	return 1 + consumed, status
}

// Resume continues decoding continuation bytes.
func (v *qpackVarintDecoder) Resume(data []byte) (int, decodeStatus) {
	const maxOffset = 63

	consumed := 0

	for v.offset < maxOffset {
		if consumed >= len(data) {
			return consumed, decodeStatusInProgress
		}

		b := data[consumed]
		consumed++

		summand := uint64(b & 0x7f)
		v.value += summand << v.offset

		if (b & 0x80) == 0 {
			v.done = true
			return consumed, decodeStatusDone
		}

		v.offset += 7
	}

	if consumed >= len(data) {
		return consumed, decodeStatusInProgress
	}

	// 10th extension byte (offset == 63)
	b := data[consumed]
	consumed++

	if (b & 0x80) == 0 {
		summand := uint64(b & 0x7f)
		if summand <= 1 {
			summand <<= 63
			if v.value <= math.MaxUint64-summand {
				v.value += summand
				v.done = true
				return consumed, decodeStatusDone
			}
		}
	}

	v.errorDetected = true

	return consumed, decodeStatusError
}

// -----------------------------------------------------------------------------
// InstructionDecoder (8-State Machine)
// -----------------------------------------------------------------------------

type qpackInstructionDecoderState int

const (
	stateStartInstruction qpackInstructionDecoderState = iota
	stateStartField
	stateReadBit
	stateVarintStart
	stateVarintResume
	stateVarintDone
	stateReadString
	stateReadStringDone
)

// InstructionDecoder parses stream bytes into QPACK instructions via an 8-state machine.
// The zero value is not ready for use; initialize with [NewInstructionDecoder].
type InstructionDecoder struct {
	language *Language
	delegate InstructionDecoderDelegate

	sBit             bool
	varint           uint64
	varint2          uint64
	name             string
	value            string
	isHuffmanEncoded bool
	stringLength     uint64
	stringBuffer     []byte

	varintDecoder qpackVarintDecoder
	errorDetected bool
	state         qpackInstructionDecoderState
	instruction   *Instruction
	fieldIdx      int
}

// NewInstructionDecoder creates an initialized [InstructionDecoder] for the given language.
func NewInstructionDecoder(language *Language, delegate InstructionDecoderDelegate) *InstructionDecoder {
	return &InstructionDecoder{
		language: language,
		delegate: delegate,
		state:    stateStartInstruction,
	}
}

// SBit returns the decoded static/sign bit from the current instruction.
func (d *InstructionDecoder) SBit() bool      { return d.sBit }

// Varint returns the first decoded variable-length integer.
func (d *InstructionDecoder) Varint() uint64  { return d.varint }

// Varint2 returns the second decoded variable-length integer.
func (d *InstructionDecoder) Varint2() uint64 { return d.varint2 }

// Name returns the decoded literal header name.
func (d *InstructionDecoder) Name() string    { return d.name }

// Value returns the decoded literal header value.
func (d *InstructionDecoder) Value() string   { return d.value }

// AtInstructionBoundary reports whether the decoder is currently positioned between instructions.
func (d *InstructionDecoder) AtInstructionBoundary() bool {
	return d.state == stateStartInstruction
}

// HasError reports whether a decoding error has been detected.
func (d *InstructionDecoder) HasError() bool {
	return d.errorDetected
}

// EndDecoding signals that no more data is expected on the stream.
// If the decoder is not at an instruction boundary, an error is reported.
func (d *InstructionDecoder) EndDecoding() {
	if !d.AtInstructionBoundary() && !d.errorDetected {
		d.onError(InstructionDecoderIntegerTooLarge, "Truncated instruction.")
	}
}

func (d *InstructionDecoder) lookupOpcode(b byte) *Instruction {
	if d.language == nil {
		return nil
	}

	for _, instruction := range *d.language {
		if (b & instruction.Opcode.Mask) == instruction.Opcode.Value {
			return instruction
		}
	}

	return nil
}

func (d *InstructionDecoder) onError(errorCode InstructionDecoderErrorCode, errorMessage string) {
	d.errorDetected = true
	d.delegate.OnInstructionDecodingError(errorCode, errorMessage)
}

// Decode feeds data to the streaming state machine.
// Returns true on success (even if more data is needed).
// Returns false if a decoding or delegate error occurred.
func (d *InstructionDecoder) Decode(data []byte) bool {
	if d.errorDetected {
		return false
	}

	for len(data) > 0 ||
		d.state == stateStartField ||
		d.state == stateVarintDone ||
		d.state == stateReadStringDone {
		switch d.state {
		case stateStartInstruction:
			d.doStartInstruction(data)
		case stateStartField:
			d.doStartField()
		case stateReadBit:
			d.doReadBit(data)
		case stateVarintStart:
			consumed := d.doVarintStart(data)
			data = data[consumed:]
		case stateVarintResume:
			consumed := d.doVarintResume(data)
			data = data[consumed:]
		case stateVarintDone:
			d.doVarintDone()
		case stateReadString:
			consumed := d.doReadString(data)
			data = data[consumed:]
		case stateReadStringDone:
			d.doReadStringDone()
		}

		if d.errorDetected {
			return false
		}
	}

	return true
}

func (d *InstructionDecoder) doStartInstruction(data []byte) {
	d.instruction = d.lookupOpcode(data[0])
	if d.instruction == nil {
		d.onError(InstructionDecoderIntegerTooLarge, "Invalid opcode.")
		return
	}

	d.sBit = false
	d.varint = 0
	d.varint2 = 0
	d.name = ""
	d.value = ""
	d.isHuffmanEncoded = false
	d.fieldIdx = 0
	d.state = stateStartField
}

func (d *InstructionDecoder) doStartField() {
	if d.fieldIdx == len(d.instruction.Fields) {
		d.state = stateStartInstruction
		if !d.delegate.OnInstructionDecoded(d.instruction) {
			d.errorDetected = true
		}

		return
	}

	field := d.instruction.Fields[d.fieldIdx]
	switch field.Type {
	case InstructionFieldTypeSbit, InstructionFieldTypeName, InstructionFieldTypeValue:
		d.state = stateReadBit
	case InstructionFieldTypeVarint, InstructionFieldTypeVarint2:
		d.state = stateVarintStart
	}
}

func (d *InstructionDecoder) doReadBit(data []byte) {
	field := d.instruction.Fields[d.fieldIdx]
	switch field.Type {
	case InstructionFieldTypeSbit:
		d.sBit = (data[0] & field.Param) != 0
		d.fieldIdx++
		d.state = stateStartField
	case InstructionFieldTypeName, InstructionFieldTypeValue:
		d.isHuffmanEncoded = (data[0] & (1 << field.Param)) != 0
		d.state = stateVarintStart
	}
}

func (d *InstructionDecoder) doVarintStart(data []byte) int {
	field := d.instruction.Fields[d.fieldIdx]

	consumed, status := d.varintDecoder.Start(field.Param, data)
	if status == decodeStatusError {
		d.onError(InstructionDecoderIntegerTooLarge, "Encoded integer too large.")
		return consumed
	}

	if status == decodeStatusDone {
		d.state = stateVarintDone
		return consumed
	}

	d.state = stateVarintResume

	return consumed
}

func (d *InstructionDecoder) doVarintResume(data []byte) int {
	consumed, status := d.varintDecoder.Resume(data)
	if status == decodeStatusError {
		d.onError(InstructionDecoderIntegerTooLarge, "Encoded integer too large.")
		return consumed
	}

	if status == decodeStatusDone {
		d.state = stateVarintDone
		return consumed
	}

	return consumed
}

func (d *InstructionDecoder) doVarintDone() {
	field := d.instruction.Fields[d.fieldIdx]
	switch field.Type {
	case InstructionFieldTypeVarint:
		d.varint = d.varintDecoder.Value()
		d.fieldIdx++
		d.state = stateStartField
	case InstructionFieldTypeVarint2:
		d.varint2 = d.varintDecoder.Value()
		d.fieldIdx++
		d.state = stateStartField
	case InstructionFieldTypeName, InstructionFieldTypeValue:
		if d.varintDecoder.Value() > qpackStringLiteralLengthLimit {
			d.onError(InstructionDecoderStringLiteralTooLong, "String literal too long.")
			return
		}

		d.stringLength = d.varintDecoder.Value()

		d.stringBuffer = d.stringBuffer[:0]
		if d.stringLength == 0 {
			d.state = stateReadStringDone
		} else {
			d.state = stateReadString
		}
	}
}

func (d *InstructionDecoder) doReadString(data []byte) int {
	remainingNeeded := int(d.stringLength) - len(d.stringBuffer)
	toRead := min(len(data), remainingNeeded)

	d.stringBuffer = append(d.stringBuffer, data[:toRead]...)
	if uint64(len(d.stringBuffer)) == d.stringLength {
		d.state = stateReadStringDone
	}

	return toRead
}

func (d *InstructionDecoder) doReadStringDone() {
	var str string
	if d.isHuffmanEncoded {
		decoded, err := hpack.HuffmanDecodeToString(d.stringBuffer)
		if err != nil {
			d.onError(InstructionDecoderHuffmanEncodingError, "Error in Huffman-encoded string.")
			return
		}

		str = decoded
	} else {
		str = string(d.stringBuffer)
	}

	field := d.instruction.Fields[d.fieldIdx]
	switch field.Type {
	case InstructionFieldTypeName:
		d.name = str
	case InstructionFieldTypeValue:
		d.value = str
	}

	d.fieldIdx++
	d.state = stateStartField
}
