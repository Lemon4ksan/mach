// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package qpack

import "iter"

// EncoderStreamErrorHandler receives notifications of fatal errors on the encoder stream.
type EncoderStreamErrorHandler func(errorCode uint64, errorMessage string)

// Decoder decodes QPACK header blocks and manages dynamic table state (RFC 9204).
// Exactly one instance should exist per QUIC connection.
type Decoder struct {
	encoderStreamErrorHandler EncoderStreamErrorHandler
	encoderStreamReceiver     *EncoderStreamReceiver
	decoderStreamSender       *DecoderStreamSender
	headerTable               *DecoderHeaderTable
	blockedStreams            map[uint64]struct{}
	maximumBlockedStreams     uint64
	knownReceivedCount        uint64
	lastErr                   error
}

// NewDecoder creates a new Decoder.
func NewDecoder(
	maximumDynamicTableCapacity uint64,
	maximumBlockedStreams uint64,
	encoderStreamErrorHandler EncoderStreamErrorHandler,
) *Decoder {
	if encoderStreamErrorHandler == nil {
		encoderStreamErrorHandler = func(errorCode uint64, errorMessage string) {}
	}

	d := &Decoder{
		encoderStreamErrorHandler: encoderStreamErrorHandler,
		decoderStreamSender:       NewDecoderStreamSender(nil),
		headerTable:               NewDecoderHeaderTable(),
		blockedStreams:            make(map[uint64]struct{}),
		maximumBlockedStreams:     maximumBlockedStreams,
		knownReceivedCount:        0,
	}

	d.headerTable.SetMaximumDynamicTableCapacity(maximumDynamicTableCapacity)
	d.encoderStreamReceiver = NewEncoderStreamReceiver(d)

	return d
}

// OnStreamReset signals to the peer's encoder that a stream is reset.
// This lets the peer's encoder know that no more header blocks will be processed on this stream,
// allowing references to dynamic table entries to be safely evicted.
// RFC 9204 §2.2.2.2 & Chromium Decoder::OnStreamReset.
func (d *Decoder) OnStreamReset(streamID uint64) {
	delete(d.blockedStreams, streamID)

	if d.headerTable.MaximumDynamicTableCapacity() > 0 {
		d.decoderStreamSender.SendStreamCancellation(streamID)
	}
}

// OnStreamBlocked is called by ProgressiveDecoder when decoding is blocked.
// Returns true if allowed, or false if the blocked stream limit is exceeded.
func (d *Decoder) OnStreamBlocked(streamID uint64) bool {
	d.blockedStreams[streamID] = struct{}{}
	return uint64(len(d.blockedStreams)) <= d.maximumBlockedStreams
}

// OnStreamUnblocked is called by ProgressiveDecoder when the stream becomes unblocked.
func (d *Decoder) OnStreamUnblocked(streamID uint64) {
	delete(d.blockedStreams, streamID)
}

// OnDecodingCompleted implements DecodingCompletedVisitor.
func (d *Decoder) OnDecodingCompleted(streamID, requiredInsertCount uint64) {
	if requiredInsertCount > 0 {
		d.decoderStreamSender.SendHeaderAcknowledgement(streamID)

		if d.knownReceivedCount < requiredInsertCount {
			d.knownReceivedCount = requiredInsertCount
		}
	}

	// Send an Insert Count Increment instruction if not all dynamic table entries
	// have been acknowledged yet. RFC 9204 §2.1.4.
	if d.knownReceivedCount < d.headerTable.InsertedEntryCount() {
		increment := d.headerTable.InsertedEntryCount() - d.knownReceivedCount
		d.decoderStreamSender.SendInsertCountIncrement(increment)
		d.knownReceivedCount = d.headerTable.InsertedEntryCount()
	}
}

// CreateProgressiveDecoder creates a ProgressiveDecoder for decoding a header block on streamID.
func (d *Decoder) CreateProgressiveDecoder(
	streamID uint64,
	handler HeadersHandlerInterface,
) *ProgressiveDecoder {
	return d.CreateProgressiveDecoderWithMaxBufferedData(streamID, 0, handler)
}

// CreateProgressiveDecoderWithMaxBufferedData creates a ProgressiveDecoder with an explicit buffer limit.
func (d *Decoder) CreateProgressiveDecoderWithMaxBufferedData(
	streamID uint64,
	maxBufferedData uint64,
	handler HeadersHandlerInterface,
) *ProgressiveDecoder {
	return NewProgressiveDecoder(
		streamID,
		maxBufferedData,
		d,
		d,
		d.headerTable,
		handler,
	)
}

type collectedHeadersHandler struct {
	headers []HeaderField
	err     error
}

func (h *collectedHeadersHandler) OnHeaderDecoded(name, value string) {
	h.headers = append(h.headers, HeaderField{Name: name, Value: value})
}

func (h *collectedHeadersHandler) OnDecodingCompleted() {
	_ = h
}

func (h *collectedHeadersHandler) OnDecodingErrorDetected(errorCode uint64, errorMessage string) {
	h.err = NewError(ErrorCode(errorCode), errorMessage)
}

// DecodeHeaderBlock decodes a complete header block and returns the decoded header fields.
// If decoding fails, it returns an error unwrappable via errors.Is.
func (d *Decoder) DecodeHeaderBlock(streamID uint64, block []byte) ([]HeaderField, error) {
	handler := &collectedHeadersHandler{headers: make([]HeaderField, 0, 16)}
	progDec := d.CreateProgressiveDecoder(streamID, handler)
	progDec.Decode(block)
	progDec.EndHeaderBlock()

	if handler.err != nil {
		return nil, handler.err
	}

	return handler.headers, nil
}

// DecodeHeaderBlockSeq decodes a complete header block and returns an iterator over (name, value) pairs.
// If decoding fails, it returns an error.
func (d *Decoder) DecodeHeaderBlockSeq(streamID uint64, block []byte) (iter.Seq2[string, string], error) {
	fields, err := d.DecodeHeaderBlock(streamID, block)
	if err != nil {
		return nil, err
	}

	return HeaderFields(fields).All(), nil
}

// InsertWithNameReference handles RFC 9204 §4.3.1 on encoder stream.
func (d *Decoder) InsertWithNameReference(isStatic bool, nameIndex uint64, value string) {
	if isStatic {
		entry := d.headerTable.LookupEntry(true, nameIndex)
		if entry == nil {
			d.OnErrorDetected(QUIC_QPACK_ENCODER_STREAM_INVALID_STATIC_ENTRY, "Invalid static table entry.")
			return
		}

		if !d.headerTable.EntryFitsDynamicTableCapacity(entry.Name, value) {
			d.OnErrorDetected(
				QUIC_QPACK_ENCODER_STREAM_ERROR_INSERTING_STATIC,
				"Error inserting entry with name reference.",
			)

			return
		}

		d.headerTable.InsertEntry(entry.Name, value)

		return
	}

	absoluteIndex, ok := EncoderStreamRelativeIndexToAbsoluteIndex(
		nameIndex, d.headerTable.InsertedEntryCount(),
	)
	if !ok {
		d.OnErrorDetected(QUIC_QPACK_ENCODER_STREAM_INSERTION_INVALID_RELATIVE_INDEX, "Invalid relative index.")
		return
	}

	entry := d.headerTable.LookupEntry(false, absoluteIndex)
	if entry == nil {
		d.OnErrorDetected(QUIC_QPACK_ENCODER_STREAM_INSERTION_DYNAMIC_ENTRY_NOT_FOUND, "Dynamic table entry not found.")
		return
	}

	if !d.headerTable.EntryFitsDynamicTableCapacity(entry.Name, value) {
		d.OnErrorDetected(
			QUIC_QPACK_ENCODER_STREAM_ERROR_INSERTING_DYNAMIC,
			"Error inserting entry with name reference.",
		)

		return
	}

	d.headerTable.InsertEntry(entry.Name, value)
}

// InsertWithoutNameReference handles RFC 9204 §4.3.2 on encoder stream.
func (d *Decoder) InsertWithoutNameReference(name, value string) {
	if !d.headerTable.EntryFitsDynamicTableCapacity(name, value) {
		d.OnErrorDetected(QUIC_QPACK_ENCODER_STREAM_ERROR_INSERTING_LITERAL, "Error inserting literal entry.")
		return
	}

	d.headerTable.InsertEntry(name, value)
}

// Duplicate handles RFC 9204 §4.3.3 on encoder stream.
func (d *Decoder) Duplicate(index uint64) {
	absoluteIndex, ok := EncoderStreamRelativeIndexToAbsoluteIndex(
		index, d.headerTable.InsertedEntryCount(),
	)
	if !ok {
		d.OnErrorDetected(QUIC_QPACK_ENCODER_STREAM_DUPLICATE_INVALID_RELATIVE_INDEX, "Invalid relative index.")
		return
	}

	entry := d.headerTable.LookupEntry(false, absoluteIndex)
	if entry == nil {
		d.OnErrorDetected(QUIC_QPACK_ENCODER_STREAM_DUPLICATE_DYNAMIC_ENTRY_NOT_FOUND, "Dynamic table entry not found.")
		return
	}

	if !d.headerTable.EntryFitsDynamicTableCapacity(entry.Name, entry.Value) {
		d.OnErrorDetected(QUIC_INTERNAL_ERROR, "Error inserting duplicate entry.")
		return
	}

	d.headerTable.InsertEntry(entry.Name, entry.Value)
}

// SetDynamicTableCapacity handles RFC 9204 §4.3.4 on encoder stream.
func (d *Decoder) SetDynamicTableCapacity(capacity uint64) {
	if !d.headerTable.SetDynamicTableCapacity(capacity) {
		d.OnErrorDetected(
			QUIC_QPACK_ENCODER_STREAM_SET_DYNAMIC_TABLE_CAPACITY,
			"Error updating dynamic table capacity.",
		)
	}
}

// Error receives decoder or wire parsing errors from EncoderStreamReceiver.
func (d *Decoder) Error(qpackError uint64, errorMessage string) {
	errorCode := qpackError
	if qpackError == QPACK_ENCODER_STREAM_ERROR {
		switch errorMessage {
		case "Encoded integer too large.":
			errorCode = QUIC_QPACK_ENCODER_STREAM_INTEGER_TOO_LARGE
		case "String literal too long.":
			errorCode = QUIC_QPACK_ENCODER_STREAM_STRING_LITERAL_TOO_LONG
		case "Error in Huffman-encoded string.":
			errorCode = QUIC_QPACK_ENCODER_STREAM_HUFFMAN_ENCODING_ERROR
		}
	}

	d.OnErrorDetected(errorCode, errorMessage)
}

// OnErrorDetected notifies the encoder stream error delegate.
func (d *Decoder) OnErrorDetected(errorCode uint64, errorMessage string) {
	d.lastErr = NewError(ErrorCode(errorCode), errorMessage)
	d.encoderStreamErrorHandler(errorCode, errorMessage)
}

// LastError returns the most recent error detected on the decoder or encoder stream, if any.
func (d *Decoder) LastError() error {
	return d.lastErr
}

// SetStreamSenderDelegate sets the stream sender delegate for transmitting feedback to peer.
func (d *Decoder) SetStreamSenderDelegate(delegate StreamSenderDelegate) {
	d.decoderStreamSender.SetDelegate(delegate)
}

// EncoderStreamReceiver returns the stream receiver for peer encoder stream data.
func (d *Decoder) EncoderStreamReceiver() *EncoderStreamReceiver {
	return d.encoderStreamReceiver
}

// DecoderStreamSender returns the stream sender for decoder stream feedback.
func (d *Decoder) DecoderStreamSender() *DecoderStreamSender {
	return d.decoderStreamSender
}

// HeaderTable returns the decoder's dynamic and static header table.
func (d *Decoder) HeaderTable() *DecoderHeaderTable {
	return d.headerTable
}

// DynamicTableEntryReferenced returns true if any dynamic table entries have been referenced.
func (d *Decoder) DynamicTableEntryReferenced() bool {
	return d.headerTable.DynamicTableEntryReferenced()
}

// FlushDecoderStream flushes buffered instructions on the decoder stream.
func (d *Decoder) FlushDecoderStream() {
	d.decoderStreamSender.Flush()
}

// MaximumDynamicTableCapacity returns the maximum dynamic table capacity in bytes.
func (d *Decoder) MaximumDynamicTableCapacity() uint64 {
	return d.headerTable.MaximumDynamicTableCapacity()
}

// DynamicTableCapacity returns the current dynamic table capacity in bytes.
func (d *Decoder) DynamicTableCapacity() uint64 {
	return d.headerTable.DynamicTableCapacity()
}

// KnownReceivedCount returns the current known received count.
func (d *Decoder) KnownReceivedCount() uint64 {
	return d.knownReceivedCount
}

// SendInsertCountIncrement sends an Insert Count Increment instruction on decoder stream.
func (d *Decoder) SendInsertCountIncrement(increment uint64) {
	d.decoderStreamSender.SendInsertCountIncrement(increment)
}

// SendSectionAcknowledgement sends a Section Acknowledgment instruction on decoder stream.
func (d *Decoder) SendSectionAcknowledgement(streamID uint64) {
	d.decoderStreamSender.SendSectionAcknowledgement(streamID)
}

// SendHeaderAcknowledgement is an alias for SendSectionAcknowledgement.
func (d *Decoder) SendHeaderAcknowledgement(streamID uint64) {
	d.decoderStreamSender.SendHeaderAcknowledgement(streamID)
}

// SendStreamCancellation sends a Stream Cancellation instruction on decoder stream.
func (d *Decoder) SendStreamCancellation(streamID uint64) {
	d.decoderStreamSender.SendStreamCancellation(streamID)
}

// Interface compliance assertions
var (
	_ EncoderStreamReceiverDelegate = (*Decoder)(nil)
	_ BlockedStreamLimitEnforcer    = (*Decoder)(nil)
	_ DecodingCompletedVisitor      = (*Decoder)(nil)
)
