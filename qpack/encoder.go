// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package qpack

import (
	"fmt"
	"io"
	"iter"
	"strings"

	"github.com/lemon4ksan/foundation/generic"
)

// DrainingFraction defines the fraction of dynamic table entries reserved for draining (RFC 9204).
// The oldest entries will not be referenced in header blocks.
// A new entry (duplicate or literal with name reference) will be added to the
// dynamic table instead to allow draining entries to be evicted faster.
const DrainingFraction float64 = 0.25

// Deprecated: use DrainingFraction.
const kDrainingFraction = DrainingFraction

// DecoderStreamErrorHandler receives notifications of errors on the decoder stream.
// This MUST be treated as a connection error of type HTTP_QPACK_DECODER_STREAM_ERROR.
type DecoderStreamErrorHandler func(errorCode uint64, errorMessage string)

// CookieCrumbling specifies whether cookie crumbling should be used when sending QPACK headers.
type CookieCrumbling int

const (
	CookieCrumblingEnabled CookieCrumbling = iota
	CookieCrumblingDisabled
)

const (
	// Deprecated: use CookieCrumblingEnabled.
	KCookieCrumblingEnabled = CookieCrumblingEnabled
	// Deprecated: use CookieCrumblingDisabled.
	KCookieCrumblingDisabled = CookieCrumblingDisabled
)

// Encoder manages dynamic table state, generates encoder stream instructions,
// processes decoder stream feedback, and serializes HTTP/3 header blocks using two-pass encoding (RFC 9204).
// Exactly one instance should exist per QUIC connection.
type Encoder struct {
	huffmanEncoding           HuffmanEncoding
	cookieCrumbling           CookieCrumbling
	decoderStreamErrorHandler DecoderStreamErrorHandler
	decoderStreamReceiver     *DecoderStreamReceiver
	encoderStreamSender       *EncoderStreamSender
	headerTable               *EncoderHeaderTable
	maximumBlockedStreams     uint64
	blockingManager           *BlockingManager
	headerListCount           int
	lastErr                   error
}

// NewEncoder constructs a new Encoder.
func NewEncoder(
	decoderStreamErrorHandler DecoderStreamErrorHandler,
	huffmanEncoding HuffmanEncoding,
	cookieCrumbling CookieCrumbling,
) *Encoder {
	if decoderStreamErrorHandler == nil {
		decoderStreamErrorHandler = func(errorCode uint64, errorMessage string) {}
	}

	encoder := &Encoder{
		huffmanEncoding:           huffmanEncoding,
		cookieCrumbling:           cookieCrumbling,
		decoderStreamErrorHandler: decoderStreamErrorHandler,
		encoderStreamSender:       NewEncoderStreamSenderWithHuffman(huffmanEncoding, nil),
		headerTable:               NewEncoderHeaderTable(),
		maximumBlockedStreams:     0,
		blockingManager:           NewBlockingManager(),
		headerListCount:           0,
	}
	encoder.decoderStreamReceiver = NewDecoderStreamReceiver(encoder)
	return encoder
}

// NewEncoderWithDefaults constructs a new Encoder with default Huffman and cookie crumbling settings.
func NewEncoderWithDefaults(decoderStreamErrorHandler DecoderStreamErrorHandler) *Encoder {
	return NewEncoder(decoderStreamErrorHandler, HuffmanEncodingEnabled, CookieCrumblingEnabled)
}

// -----------------------------------------------------------------------------
// Header Encoding
// -----------------------------------------------------------------------------

// EncodeHeaderList encodes a header list for streamID.
// If encoderStreamSentByteCount is non-nil, it is populated with the number of bytes
// sent on the encoder stream to insert dynamic table entries.
func (e *Encoder) EncodeHeaderList(
	streamID uint64,
	headerList []HeaderField,
	encoderStreamSentByteCount *uint64,
) []byte {
	referredIndices := NewIndexSet()
	representations := e.firstPassEncode(streamID, headerList, referredIndices, encoderStreamSentByteCount)

	var requiredInsertCount uint64
	if !referredIndices.Empty() {
		requiredInsertCount = referredIndices.RequiredInsertCount()
		e.blockingManager.OnHeaderBlockSent(streamID, referredIndices.Indices())
	}

	return e.secondPassEncode(representations, requiredInsertCount)
}

// EncodeHeaderSeq encodes headers supplied as an iter.Seq[HeaderField].
func (e *Encoder) EncodeHeaderSeq(
	streamID uint64,
	headers iter.Seq[HeaderField],
	encoderStreamSentByteCount *uint64,
) []byte {
	var list []HeaderField
	for hf := range headers {
		list = append(list, hf)
	}
	return e.EncodeHeaderList(streamID, list, encoderStreamSentByteCount)
}

// EncodeHeaderSeq2 encodes headers supplied as an iter.Seq2[string, string].
func (e *Encoder) EncodeHeaderSeq2(
	streamID uint64,
	headers iter.Seq2[string, string],
	encoderStreamSentByteCount *uint64,
) []byte {
	var list []HeaderField
	for name, value := range headers {
		list = append(list, HeaderField{Name: name, Value: value})
	}
	return e.EncodeHeaderList(streamID, list, encoderStreamSentByteCount)
}

// EncodeHeaderListTo encodes headerList directly into an io.Writer.
// Returns the number of bytes written and any write error.
func (e *Encoder) EncodeHeaderListTo(
	w io.Writer,
	streamID uint64,
	headerList []HeaderField,
	encoderStreamSentByteCount *uint64,
) (int, error) {
	data := e.EncodeHeaderList(streamID, headerList, encoderStreamSentByteCount)
	return w.Write(data)
}

// SetMaximumDynamicTableCapacity sets the maximum dynamic table capacity in bytes.
// Called when SETTINGS_QPACK_MAX_TABLE_CAPACITY is received.
// Returns true if maximumDynamicTableCapacity is set for the first time or if it doesn't change current value.
func (e *Encoder) SetMaximumDynamicTableCapacity(maximumDynamicTableCapacity uint64) bool {
	return e.headerTable.SetMaximumDynamicTableCapacity(maximumDynamicTableCapacity)
}

// SetDynamicTableCapacity sets the dynamic table capacity.
// dynamicTableCapacity must not exceed maximum dynamic table capacity.
// Also emits a Set Dynamic Table Capacity instruction on the encoder stream.
func (e *Encoder) SetDynamicTableCapacity(dynamicTableCapacity uint64) {
	e.encoderStreamSender.SendSetDynamicTableCapacity(dynamicTableCapacity)
	// Do not flush encoder stream. This write can safely be delayed until more instructions are written.

	success := e.headerTable.SetDynamicTableCapacity(dynamicTableCapacity)
	if !success {
		panic("qpack: dynamic table capacity exceeds maximum dynamic table capacity")
	}
}

// SetMaximumBlockedStreams sets the maximum number of blocked streams.
// Called when SETTINGS_QPACK_BLOCKED_STREAMS is received.
// Returns true if maximumBlockedStreams does not decrease current value.
func (e *Encoder) SetMaximumBlockedStreams(maximumBlockedStreams uint64) bool {
	if maximumBlockedStreams < e.maximumBlockedStreams {
		return false
	}
	e.maximumBlockedStreams = maximumBlockedStreams
	return true
}

// SetStreamSenderDelegate sets the delegate for the encoder stream sender.
// delegate must be set if dynamic table capacity is not zero.
func (e *Encoder) SetStreamSenderDelegate(delegate StreamSenderDelegate) {
	e.encoderStreamSender.SetDelegate(delegate)
}

// DecoderStreamReceiver returns the control stream receiver for peer decoder stream data.
func (e *Encoder) DecoderStreamReceiver() *DecoderStreamReceiver {
	return e.decoderStreamReceiver
}

// EncoderStreamSender returns the encoder stream sender.
func (e *Encoder) EncoderStreamSender() *EncoderStreamSender {
	return e.encoderStreamSender
}

// HeaderTable returns the encoder's dynamic header table.
func (e *Encoder) HeaderTable() *EncoderHeaderTable {
	return e.headerTable
}

// BlockingManager returns the encoder's blocking manager.
func (e *Encoder) BlockingManager() *BlockingManager {
	return e.blockingManager
}

// DynamicTableEntryReferenced returns true if any dynamic table entries have been referenced.
func (e *Encoder) DynamicTableEntryReferenced() bool {
	return e.headerTable.DynamicTableEntryReferenced()
}

// MaximumBlockedStreams returns the maximum allowed concurrent blocked streams limit.
func (e *Encoder) MaximumBlockedStreams() uint64 {
	return e.maximumBlockedStreams
}

// MaximumDynamicTableCapacity returns the maximum dynamic table capacity in bytes.
func (e *Encoder) MaximumDynamicTableCapacity() uint64 {
	return e.headerTable.MaximumDynamicTableCapacity()
}

// -----------------------------------------------------------------------------
// DecoderStreamReceiverDelegate implementation & Chromium Feedback Handlers
// -----------------------------------------------------------------------------

// SectionAck implements DecoderStreamReceiverDelegate.SectionAck.
func (e *Encoder) SectionAck(streamID uint64) {
	e.OnHeaderAcknowledgement(streamID)
}

// StreamCancellation implements DecoderStreamReceiverDelegate.StreamCancellation.
func (e *Encoder) StreamCancellation(streamID uint64) {
	e.OnStreamCancellation(streamID)
}

// InsertCountIncrement implements DecoderStreamReceiverDelegate.InsertCountIncrement.
func (e *Encoder) InsertCountIncrement(increment uint64) {
	e.OnInsertCountIncrement(increment)
}

// Error implements DecoderStreamReceiverDelegate.Error.
func (e *Encoder) Error(qpackError uint64, errorMessage string) {
	e.OnErrorDetected(qpackError, errorMessage)
}

// OnInsertCountIncrement handles an Insert Count Increment instruction from the decoder stream.
func (e *Encoder) OnInsertCountIncrement(increment uint64) {
	if increment == 0 {
		e.OnErrorDetected(QUIC_QPACK_DECODER_STREAM_INVALID_ZERO_INCREMENT, "Invalid increment value 0.")
		return
	}

	if !e.blockingManager.OnInsertCountIncrement(increment) {
		e.OnErrorDetected(
			QUIC_QPACK_DECODER_STREAM_INCREMENT_OVERFLOW,
			"Insert Count Increment instruction causes overflow.",
		)
	}

	if e.blockingManager.KnownReceivedCount() > e.headerTable.InsertedEntryCount() {
		e.OnErrorDetected(
			QUIC_QPACK_DECODER_STREAM_IMPOSSIBLE_INSERT_COUNT,
			fmt.Sprintf("Increment value %d raises known received count to %d exceeding inserted entry count %d",
				increment, e.blockingManager.KnownReceivedCount(), e.headerTable.InsertedEntryCount()),
		)
	}
}

// OnHeaderAcknowledgement handles a Header Acknowledgement instruction from the decoder stream.
func (e *Encoder) OnHeaderAcknowledgement(streamID uint64) {
	if !e.blockingManager.OnHeaderAcknowledgement(streamID) {
		e.OnErrorDetected(
			QUIC_QPACK_DECODER_STREAM_INCORRECT_ACKNOWLEDGEMENT,
			fmt.Sprintf("Header Acknowledgement received for stream %d with no outstanding header blocks.", streamID),
		)
	}
}

// OnStreamCancellation handles a Stream Cancellation instruction from the decoder stream.
func (e *Encoder) OnStreamCancellation(streamID uint64) {
	e.blockingManager.OnStreamCancellation(streamID)
}

// OnErrorDetected notifies the decoder stream error delegate.
func (e *Encoder) OnErrorDetected(errorCode uint64, errorMessage string) {
	// Map generic QPACK decoder stream error to integer too large if appropriate
	if errorCode == QPACK_DECODER_STREAM_ERROR && errorMessage == "Encoded integer too large." {
		errorCode = QUIC_QPACK_DECODER_STREAM_INTEGER_TOO_LARGE
	}

	e.lastErr = NewError(ErrorCode(errorCode), errorMessage)

	if e.decoderStreamErrorHandler != nil {
		e.decoderStreamErrorHandler(errorCode, errorMessage)
	}
}

// LastError returns the most recent error detected on the decoder stream, if any.
func (e *Encoder) LastError() error {
	return e.lastErr
}

// -----------------------------------------------------------------------------
// Two-Pass Header Block Encoding
// -----------------------------------------------------------------------------

// splitHeaderField splits a single HeaderField value along ';' for cookies (if cookie
// crumbling is enabled) or along '\0' for non-cookie headers.
func splitHeaderField(name, value string, crumbling CookieCrumbling) []HeaderField {
	if name == "cookie" {
		if crumbling == CookieCrumblingEnabled {
			var result []HeaderField
			start := 0
			for start < len(value) {
				idx := strings.IndexByte(value[start:], ';')
				if idx == -1 {
					result = append(result, HeaderField{Name: name, Value: value[start:]})
					break
				}
				end := start + idx
				result = append(result, HeaderField{Name: name, Value: value[start:end]})
				start = end + 1
				if start < len(value) && value[start] == ' ' {
					start++
				}
			}
			if len(result) == 0 {
				result = append(result, HeaderField{Name: name, Value: ""})
			}
			return result
		}
		return []HeaderField{{Name: name, Value: value}}
	}

	// Non-cookie headers: split along '\0'
	if !strings.ContainsRune(value, '\x00') {
		return []HeaderField{{Name: name, Value: value}}
	}
	parts := strings.Split(value, "\x00")
	result := make([]HeaderField, len(parts))
	for i, p := range parts {
		result[i] = HeaderField{Name: name, Value: p}
	}
	return result
}

// firstPassEncode performs the first pass of two-pass encoding: represents each header field
// as an indexed reference, a literal with name reference, or a literal name and value.
// Emits necessary instructions on the encoder stream coalesced in a single write.
func (e *Encoder) firstPassEncode(
	streamID uint64,
	headerList []HeaderField,
	referredIndices *IndexSet,
	encoderStreamSentByteCount *uint64,
) []*InstructionWithValues {
	initialBuffered := e.encoderStreamSender.BufferedByteCount()
	canWrite := e.encoderStreamSender.CanWrite()

	var representations []*InstructionWithValues

	knownReceivedCount := e.blockingManager.KnownReceivedCount()
	smallestNonEvictableIndex := e.blockingManager.SmallestBlockingIndex()
	if knownReceivedCount < smallestNonEvictableIndex {
		smallestNonEvictableIndex = knownReceivedCount
	}

	drainingIndex := e.headerTable.DrainingIndex(kDrainingFraction)
	blockingAllowed := e.blockingManager.BlockingAllowedOnStream(streamID, e.maximumBlockedStreams)

	// Update eviction barrier on header table
	e.headerTable.SetSmallestAllowedIndex(smallestNonEvictableIndex)

	// Flatten header list with value splitting
	flattened := generic.FlatMap(headerList, func(hf HeaderField) []HeaderField {
		return splitHeaderField(hf.Name, hf.Value, e.cookieCrumbling)
	})

	for _, hf := range flattened {
		name := hf.Name
		value := hf.Value

		matchResult := e.headerTable.FindHeaderField(name, value)

		switch matchResult.Match {
		case MatchTypeNameAndValue:
			if matchResult.IsStatic {
				representations = append(
					representations,
					EncodeIndexedHeaderField(true, matchResult.Index, referredIndices),
				)
				continue
			}

			if matchResult.Index >= drainingIndex {
				if !blockingAllowed && matchResult.Index >= knownReceivedCount {
					// Blocked stream limit exhausted; cannot reference unacknowledged entry.
				} else {
					representations = append(
						representations,
						EncodeIndexedHeaderField(false, matchResult.Index, referredIndices),
					)
					if matchResult.Index < smallestNonEvictableIndex {
						smallestNonEvictableIndex = matchResult.Index
						e.headerTable.SetSmallestAllowedIndex(smallestNonEvictableIndex)
					}
					e.headerTable.SetDynamicTableEntryReferenced()
					continue
				}
			} else {
				// Entry is draining. Try duplicate instead if possible.
				entrySize := EntrySize(name, value)
				limitIndex := smallestNonEvictableIndex
				if matchResult.Index < limitIndex {
					limitIndex = matchResult.Index
				}
				if blockingAllowed &&
					entrySize <= e.headerTable.MaxInsertSizeWithoutEvictingGivenEntry(limitIndex) &&
					canWrite {
					relIndex := AbsoluteIndexToEncoderRelativeIndex(
						matchResult.Index,
						e.headerTable.InsertedEntryCount(),
					)
					e.encoderStreamSender.SendDuplicate(relIndex)
					newIndex := e.headerTable.InsertEntry(name, value)
					representations = append(
						representations,
						EncodeIndexedHeaderField(false, newIndex, referredIndices),
					)
					if matchResult.Index < smallestNonEvictableIndex {
						smallestNonEvictableIndex = matchResult.Index
						e.headerTable.SetSmallestAllowedIndex(smallestNonEvictableIndex)
					}
					e.headerTable.SetDynamicTableEntryReferenced()
					continue
				}
			}

			// Match cannot be used. Fall back to name-only match.
			matchResultNameOnly := e.headerTable.FindHeaderName(name)
			if matchResultNameOnly.Match != MatchTypeName ||
				(matchResultNameOnly.IsStatic == matchResult.IsStatic && matchResultNameOnly.Index == matchResult.Index) {
				representations = append(representations, EncodeLiteralHeaderField(name, value))
				continue
			}
			matchResult = matchResultNameOnly
			fallthrough

		case MatchTypeName:
			if matchResult.IsStatic {
				entrySize := EntrySize(name, value)
				if blockingAllowed &&
					entrySize <= e.headerTable.MaxInsertSizeWithoutEvictingGivenEntry(smallestNonEvictableIndex) &&
					canWrite {
					e.encoderStreamSender.SendInsertWithNameReference(true, matchResult.Index, value)
					newIndex := e.headerTable.InsertEntry(name, value)
					representations = append(
						representations,
						EncodeIndexedHeaderField(false, newIndex, referredIndices),
					)
					if newIndex < smallestNonEvictableIndex {
						smallestNonEvictableIndex = newIndex
						e.headerTable.SetSmallestAllowedIndex(smallestNonEvictableIndex)
					}
					continue
				}
				representations = append(
					representations,
					EncodeLiteralHeaderFieldWithNameReference(true, matchResult.Index, value, referredIndices),
				)
				continue
			}

			// Dynamic name match
			entrySize := EntrySize(name, value)
			limitIndex := smallestNonEvictableIndex
			if matchResult.Index < limitIndex {
				limitIndex = matchResult.Index
			}
			if blockingAllowed &&
				entrySize <= e.headerTable.MaxInsertSizeWithoutEvictingGivenEntry(limitIndex) &&
				canWrite {
				relIndex := AbsoluteIndexToEncoderRelativeIndex(matchResult.Index, e.headerTable.InsertedEntryCount())
				e.encoderStreamSender.SendInsertWithNameReference(false, relIndex, value)
				newIndex := e.headerTable.InsertEntry(name, value)
				representations = append(representations, EncodeIndexedHeaderField(false, newIndex, referredIndices))
				if matchResult.Index < smallestNonEvictableIndex {
					smallestNonEvictableIndex = matchResult.Index
					e.headerTable.SetSmallestAllowedIndex(smallestNonEvictableIndex)
				}
				e.headerTable.SetDynamicTableEntryReferenced()
				continue
			}

			if (blockingAllowed || matchResult.Index < knownReceivedCount) && matchResult.Index >= drainingIndex {
				representations = append(
					representations,
					EncodeLiteralHeaderFieldWithNameReference(false, matchResult.Index, value, referredIndices),
				)
				if matchResult.Index < smallestNonEvictableIndex {
					smallestNonEvictableIndex = matchResult.Index
					e.headerTable.SetSmallestAllowedIndex(smallestNonEvictableIndex)
				}
				e.headerTable.SetDynamicTableEntryReferenced()
				continue
			}

			representations = append(representations, EncodeLiteralHeaderField(name, value))
			continue

		case MatchTypeNoMatch:
			entrySize := EntrySize(name, value)
			if blockingAllowed &&
				entrySize <= e.headerTable.MaxInsertSizeWithoutEvictingGivenEntry(smallestNonEvictableIndex) &&
				canWrite {
				e.encoderStreamSender.SendInsertWithoutNameReference(name, value)
				newIndex := e.headerTable.InsertEntry(name, value)
				representations = append(representations, EncodeIndexedHeaderField(false, newIndex, referredIndices))
				if newIndex < smallestNonEvictableIndex {
					smallestNonEvictableIndex = newIndex
					e.headerTable.SetSmallestAllowedIndex(smallestNonEvictableIndex)
				}
				continue
			}

			representations = append(representations, EncodeLiteralHeaderField(name, value))
			continue
		}
	}

	currentBuffered := e.encoderStreamSender.BufferedByteCount()
	if encoderStreamSentByteCount != nil {
		*encoderStreamSentByteCount = currentBuffered - initialBuffered
	}
	if canWrite {
		e.encoderStreamSender.Flush()
	}

	e.headerListCount++
	return representations
}

// secondPassEncode performs the second pass of two-pass encoding: serializes representations
// generated in the first pass, transforming absolute dynamic indices to relative indices.
func (e *Encoder) secondPassEncode(
	representations []*InstructionWithValues,
	requiredInsertCount uint64,
) []byte {
	instructionEncoder := NewInstructionEncoder(e.huffmanEncoding)
	var encodedHeaders []byte

	// Header block prefix
	encodedRIC := EncodeRequiredInsertCount(requiredInsertCount, e.headerTable.MaxEntries())
	prefix := InstructionWithValuesPrefix(encodedRIC)
	encodedHeaders = instructionEncoder.Encode(prefix, encodedHeaders)

	base := requiredInsertCount

	for _, representation := range representations {
		// Dynamic table references must be transformed from absolute to relative indices
		if (representation.Instruction() == IndexedHeaderFieldInstruction() ||
			representation.Instruction() == LiteralHeaderFieldNameReferenceInstruction()) &&
			!representation.SBit() {
			relIndex := AbsoluteIndexToRequestStreamRelativeIndex(representation.Varint(), base)
			representation.SetVarint(relIndex)
		}
		encodedHeaders = instructionEncoder.Encode(representation, encodedHeaders)
	}

	return encodedHeaders
}

// EncodeIndexedHeaderField generates an indexed header field representation and tracks dynamic indices.
func EncodeIndexedHeaderField(
	isStatic bool,
	index uint64,
	referredIndices *IndexSet,
) *InstructionWithValues {
	if !isStatic {
		referredIndices.Insert(index)
	}
	return InstructionWithValuesIndexedHeaderField(isStatic, index)
}

// EncodeLiteralHeaderFieldWithNameReference generates a literal header field with name reference.
func EncodeLiteralHeaderFieldWithNameReference(
	isStatic bool,
	index uint64,
	value string,
	referredIndices *IndexSet,
) *InstructionWithValues {
	if !isStatic {
		referredIndices.Insert(index)
	}
	return InstructionWithValuesLiteralHeaderFieldNameReference(isStatic, index, value)
}

// EncodeLiteralHeaderField generates a literal header field with literal name and value.
func EncodeLiteralHeaderField(name, value string) *InstructionWithValues {
	return InstructionWithValuesLiteralHeaderField(name, value)
}

// -----------------------------------------------------------------------------
// EncoderPeer for Unit Test Parity
// -----------------------------------------------------------------------------

// EncoderPeer provides access to internal encoder state for unit tests.
//
// Deprecated: use Encoder.HeaderTable(), Encoder.MaximumBlockedStreams(), or Encoder.BlockingManager().
type EncoderPeer struct{}

// EncoderPeerHeaderTable returns encoder's header table.
//
// Deprecated: use encoder.HeaderTable().
func EncoderPeerHeaderTable(encoder *Encoder) *EncoderHeaderTable {
	return encoder.headerTable
}

// EncoderPeerMaximumBlockedStreams returns encoder's maximum blocked streams limit.
//
// Deprecated: use encoder.MaximumBlockedStreams().
func EncoderPeerMaximumBlockedStreams(encoder *Encoder) uint64 {
	return encoder.maximumBlockedStreams
}

// EncoderPeerSmallestBlockingIndex returns encoder's smallest blocking index.
//
// Deprecated: use encoder.BlockingManager().SmallestBlockingIndex().
func EncoderPeerSmallestBlockingIndex(encoder *Encoder) uint64 {
	return encoder.blockingManager.SmallestBlockingIndex()
}

// Interface compliance assertion
var _ DecoderStreamReceiverDelegate = (*Encoder)(nil)
