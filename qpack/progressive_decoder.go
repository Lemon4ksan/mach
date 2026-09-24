// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package qpack

import (
	"io"
	"math"
)

// HeadersHandlerInterface receives decoded header block fields from the progressive decoder.
type HeadersHandlerInterface interface {
	// OnHeaderDecoded is called when a new header name-value pair is decoded.
	// Multiple values for a given name will be emitted as multiple calls to OnHeaderDecoded.
	OnHeaderDecoded(name, value string)

	// OnDecodingCompleted is called when the header block is completely decoded.
	// The decoder will not access the handler after this call.
	// Note that this method might not be called synchronously when the header block
	// is received on the wire, in case decoding is blocked on receiving entries on the encoder stream.
	OnDecodingCompleted()

	OnDecodingErrorDetected(errorCode uint64, errorMessage string)
}

// BlockedStreamLimitEnforcer keeps track of blocked streams for enforcing QPACK_BLOCKED_STREAMS.
type BlockedStreamLimitEnforcer interface {
	// OnStreamBlocked is called when the stream becomes blocked.
	// Returns true if allowed, or false if the limit is violated.
	OnStreamBlocked(streamID uint64) bool

	// OnStreamUnblocked is called when the stream becomes unblocked.
	OnStreamUnblocked(streamID uint64)
}

// DecodingCompletedVisitor is notified when decoding of a header block is completed.
type DecodingCompletedVisitor interface {
	// OnDecodingCompleted is called when decoding is completed with the block's Required Insert Count.
	OnDecodingCompleted(streamID, requiredInsertCount uint64)
}

// ProgressiveDecoder decodes a single header block progressively (RFC 9204 §4.5).
type ProgressiveDecoder struct {
	streamID        uint64
	maxBufferedData uint64

	prefixDecoder      *InstructionDecoder
	instructionDecoder *InstructionDecoder

	enforcer    BlockedStreamLimitEnforcer
	visitor     DecodingCompletedVisitor
	headerTable *DecoderHeaderTable
	handler     HeadersHandlerInterface

	requiredInsertCount      uint64
	base                     uint64
	requiredInsertCountSoFar uint64

	prefixDecoded bool
	blocked       bool
	buffer        []byte
	decoding      bool
	errorDetected bool
	cancelled     bool
	lastErr       error
}

// NewProgressiveDecoder creates a new progressive decoder for streamID.
func NewProgressiveDecoder(
	streamID uint64,
	maxBufferedData uint64,
	enforcer BlockedStreamLimitEnforcer,
	visitor DecodingCompletedVisitor,
	headerTable *DecoderHeaderTable,
	handler HeadersHandlerInterface,
) *ProgressiveDecoder {
	d := &ProgressiveDecoder{
		streamID:        streamID,
		maxBufferedData: maxBufferedData,
		enforcer:        enforcer,
		visitor:         visitor,
		headerTable:     headerTable,
		handler:         handler,
		decoding:        true,
	}

	d.prefixDecoder = NewInstructionDecoder(PrefixLanguage(), d)
	d.instructionDecoder = NewInstructionDecoder(RequestStreamLanguage(), d)

	return d
}

// Decode feeds a data fragment into the progressive decoder.
func (d *ProgressiveDecoder) Decode(data []byte) {
	if !d.decoding || len(data) == 0 || d.errorDetected {
		return
	}

	// Decode prefix byte by byte until the first (and only) instruction is decoded.
	for !d.prefixDecoded {
		if !d.prefixDecoder.Decode(data[:1]) {
			return
		}

		if d.errorDetected {
			return
		}

		data = data[1:]
		if len(data) == 0 {
			return
		}
	}

	if d.blocked {
		if d.maxBufferedData > 0 && uint64(len(d.buffer)+len(data)) > d.maxBufferedData {
			d.onError(QPACK_DECOMPRESSION_FAILED, "Too much buffered data.")
			return
		}

		d.buffer = append(d.buffer, data...)
	} else {
		d.instructionDecoder.Decode(data)
	}
}

// Write implements io.Writer, feeding data into the progressive decoder.
// Returns the number of bytes written, or (0, error) if decoding previously failed or fails during processing.
func (d *ProgressiveDecoder) Write(p []byte) (n int, err error) {
	if d.errorDetected {
		if d.lastErr != nil {
			return 0, d.lastErr
		}

		return 0, ErrDecompressionFailed
	}

	d.Decode(p)

	if d.errorDetected {
		if d.lastErr != nil {
			return 0, d.lastErr
		}

		return 0, ErrDecompressionFailed
	}

	return len(p), nil
}

// EndHeaderBlock signals that the entire header block has been delivered.
// No methods must be called on this instance afterwards.
func (d *ProgressiveDecoder) EndHeaderBlock() {
	d.decoding = false

	if !d.blocked {
		d.finishDecoding()
	}
}

// EndDecoding is an alias for EndHeaderBlock.
func (d *ProgressiveDecoder) EndDecoding() {
	d.EndHeaderBlock()
}

// OnInstructionDecoded implements InstructionDecoderDelegate.
func (d *ProgressiveDecoder) OnInstructionDecoded(instruction *Instruction) bool {
	if instruction == PrefixInstruction() {
		return d.doPrefixInstruction()
	}

	if instruction == IndexedHeaderFieldInstruction() {
		return d.doIndexedHeaderFieldInstruction()
	}

	if instruction == IndexedHeaderFieldPostBaseInstruction() {
		return d.doIndexedHeaderFieldPostBaseInstruction()
	}

	if instruction == LiteralHeaderFieldNameReferenceInstruction() {
		return d.doLiteralHeaderFieldNameReferenceInstruction()
	}

	if instruction == LiteralHeaderFieldPostBaseNameReferenceInstruction() {
		return d.doLiteralHeaderFieldPostBaseInstruction()
	}

	if instruction == LiteralHeaderFieldInstruction() {
		return d.doLiteralHeaderFieldInstruction()
	}

	return false
}

// OnInstructionDecodingError implements InstructionDecoderDelegate.
func (d *ProgressiveDecoder) OnInstructionDecodingError(
	errorCode InstructionDecoderErrorCode,
	errorMessage string,
) {
	// Ignore errorCode and always use QPACK_DECOMPRESSION_FAILED matching Chromium.
	d.onError(QPACK_DECOMPRESSION_FAILED, errorMessage)
}

// OnInsertCountReachedThreshold implements DecoderHeaderTableObserver.
func (d *ProgressiveDecoder) OnInsertCountReachedThreshold() {
	d.blocked = false
	d.enforcer.OnStreamUnblocked(d.streamID)

	if len(d.buffer) > 0 {
		buf := d.buffer

		d.buffer = nil
		if !d.instructionDecoder.Decode(buf) {
			return
		}
	}

	if !d.decoding {
		d.finishDecoding()
	}
}

// Cancel implements DecoderHeaderTableObserver.
func (d *ProgressiveDecoder) Cancel() {
	d.cancelled = true
}

// Close implements io.Closer. If decoding is still active and the stream has not been cancelled,
// it signals EndHeaderBlock(). If blocked, it unregisters table observers and unblocks stream tracking.
// Returns an error if decompression failed.
func (d *ProgressiveDecoder) Close() error {
	if d.decoding && !d.cancelled && !d.blocked {
		d.EndHeaderBlock()
	}

	if d.blocked {
		if d.enforcer != nil {
			d.enforcer.OnStreamUnblocked(d.streamID)
		}

		if !d.cancelled && d.headerTable != nil {
			d.headerTable.UnregisterObserver(d.requiredInsertCount, d)
		}

		d.blocked = false
		d.buffer = nil
	}

	if d.errorDetected {
		if d.lastErr != nil {
			return d.lastErr
		}

		return ErrDecompressionFailed
	}

	return nil
}

// Err returns the first error detected during decoding, or nil if no error occurred.
func (d *ProgressiveDecoder) Err() error {
	return d.lastErr
}

func (d *ProgressiveDecoder) doPrefixInstruction() bool {
	ric, ok := DecodeRequiredInsertCount(
		d.prefixDecoder.Varint(),
		d.headerTable.MaxEntries(),
		d.headerTable.InsertedEntryCount(),
	)
	if !ok {
		d.onError(QPACK_DECOMPRESSION_FAILED, "Error decoding Required Insert Count.")
		return false
	}

	d.requiredInsertCount = ric

	sign := d.prefixDecoder.SBit()
	deltaBase := d.prefixDecoder.Varint2()

	base, ok := d.deltaBaseToBase(sign, deltaBase)
	if !ok {
		d.onError(QPACK_DECOMPRESSION_FAILED, "Error calculating Base.")
		return false
	}

	d.base = base

	d.prefixDecoded = true

	if d.requiredInsertCount > d.headerTable.InsertedEntryCount() {
		if !d.enforcer.OnStreamBlocked(d.streamID) {
			d.onError(QPACK_DECOMPRESSION_FAILED, "Limit on number of blocked streams exceeded.")
			return false
		}

		d.blocked = true
		d.headerTable.RegisterObserver(d.requiredInsertCount, d)
	}

	return true
}

func (d *ProgressiveDecoder) doIndexedHeaderFieldInstruction() bool {
	if !d.instructionDecoder.SBit() {
		absoluteIndex, ok := RequestStreamRelativeIndexToAbsoluteIndex(
			d.instructionDecoder.Varint(), d.base,
		)
		if !ok {
			d.onError(QPACK_DECOMPRESSION_FAILED, "Invalid relative index.")
			return false
		}

		if absoluteIndex >= d.requiredInsertCount {
			d.onError(QPACK_DECOMPRESSION_FAILED, "Absolute Index must be smaller than Required Insert Count.")
			return false
		}

		if absoluteIndex+1 > d.requiredInsertCountSoFar {
			d.requiredInsertCountSoFar = absoluteIndex + 1
		}

		entry := d.headerTable.LookupEntry(false, absoluteIndex)
		if entry == nil {
			d.onError(QPACK_DECOMPRESSION_FAILED, "Dynamic table entry already evicted.")
			return false
		}

		d.headerTable.SetDynamicTableEntryReferenced()

		return d.onHeaderDecoded(false, entry.Name, entry.Value)
	}

	entry := d.headerTable.LookupEntry(true, d.instructionDecoder.Varint())
	if entry == nil {
		d.onError(QPACK_DECOMPRESSION_FAILED, "Static table entry not found.")
		return false
	}

	return d.onHeaderDecoded(true, entry.Name, entry.Value)
}

func (d *ProgressiveDecoder) doIndexedHeaderFieldPostBaseInstruction() bool {
	absoluteIndex, ok := PostBaseIndexToAbsoluteIndex(
		d.instructionDecoder.Varint(), d.base,
	)
	if !ok {
		d.onError(QPACK_DECOMPRESSION_FAILED, "Invalid post-base index.")
		return false
	}

	if absoluteIndex >= d.requiredInsertCount {
		d.onError(QPACK_DECOMPRESSION_FAILED, "Absolute Index must be smaller than Required Insert Count.")
		return false
	}

	if absoluteIndex+1 > d.requiredInsertCountSoFar {
		d.requiredInsertCountSoFar = absoluteIndex + 1
	}

	entry := d.headerTable.LookupEntry(false, absoluteIndex)
	if entry == nil {
		d.onError(QPACK_DECOMPRESSION_FAILED, "Dynamic table entry already evicted.")
		return false
	}

	d.headerTable.SetDynamicTableEntryReferenced()

	return d.onHeaderDecoded(false, entry.Name, entry.Value)
}

func (d *ProgressiveDecoder) doLiteralHeaderFieldNameReferenceInstruction() bool {
	if !d.instructionDecoder.SBit() {
		absoluteIndex, ok := RequestStreamRelativeIndexToAbsoluteIndex(
			d.instructionDecoder.Varint(), d.base,
		)
		if !ok {
			d.onError(QPACK_DECOMPRESSION_FAILED, "Invalid relative index.")
			return false
		}

		if absoluteIndex >= d.requiredInsertCount {
			d.onError(QPACK_DECOMPRESSION_FAILED, "Absolute Index must be smaller than Required Insert Count.")
			return false
		}

		if absoluteIndex+1 > d.requiredInsertCountSoFar {
			d.requiredInsertCountSoFar = absoluteIndex + 1
		}

		entry := d.headerTable.LookupEntry(false, absoluteIndex)
		if entry == nil {
			d.onError(QPACK_DECOMPRESSION_FAILED, "Dynamic table entry already evicted.")
			return false
		}

		d.headerTable.SetDynamicTableEntryReferenced()

		return d.onHeaderDecoded(false, entry.Name, d.instructionDecoder.Value())
	}

	entry := d.headerTable.LookupEntry(true, d.instructionDecoder.Varint())
	if entry == nil {
		d.onError(QPACK_DECOMPRESSION_FAILED, "Static table entry not found.")
		return false
	}

	return d.onHeaderDecoded(true, entry.Name, d.instructionDecoder.Value())
}

func (d *ProgressiveDecoder) doLiteralHeaderFieldPostBaseInstruction() bool {
	absoluteIndex, ok := PostBaseIndexToAbsoluteIndex(
		d.instructionDecoder.Varint(), d.base,
	)
	if !ok {
		d.onError(QPACK_DECOMPRESSION_FAILED, "Invalid post-base index.")
		return false
	}

	if absoluteIndex >= d.requiredInsertCount {
		d.onError(QPACK_DECOMPRESSION_FAILED, "Absolute Index must be smaller than Required Insert Count.")
		return false
	}

	if absoluteIndex+1 > d.requiredInsertCountSoFar {
		d.requiredInsertCountSoFar = absoluteIndex + 1
	}

	entry := d.headerTable.LookupEntry(false, absoluteIndex)
	if entry == nil {
		d.onError(QPACK_DECOMPRESSION_FAILED, "Dynamic table entry already evicted.")
		return false
	}

	d.headerTable.SetDynamicTableEntryReferenced()

	return d.onHeaderDecoded(false, entry.Name, d.instructionDecoder.Value())
}

func (d *ProgressiveDecoder) doLiteralHeaderFieldInstruction() bool {
	return d.onHeaderDecoded(false, d.instructionDecoder.Name(), d.instructionDecoder.Value())
}

func (d *ProgressiveDecoder) onHeaderDecoded(valueFromStaticTable bool, name, value string) bool {
	d.handler.OnHeaderDecoded(name, value)
	return true
}

func (d *ProgressiveDecoder) finishDecoding() {
	if d.errorDetected {
		return
	}

	if !d.instructionDecoder.AtInstructionBoundary() {
		d.onError(QPACK_DECOMPRESSION_FAILED, "Incomplete header block.")
		return
	}

	if !d.prefixDecoded {
		d.onError(QPACK_DECOMPRESSION_FAILED, "Incomplete header data prefix.")
		return
	}

	if d.requiredInsertCount != d.requiredInsertCountSoFar {
		d.onError(QPACK_DECOMPRESSION_FAILED, "Required Insert Count too large.")
		return
	}

	d.visitor.OnDecodingCompleted(d.streamID, d.requiredInsertCount)
	d.handler.OnDecodingCompleted()
}

func (d *ProgressiveDecoder) onError(errorCode uint64, errorMessage string) {
	if d.errorDetected {
		return
	}

	d.errorDetected = true
	d.lastErr = NewError(ErrorCode(errorCode), errorMessage)
	// Might destroy d synchronously in handler.
	d.handler.OnDecodingErrorDetected(errorCode, errorMessage)
}

func (d *ProgressiveDecoder) deltaBaseToBase(sign bool, deltaBase uint64) (uint64, bool) {
	if sign {
		if deltaBase == math.MaxUint64 || d.requiredInsertCount < deltaBase+1 {
			return 0, false
		}

		return d.requiredInsertCount - deltaBase - 1, true
	}

	if deltaBase > math.MaxUint64-d.requiredInsertCount {
		return 0, false
	}

	return d.requiredInsertCount + deltaBase, true
}

// Interface compliance assertions
var (
	_ InstructionDecoderDelegate = (*ProgressiveDecoder)(nil)
	_ DecoderHeaderTableObserver = (*ProgressiveDecoder)(nil)
	_ io.Writer                  = (*ProgressiveDecoder)(nil)
	_ io.Closer                  = (*ProgressiveDecoder)(nil)
)
