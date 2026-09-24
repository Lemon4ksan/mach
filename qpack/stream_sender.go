// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package qpack

import "io"

const maxBytesBufferedByStream uint64 = 64 * 1024

// EncoderStreamSender serializes instructions for the encoder stream (RFC 9204 §4.3).
type EncoderStreamSender struct {
	delegate           StreamSenderDelegate
	instructionEncoder *InstructionEncoder
	buffer             []byte
}

// NewEncoderStreamSender creates a new EncoderStreamSender with default
// Huffman encoding (enabled) and the specified delegate.
func NewEncoderStreamSender(delegate StreamSenderDelegate) *EncoderStreamSender {
	return NewEncoderStreamSenderWithHuffman(HuffmanEncodingEnabled, delegate)
}

// NewEncoderStreamSenderWithHuffman creates a new EncoderStreamSender
// with an explicit Huffman encoding policy.
func NewEncoderStreamSenderWithHuffman(huffman HuffmanEncoding, delegate StreamSenderDelegate) *EncoderStreamSender {
	return &EncoderStreamSender{
		delegate:           delegate,
		instructionEncoder: NewInstructionEncoder(huffman),
		buffer:             make([]byte, 0),
	}
}

// Delegate returns the current stream sender delegate.
func (s *EncoderStreamSender) Delegate() StreamSenderDelegate {
	return s.delegate
}

// SetDelegate sets the stream sender delegate.
func (s *EncoderStreamSender) SetDelegate(delegate StreamSenderDelegate) {
	s.delegate = delegate
}

// SendInsertWithNameReference encodes an Insert with Name Reference instruction.
// RFC 9204 §4.3.1.
func (s *EncoderStreamSender) SendInsertWithNameReference(isStatic bool, nameIndex uint64, value string) {
	instruction := InstructionWithValuesInsertWithNameReference(isStatic, nameIndex, value)
	s.instructionEncoder.EncodeTo(instruction, &s.buffer)
}

// SendInsertWithoutNameReference encodes an Insert Without Name Reference instruction.
// RFC 9204 §4.3.2.
func (s *EncoderStreamSender) SendInsertWithoutNameReference(name, value string) {
	instruction := InstructionWithValuesInsertWithoutNameReference(name, value)
	s.instructionEncoder.EncodeTo(instruction, &s.buffer)
}

// SendDuplicate encodes a Duplicate instruction.
// RFC 9204 §4.3.3.
func (s *EncoderStreamSender) SendDuplicate(index uint64) {
	instruction := InstructionWithValuesDuplicate(index)
	s.instructionEncoder.EncodeTo(instruction, &s.buffer)
}

// SendSetDynamicTableCapacity encodes a Set Dynamic Table Capacity instruction.
// RFC 9204 §4.3.4.
func (s *EncoderStreamSender) SendSetDynamicTableCapacity(capacity uint64) {
	instruction := InstructionWithValuesSetDynamicTableCapacity(capacity)
	s.instructionEncoder.EncodeTo(instruction, &s.buffer)
}

// BufferedByteCount returns the number of bytes currently buffered in the sender.
func (s *EncoderStreamSender) BufferedByteCount() uint64 {
	return uint64(len(s.buffer))
}

// CanWrite returns whether writing to the encoder stream is allowed.
func (s *EncoderStreamSender) CanWrite() bool {
	return s.delegate != nil && s.delegate.NumBytesBuffered()+uint64(len(s.buffer)) <= maxBytesBufferedByStream
}

// Flush writes any buffered instructions to the delegate stream and clears the buffer.
// If the buffer is empty, no call to WriteStreamData is made.
func (s *EncoderStreamSender) Flush() {
	if len(s.buffer) == 0 {
		return
	}
	data := s.buffer
	s.buffer = nil
	if s.delegate != nil {
		s.delegate.WriteStreamData(data)
	}
}

// FlushTo writes buffered instructions to w and clears the internal buffer.
// If the buffer is empty, it returns nil without writing.
func (s *EncoderStreamSender) FlushTo(w io.Writer) error {
	if len(s.buffer) == 0 {
		return nil
	}
	data := s.buffer
	s.buffer = nil
	_, err := w.Write(data)
	return err
}

// -----------------------------------------------------------------------------
// DecoderStreamSender
// -----------------------------------------------------------------------------

// DecoderStreamSender serializes instructions for the decoder stream (RFC 9204 §4.4).
type DecoderStreamSender struct {
	delegate           StreamSenderDelegate
	instructionEncoder *InstructionEncoder
	buffer             []byte
}

// NewDecoderStreamSender creates a new DecoderStreamSender.
// Decoder stream instructions only contain integer values, so Huffman encoding is disabled.
func NewDecoderStreamSender(delegate StreamSenderDelegate) *DecoderStreamSender {
	return &DecoderStreamSender{
		delegate:           delegate,
		instructionEncoder: NewInstructionEncoder(HuffmanEncodingDisabled),
		buffer:             make([]byte, 0),
	}
}

// Delegate returns the current stream sender delegate.
func (s *DecoderStreamSender) Delegate() StreamSenderDelegate {
	return s.delegate
}

// SetDelegate sets the stream sender delegate.
func (s *DecoderStreamSender) SetDelegate(delegate StreamSenderDelegate) {
	s.delegate = delegate
}

// SendSectionAcknowledgement encodes a Section Acknowledgment instruction.
// RFC 9204 §4.4.1.
func (s *DecoderStreamSender) SendSectionAcknowledgement(streamId uint64) {
	instruction := InstructionWithValuesHeaderAcknowledgement(streamId)
	s.instructionEncoder.EncodeTo(instruction, &s.buffer)
}

// SendHeaderAcknowledgement is an alias for SendSectionAcknowledgement matching Chromium C++ naming.
func (s *DecoderStreamSender) SendHeaderAcknowledgement(streamId uint64) {
	s.SendSectionAcknowledgement(streamId)
}

// SendStreamCancellation encodes a Stream Cancellation instruction.
// RFC 9204 §4.4.2.
func (s *DecoderStreamSender) SendStreamCancellation(streamId uint64) {
	instruction := InstructionWithValuesStreamCancellation(streamId)
	s.instructionEncoder.EncodeTo(instruction, &s.buffer)
}

// SendInsertCountIncrement encodes an Insert Count Increment instruction.
// RFC 9204 §4.4.3.
func (s *DecoderStreamSender) SendInsertCountIncrement(increment uint64) {
	instruction := InstructionWithValuesInsertCountIncrement(increment)
	s.instructionEncoder.EncodeTo(instruction, &s.buffer)
}

// Flush writes any buffered instructions to the delegate stream and clears the buffer.
// If the buffer is empty, no call to WriteStreamData is made.
func (s *DecoderStreamSender) Flush() {
	if len(s.buffer) == 0 {
		return
	}
	data := s.buffer
	s.buffer = nil
	if s.delegate != nil {
		s.delegate.WriteStreamData(data)
	}
}

// FlushTo writes buffered instructions to w and clears the internal buffer.
// If the buffer is empty, it returns nil without writing.
func (s *DecoderStreamSender) FlushTo(w io.Writer) error {
	if len(s.buffer) == 0 {
		return nil
	}
	data := s.buffer
	s.buffer = nil
	_, err := w.Write(data)
	return err
}
