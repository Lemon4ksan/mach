// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package qpack

import (
	"bytes"
	"fmt"
	"math"
	"math/rand"
	"testing"

	"github.com/lemon4ksan/foundation/testing/assert"
	"github.com/lemon4ksan/foundation/testing/require"
)

// =============================================================================
// Adversarial Mock Delegates & Recorded Operation Types
// =============================================================================

type advEncoderOpType int

const (
	advOpInsertWithNameRef advEncoderOpType = iota
	advOpInsertWithoutNameRef
	advOpDuplicate
	advOpSetCapacity
)

type advEncoderOp struct {
	opType    advEncoderOpType
	isStatic  bool
	nameIndex uint64
	name      string
	value     string
	index     uint64
	capacity  uint64
}

type advErrorRecord struct {
	qpackError uint64
	message    string
}

type advEncoderReceiverDelegate struct {
	ops        []advEncoderOp
	errorCalls []advErrorRecord
}

func newAdvEncoderReceiverDelegate() *advEncoderReceiverDelegate {
	return &advEncoderReceiverDelegate{
		ops:        make([]advEncoderOp, 0),
		errorCalls: make([]advErrorRecord, 0),
	}
}

func (d *advEncoderReceiverDelegate) InsertWithNameReference(isStatic bool, nameIndex uint64, value string) {
	d.ops = append(d.ops, advEncoderOp{
		opType:    advOpInsertWithNameRef,
		isStatic:  isStatic,
		nameIndex: nameIndex,
		value:     value,
	})
}

func (d *advEncoderReceiverDelegate) InsertWithoutNameReference(name, value string) {
	d.ops = append(d.ops, advEncoderOp{
		opType: advOpInsertWithoutNameRef,
		name:   name,
		value:  value,
	})
}

func (d *advEncoderReceiverDelegate) Duplicate(index uint64) {
	d.ops = append(d.ops, advEncoderOp{
		opType: advOpDuplicate,
		index:  index,
	})
}

func (d *advEncoderReceiverDelegate) SetDynamicTableCapacity(capacity uint64) {
	d.ops = append(d.ops, advEncoderOp{
		opType:   advOpSetCapacity,
		capacity: capacity,
	})
}

func (d *advEncoderReceiverDelegate) Error(qpackError uint64, errorMessage string) {
	d.errorCalls = append(d.errorCalls, advErrorRecord{
		qpackError: qpackError,
		message:    errorMessage,
	})
}

type advDecoderOpType int

const (
	advOpSectionAck advDecoderOpType = iota
	advOpStreamCancellation
	advOpInsertCountIncrement
)

type advDecoderOp struct {
	opType    advDecoderOpType
	streamID  uint64
	increment uint64
}

type advDecoderReceiverDelegate struct {
	ops        []advDecoderOp
	errorCalls []advErrorRecord
}

func newAdvDecoderReceiverDelegate() *advDecoderReceiverDelegate {
	return &advDecoderReceiverDelegate{
		ops:        make([]advDecoderOp, 0),
		errorCalls: make([]advErrorRecord, 0),
	}
}

func (d *advDecoderReceiverDelegate) SectionAck(streamID uint64) {
	d.ops = append(d.ops, advDecoderOp{
		opType:   advOpSectionAck,
		streamID: streamID,
	})
}

func (d *advDecoderReceiverDelegate) StreamCancellation(streamID uint64) {
	d.ops = append(d.ops, advDecoderOp{
		opType:   advOpStreamCancellation,
		streamID: streamID,
	})
}

func (d *advDecoderReceiverDelegate) InsertCountIncrement(increment uint64) {
	d.ops = append(d.ops, advDecoderOp{
		opType:    advOpInsertCountIncrement,
		increment: increment,
	})
}

func (d *advDecoderReceiverDelegate) Error(qpackError uint64, errorMessage string) {
	d.errorCalls = append(d.errorCalls, advErrorRecord{
		qpackError: qpackError,
		message:    errorMessage,
	})
}

type advStreamSenderDelegate struct {
	writes [][]byte
}

func newAdvStreamSenderDelegate() *advStreamSenderDelegate {
	return &advStreamSenderDelegate{
		writes: make([][]byte, 0),
	}
}

func (d *advStreamSenderDelegate) WriteStreamData(data []byte) {
	// Store the slice reference directly to detect if sender mutates it post-flush.
	d.writes = append(d.writes, data)
}

func (d *advStreamSenderDelegate) NumBytesBuffered() uint64 {
	return 0
}

// =============================================================================
// Chunk Slicing Utilities
// =============================================================================

func splitChunksFixed(data []byte, chunkSize int) [][]byte {
	if chunkSize <= 0 || len(data) == 0 {
		return [][]byte{data}
	}
	var chunks [][]byte
	for len(data) > 0 {
		n := min(chunkSize, len(data))
		chunks = append(chunks, data[:n])
		data = data[n:]
	}
	return chunks
}

func splitChunksRandom(data []byte, minChunk, maxChunk int, seed int64) [][]byte {
	if len(data) == 0 {
		return [][]byte{}
	}
	rng := rand.New(rand.NewSource(seed))
	var chunks [][]byte
	for len(data) > 0 {
		span := maxChunk - minChunk + 1
		n := minChunk + rng.Intn(span)
		if n > len(data) {
			n = len(data)
		}
		chunks = append(chunks, data[:n])
		data = data[n:]
	}
	return chunks
}

func splitChunksPathological(data []byte) [][]byte {
	// Alternating pattern: 1, 3, 2, 7, 1, 4, 1, 2...
	pattern := []int{1, 3, 2, 7, 1, 4, 1, 2, 5, 1}
	var chunks [][]byte
	idx := 0
	for len(data) > 0 {
		size := pattern[idx%len(pattern)]
		idx++
		if size > len(data) {
			size = len(data)
		}
		chunks = append(chunks, data[:size])
		data = data[size:]
	}
	return chunks
}

// =============================================================================
// Target 1: End-to-End Sender-to-Receiver Roundtrip Pipeline
// =============================================================================

func TestM3StreamAdversarial_EncoderStream_Roundtrip_ChunkSplits(t *testing.T) {
	testEncoders := []struct {
		name    string
		huffman HuffmanEncoding
	}{
		{"HuffmanEnabled", HuffmanEncodingEnabled},
		{"HuffmanDisabled", HuffmanEncodingDisabled},
	}

	for _, tc := range testEncoders {
		t.Run(tc.name, func(t *testing.T) {
			senderDelegate := newAdvStreamSenderDelegate()
			sender := NewEncoderStreamSenderWithHuffman(tc.huffman, senderDelegate)

			expectedOps := []advEncoderOp{
				// InsertWithNameReference edge cases
				{opType: advOpInsertWithNameRef, isStatic: true, nameIndex: 0, value: ""},
				{opType: advOpInsertWithNameRef, isStatic: false, nameIndex: 1, value: "custom-val"},
				{opType: advOpInsertWithNameRef, isStatic: true, nameIndex: 62, value: "static-62"},
				{opType: advOpInsertWithNameRef, isStatic: false, nameIndex: 63, value: "prefix-63-boundary"},
				{opType: advOpInsertWithNameRef, isStatic: true, nameIndex: 1234567, value: "large-index"},
				// InsertWithoutNameReference edge cases
				{opType: advOpInsertWithoutNameRef, name: "", value: ""},
				{opType: advOpInsertWithoutNameRef, name: "header-name", value: "header-value"},
				{opType: advOpInsertWithoutNameRef, name: ":status", value: "200"},
				{
					opType: advOpInsertWithoutNameRef,
					name:   "long-name-string-for-boundary-validation",
					value:  "long-value-string-with-special-chars-~!@#$%^&*()_+`-={}|[]\\:\";'<>?,./",
				},
				// Duplicate edge cases
				{opType: advOpDuplicate, index: 0},
				{opType: advOpDuplicate, index: 30},
				{opType: advOpDuplicate, index: 31},
				{opType: advOpDuplicate, index: 32},
				{opType: advOpDuplicate, index: 999999},
				// SetDynamicTableCapacity edge cases
				{opType: advOpSetCapacity, capacity: 0},
				{opType: advOpSetCapacity, capacity: 30},
				{opType: advOpSetCapacity, capacity: 31},
				{opType: advOpSetCapacity, capacity: 32},
				{opType: advOpSetCapacity, capacity: 4096},
				{opType: advOpSetCapacity, capacity: math.MaxUint32},
				{opType: advOpSetCapacity, capacity: math.MaxUint64},
			}

			// Encode all expected operations into sender
			for _, op := range expectedOps {
				switch op.opType {
				case advOpInsertWithNameRef:
					sender.SendInsertWithNameReference(op.isStatic, op.nameIndex, op.value)
				case advOpInsertWithoutNameRef:
					sender.SendInsertWithoutNameReference(op.name, op.value)
				case advOpDuplicate:
					sender.SendDuplicate(op.index)
				case advOpSetCapacity:
					sender.SendSetDynamicTableCapacity(op.capacity)
				}
			}

			sender.Flush()
			require.Equal(t, 1, len(senderDelegate.writes))
			encodedBytes := senderDelegate.writes[0]
			require.True(t, len(encodedBytes) > 0)

			// Test strategies: Full, 1-byte, 2-byte, 3-byte, 7-byte, Pathological, and multiple Random seeds
			chunkingStrategies := []struct {
				name   string
				chunks [][]byte
			}{
				{"FullBuffer", [][]byte{encodedBytes}},
				{"1ByteChunks", splitChunksFixed(encodedBytes, 1)},
				{"2ByteChunks", splitChunksFixed(encodedBytes, 2)},
				{"3ByteChunks", splitChunksFixed(encodedBytes, 3)},
				{"7ByteChunks", splitChunksFixed(encodedBytes, 7)},
				{"PathologicalChunks", splitChunksPathological(encodedBytes)},
				{"RandomChunksSeed1", splitChunksRandom(encodedBytes, 1, 5, 42)},
				{"RandomChunksSeed2", splitChunksRandom(encodedBytes, 1, 13, 999)},
				{"RandomChunksSeed3", splitChunksRandom(encodedBytes, 2, 9, 1337)},
			}

			for _, cs := range chunkingStrategies {
				t.Run(cs.name, func(t *testing.T) {
					receiverDelegate := newAdvEncoderReceiverDelegate()
					receiver := NewEncoderStreamReceiver(receiverDelegate)

					for _, chunk := range cs.chunks {
						receiver.Decode(chunk)
					}
					receiver.EndDecoding()

					require.Equal(t, 0, len(receiverDelegate.errorCalls), "No errors expected in valid roundtrip")
					require.Equal(
						t,
						len(expectedOps),
						len(receiverDelegate.ops),
						"Received op count must match expected",
					)

					for i, expected := range expectedOps {
						actual := receiverDelegate.ops[i]
						assert.Equal(t, expected.opType, actual.opType, fmt.Sprintf("Op %d type mismatch", i))
						switch expected.opType {
						case advOpInsertWithNameRef:
							assert.Equal(t, expected.isStatic, actual.isStatic)
							assert.Equal(t, expected.nameIndex, actual.nameIndex)
							assert.Equal(t, expected.value, actual.value)
						case advOpInsertWithoutNameRef:
							assert.Equal(t, expected.name, actual.name)
							assert.Equal(t, expected.value, actual.value)
						case advOpDuplicate:
							assert.Equal(t, expected.index, actual.index)
						case advOpSetCapacity:
							assert.Equal(t, expected.capacity, actual.capacity)
						}
					}
				})
			}
		})
	}
}

func TestM3StreamAdversarial_DecoderStream_Roundtrip_ChunkSplits(t *testing.T) {
	senderDelegate := newAdvStreamSenderDelegate()
	sender := NewDecoderStreamSender(senderDelegate)

	expectedOps := []advDecoderOp{
		// SectionAck edge cases
		{opType: advOpSectionAck, streamID: 0},
		{opType: advOpSectionAck, streamID: 1},
		{opType: advOpSectionAck, streamID: 126},
		{opType: advOpSectionAck, streamID: 127},
		{opType: advOpSectionAck, streamID: 128},
		{opType: advOpSectionAck, streamID: 1000000},
		{opType: advOpSectionAck, streamID: math.MaxUint64},
		// StreamCancellation edge cases
		{opType: advOpStreamCancellation, streamID: 0},
		{opType: advOpStreamCancellation, streamID: 1},
		{opType: advOpStreamCancellation, streamID: 62},
		{opType: advOpStreamCancellation, streamID: 63},
		{opType: advOpStreamCancellation, streamID: 64},
		{opType: advOpStreamCancellation, streamID: 8888888},
		{opType: advOpStreamCancellation, streamID: math.MaxUint64},
		// InsertCountIncrement edge cases
		{opType: advOpInsertCountIncrement, increment: 1},
		{opType: advOpInsertCountIncrement, increment: 62},
		{opType: advOpInsertCountIncrement, increment: 63},
		{opType: advOpInsertCountIncrement, increment: 64},
		{opType: advOpInsertCountIncrement, increment: 7777777},
		{opType: advOpInsertCountIncrement, increment: math.MaxUint64},
	}

	for _, op := range expectedOps {
		switch op.opType {
		case advOpSectionAck:
			sender.SendSectionAcknowledgement(op.streamID)
		case advOpStreamCancellation:
			sender.SendStreamCancellation(op.streamID)
		case advOpInsertCountIncrement:
			sender.SendInsertCountIncrement(op.increment)
		}
	}

	sender.Flush()
	require.Equal(t, 1, len(senderDelegate.writes))
	encodedBytes := senderDelegate.writes[0]
	require.True(t, len(encodedBytes) > 0)

	chunkingStrategies := []struct {
		name   string
		chunks [][]byte
	}{
		{"FullBuffer", [][]byte{encodedBytes}},
		{"1ByteChunks", splitChunksFixed(encodedBytes, 1)},
		{"2ByteChunks", splitChunksFixed(encodedBytes, 2)},
		{"3ByteChunks", splitChunksFixed(encodedBytes, 3)},
		{"5ByteChunks", splitChunksFixed(encodedBytes, 5)},
		{"PathologicalChunks", splitChunksPathological(encodedBytes)},
		{"RandomChunksSeed1", splitChunksRandom(encodedBytes, 1, 4, 101)},
		{"RandomChunksSeed2", splitChunksRandom(encodedBytes, 1, 10, 202)},
	}

	for _, cs := range chunkingStrategies {
		t.Run(cs.name, func(t *testing.T) {
			receiverDelegate := newAdvDecoderReceiverDelegate()
			receiver := NewDecoderStreamReceiver(receiverDelegate)

			for _, chunk := range cs.chunks {
				receiver.Decode(chunk)
			}
			receiver.EndDecoding()

			require.Equal(t, 0, len(receiverDelegate.errorCalls), "No errors expected in valid roundtrip")
			require.Equal(t, len(expectedOps), len(receiverDelegate.ops), "Received op count must match expected")

			for i, expected := range expectedOps {
				actual := receiverDelegate.ops[i]
				assert.Equal(t, expected.opType, actual.opType)
				switch expected.opType {
				case advOpSectionAck, advOpStreamCancellation:
					assert.Equal(t, expected.streamID, actual.streamID)
				case advOpInsertCountIncrement:
					assert.Equal(t, expected.increment, actual.increment)
				}
			}
		})
	}
}

// =============================================================================
// Target 2: Malformed Byte Streams Fed to Receivers
// =============================================================================

func TestM3StreamAdversarial_EncoderReceiver_MalformedStreams(t *testing.T) {
	t.Run("VarintIntegerTooLarge_MoreThan10ExtensionBytes", func(t *testing.T) {
		// Duplicate opcode: 0x00, 5-bit prefix 0x1f.
		// Followed by 11 continuation bytes with MSB set, plus 0x00.
		malformed := []byte{0x1f}
		for i := 0; i < 11; i++ {
			malformed = append(malformed, 0x80)
		}
		malformed = append(malformed, 0x00)

		delegate := newAdvEncoderReceiverDelegate()
		receiver := NewEncoderStreamReceiver(delegate)
		receiver.Decode(malformed)

		require.Equal(t, 1, len(delegate.errorCalls))
		assert.Equal(t, uint64(QPACK_ENCODER_STREAM_ERROR), delegate.errorCalls[0].qpackError)
		assert.Equal(t, "Encoded integer too large.", delegate.errorCalls[0].message)
	})

	t.Run("VarintIntegerTooLarge_64BitOverflowAt10thByte", func(t *testing.T) {
		// Duplicate opcode 0x1f, then 9 bytes of 0xff, then 0x02 (causes summand > 1 at offset 63).
		malformed := []byte{0x1f}
		for i := 0; i < 9; i++ {
			malformed = append(malformed, 0xff)
		}
		malformed = append(malformed, 0x02)

		delegate := newAdvEncoderReceiverDelegate()
		receiver := NewEncoderStreamReceiver(delegate)
		receiver.Decode(malformed)

		require.Equal(t, 1, len(delegate.errorCalls))
		assert.Equal(t, uint64(QPACK_ENCODER_STREAM_ERROR), delegate.errorCalls[0].qpackError)
		assert.Equal(t, "Encoded integer too large.", delegate.errorCalls[0].message)
	})

	t.Run("StringLiteralTooLong_NameExceeds1MB", func(t *testing.T) {
		// InsertWithoutNameReference: opcode 0x40. 5-bit name length prefix.
		// Prefix = 31 (0x1f), wire varint = 1048546 (so length = 1048577 > 1MB).
		// 1048546 varint bytes: 0xe2, 0xff, 0x3f.
		malformed := []byte{0x40 | 0x1f, 0xe2, 0xff, 0x3f}

		delegate := newAdvEncoderReceiverDelegate()
		receiver := NewEncoderStreamReceiver(delegate)
		receiver.Decode(malformed)

		require.Equal(t, 1, len(delegate.errorCalls))
		assert.Equal(t, uint64(QPACK_ENCODER_STREAM_ERROR), delegate.errorCalls[0].qpackError)
		assert.Equal(t, "String literal too long.", delegate.errorCalls[0].message)
	})

	t.Run("StringLiteralTooLong_ValueExceeds1MB", func(t *testing.T) {
		// InsertWithNameReference: static=false, index=0 (0x80).
		// Value length: 7-bit prefix = 127 (0x7f), wire varint = 1048450 (so length = 1048577 > 1MB).
		// 1048450 varint bytes: 0x82, 0xff, 0x3f.
		malformed := []byte{0x80, 0x7f, 0x82, 0xff, 0x3f}

		delegate := newAdvEncoderReceiverDelegate()
		receiver := NewEncoderStreamReceiver(delegate)
		receiver.Decode(malformed)

		require.Equal(t, 1, len(delegate.errorCalls))
		assert.Equal(t, uint64(QPACK_ENCODER_STREAM_ERROR), delegate.errorCalls[0].qpackError)
		assert.Equal(t, "String literal too long.", delegate.errorCalls[0].message)
	})

	t.Run("InvalidHuffmanEncoding", func(t *testing.T) {
		// InsertWithNameReference: static=true, index=0 (0xc0).
		// Value: Huffman encoded (H bit 0x80 set), length 1 (0x81), payload 0x00 (invalid HPACK Huffman).
		malformed := []byte{0xc0, 0x81, 0x00}

		delegate := newAdvEncoderReceiverDelegate()
		receiver := NewEncoderStreamReceiver(delegate)
		receiver.Decode(malformed)

		require.Equal(t, 1, len(delegate.errorCalls))
		assert.Equal(t, uint64(QPACK_ENCODER_STREAM_ERROR), delegate.errorCalls[0].qpackError)
		assert.Equal(t, "Error in Huffman-encoded string.", delegate.errorCalls[0].message)
	})

	t.Run("TruncatedPayloads_UnexpectedEOF", func(t *testing.T) {
		truncatedCases := []struct {
			name string
			data []byte
		}{
			{"InsertWithNameRef_OpcodeOnly", []byte{0x80}},
			{"InsertWithNameRef_MissingValueBody", []byte{0x80, 0x05, 'a', 'b'}},
			{"InsertWithNameRef_IncompleteVarint", []byte{0xbf, 0x80}},
			{"InsertWithoutNameRef_OpcodeOnly", []byte{0x40}},
			{"InsertWithoutNameRef_IncompleteName", []byte{0x44, 'a', 'b'}},
			{"InsertWithoutNameRef_MissingValue", []byte{0x41, 'a', 0x03, 'x'}},
			{"Duplicate_IncompleteVarint", []byte{0x1f, 0x80}},
			{"SetDynamicTableCapacity_IncompleteVarint", []byte{0x3f, 0x80}},
		}

		for _, tc := range truncatedCases {
			t.Run(tc.name, func(t *testing.T) {
				delegate := newAdvEncoderReceiverDelegate()
				receiver := NewEncoderStreamReceiver(delegate)

				receiver.Decode(tc.data)
				// Receiver should still be waiting for more data, no error yet
				require.Equal(t, 0, len(delegate.errorCalls))

				// EndDecoding() signals unexpected EOF mid-instruction
				receiver.EndDecoding()
				require.Equal(t, 1, len(delegate.errorCalls))
				assert.Equal(t, uint64(QPACK_ENCODER_STREAM_ERROR), delegate.errorCalls[0].qpackError)
				assert.Equal(t, "Truncated instruction.", delegate.errorCalls[0].message)
			})
		}
	})

	t.Run("CleanEOF_AtInstructionBoundary", func(t *testing.T) {
		delegate := newAdvEncoderReceiverDelegate()
		receiver := NewEncoderStreamReceiver(delegate)

		// Duplicate index 0: complete 1-byte instruction 0x00
		receiver.Decode([]byte{0x00})
		require.Equal(t, 1, len(delegate.ops))
		require.Equal(t, 0, len(delegate.errorCalls))

		receiver.EndDecoding()
		// Clean EOF at boundary must not trigger error
		assert.Equal(t, 0, len(delegate.errorCalls))
	})

	t.Run("EmptyAndNilDecodeCalls", func(t *testing.T) {
		delegate := newAdvEncoderReceiverDelegate()
		receiver := NewEncoderStreamReceiver(delegate)

		receiver.Decode(nil)
		receiver.Decode([]byte{})
		assert.Equal(t, 0, len(delegate.ops))
		assert.Equal(t, 0, len(delegate.errorCalls))
	})
}

func TestM3StreamAdversarial_DecoderReceiver_MalformedStreams(t *testing.T) {
	t.Run("SectionAck_VarintIntegerTooLarge", func(t *testing.T) {
		// SectionAck opcode 0x80, 7-bit prefix 0x7f -> 0xff.
		// 11 continuation bytes.
		malformed := []byte{0xff}
		for i := 0; i < 11; i++ {
			malformed = append(malformed, 0x80)
		}
		malformed = append(malformed, 0x00)

		delegate := newAdvDecoderReceiverDelegate()
		receiver := NewDecoderStreamReceiver(delegate)
		receiver.Decode(malformed)

		require.Equal(t, 1, len(delegate.errorCalls))
		assert.Equal(t, uint64(QPACK_DECODER_STREAM_ERROR), delegate.errorCalls[0].qpackError)
		assert.Equal(t, "Encoded integer too large.", delegate.errorCalls[0].message)
	})

	t.Run("StreamCancellation_VarintIntegerTooLarge", func(t *testing.T) {
		// StreamCancellation opcode 0x40, 6-bit prefix 0x3f -> 0x7f.
		malformed := []byte{0x7f}
		for i := 0; i < 11; i++ {
			malformed = append(malformed, 0x80)
		}
		malformed = append(malformed, 0x00)

		delegate := newAdvDecoderReceiverDelegate()
		receiver := NewDecoderStreamReceiver(delegate)
		receiver.Decode(malformed)

		require.Equal(t, 1, len(delegate.errorCalls))
		assert.Equal(t, uint64(QPACK_DECODER_STREAM_ERROR), delegate.errorCalls[0].qpackError)
		assert.Equal(t, "Encoded integer too large.", delegate.errorCalls[0].message)
	})

	t.Run("InsertCountIncrement_VarintIntegerTooLarge", func(t *testing.T) {
		// InsertCountIncrement opcode 0x00, 6-bit prefix 0x3f -> 0x3f.
		malformed := []byte{0x3f}
		for i := 0; i < 11; i++ {
			malformed = append(malformed, 0x80)
		}
		malformed = append(malformed, 0x00)

		delegate := newAdvDecoderReceiverDelegate()
		receiver := NewDecoderStreamReceiver(delegate)
		receiver.Decode(malformed)

		require.Equal(t, 1, len(delegate.errorCalls))
		assert.Equal(t, uint64(QPACK_DECODER_STREAM_ERROR), delegate.errorCalls[0].qpackError)
		assert.Equal(t, "Encoded integer too large.", delegate.errorCalls[0].message)
	})

	t.Run("TruncatedPayloads_UnexpectedEOF", func(t *testing.T) {
		truncatedCases := []struct {
			name string
			data []byte
		}{
			{"SectionAck_IncompleteVarint", []byte{0xff, 0x80}},
			{"StreamCancellation_IncompleteVarint", []byte{0x7f, 0x80}},
			{"InsertCountIncrement_IncompleteVarint", []byte{0x3f, 0x80}},
		}

		for _, tc := range truncatedCases {
			t.Run(tc.name, func(t *testing.T) {
				delegate := newAdvDecoderReceiverDelegate()
				receiver := NewDecoderStreamReceiver(delegate)

				receiver.Decode(tc.data)
				require.Equal(t, 0, len(delegate.errorCalls))

				receiver.EndDecoding()
				require.Equal(t, 1, len(delegate.errorCalls))
				assert.Equal(t, uint64(QPACK_DECODER_STREAM_ERROR), delegate.errorCalls[0].qpackError)
				assert.Equal(t, "Truncated instruction.", delegate.errorCalls[0].message)
			})
		}
	})

	t.Run("CleanEOF_AtInstructionBoundary", func(t *testing.T) {
		delegate := newAdvDecoderReceiverDelegate()
		receiver := NewDecoderStreamReceiver(delegate)

		// InsertCountIncrement 1: complete 1-byte instruction 0x01
		receiver.Decode([]byte{0x01})
		require.Equal(t, 1, len(delegate.ops))
		require.Equal(t, 0, len(delegate.errorCalls))

		receiver.EndDecoding()
		assert.Equal(t, 0, len(delegate.errorCalls))
	})
}

// =============================================================================
// Target 3: Massive Instruction Coalescing & Memory Aliasing Prevention
// =============================================================================

func TestM3StreamAdversarial_EncoderSender_MassiveCoalescingAndMemoryIsolation(t *testing.T) {
	senderDelegate := newAdvStreamSenderDelegate()
	sender := NewEncoderStreamSender(senderDelegate)

	const totalInstructions = 1000
	var batch1Ops []advEncoderOp

	// 1. Queue 1,000 diverse instructions without flushing
	for i := 0; i < totalInstructions; i++ {
		switch i % 4 {
		case 0:
			op := advEncoderOp{
				opType:    advOpInsertWithNameRef,
				isStatic:  i%2 == 0,
				nameIndex: uint64(i * 3),
				value:     fmt.Sprintf("val-%d", i),
			}
			batch1Ops = append(batch1Ops, op)
			sender.SendInsertWithNameReference(op.isStatic, op.nameIndex, op.value)
		case 1:
			op := advEncoderOp{
				opType: advOpInsertWithoutNameRef,
				name:   fmt.Sprintf("header-%d", i),
				value:  fmt.Sprintf("value-%d", i),
			}
			batch1Ops = append(batch1Ops, op)
			sender.SendInsertWithoutNameReference(op.name, op.value)
		case 2:
			op := advEncoderOp{
				opType: advOpDuplicate,
				index:  uint64(i * 7),
			}
			batch1Ops = append(batch1Ops, op)
			sender.SendDuplicate(op.index)
		case 3:
			op := advEncoderOp{
				opType:   advOpSetCapacity,
				capacity: uint64(i * 128),
			}
			batch1Ops = append(batch1Ops, op)
			sender.SendSetDynamicTableCapacity(op.capacity)
		}
	}

	// Delegate must not have received any writes yet
	require.Equal(t, 0, len(senderDelegate.writes))

	// Single flush coalesces all 1,000 instructions
	sender.Flush()
	require.Equal(t, 1, len(senderDelegate.writes))
	batch1Bytes := senderDelegate.writes[0]
	require.True(t, len(batch1Bytes) > 0)

	// Make an independent copy of batch1Bytes to verify memory isolation
	batch1Clone := make([]byte, len(batch1Bytes))
	copy(batch1Clone, batch1Bytes)

	// Immediate redundant flush should do nothing (buffer empty)
	sender.Flush()
	require.Equal(t, 1, len(senderDelegate.writes), "Redundant flush must not emit empty write")

	// 2. Queue ANOTHER 500 instructions into the same sender instance
	const batch2Count = 500
	var batch2Ops []advEncoderOp
	for i := 0; i < batch2Count; i++ {
		op := advEncoderOp{
			opType: advOpDuplicate,
			index:  uint64(10000 + i),
		}
		batch2Ops = append(batch2Ops, op)
		sender.SendDuplicate(op.index)
	}

	sender.Flush()
	require.Equal(t, 2, len(senderDelegate.writes))
	batch2Bytes := senderDelegate.writes[1]
	require.True(t, len(batch2Bytes) > 0)

	// 3. Verify Memory Isolation: batch1Bytes must NOT have been mutated by batch2 allocations
	assert.True(t, bytes.Equal(batch1Clone, batch1Bytes), "Sender buffer reset must prevent memory aliasing")

	// 4. Verify roundtrip integrity: decode batch1 across random chunks into receiver
	receiverDelegate1 := newAdvEncoderReceiverDelegate()
	receiver1 := NewEncoderStreamReceiver(receiverDelegate1)
	chunks1 := splitChunksRandom(batch1Bytes, 1, 17, 777)
	for _, chunk := range chunks1 {
		receiver1.Decode(chunk)
	}
	receiver1.EndDecoding()

	require.Equal(t, 0, len(receiverDelegate1.errorCalls))
	require.Equal(t, totalInstructions, len(receiverDelegate1.ops))
	for i := 0; i < totalInstructions; i++ {
		assert.Equal(t, batch1Ops[i], receiverDelegate1.ops[i], fmt.Sprintf("Mismatch at instruction %d", i))
	}

	// 5. Verify batch2 decode integrity
	receiverDelegate2 := newAdvEncoderReceiverDelegate()
	receiver2 := NewEncoderStreamReceiver(receiverDelegate2)
	receiver2.Decode(batch2Bytes)
	receiver2.EndDecoding()

	require.Equal(t, 0, len(receiverDelegate2.errorCalls))
	require.Equal(t, batch2Count, len(receiverDelegate2.ops))
	for i := 0; i < batch2Count; i++ {
		assert.Equal(t, batch2Ops[i], receiverDelegate2.ops[i])
	}
}

func TestM3StreamAdversarial_DecoderSender_MassiveCoalescingAndMemoryIsolation(t *testing.T) {
	senderDelegate := newAdvStreamSenderDelegate()
	sender := NewDecoderStreamSender(senderDelegate)

	const totalInstructions = 1000
	var batch1Ops []advDecoderOp

	for i := 0; i < totalInstructions; i++ {
		switch i % 3 {
		case 0:
			op := advDecoderOp{
				opType:   advOpSectionAck,
				streamID: uint64(i * 2),
			}
			batch1Ops = append(batch1Ops, op)
			sender.SendSectionAcknowledgement(op.streamID)
		case 1:
			op := advDecoderOp{
				opType:   advOpStreamCancellation,
				streamID: uint64(i * 5),
			}
			batch1Ops = append(batch1Ops, op)
			sender.SendStreamCancellation(op.streamID)
		case 2:
			op := advDecoderOp{
				opType:    advOpInsertCountIncrement,
				increment: uint64(i + 1),
			}
			batch1Ops = append(batch1Ops, op)
			sender.SendInsertCountIncrement(op.increment)
		}
	}

	require.Equal(t, 0, len(senderDelegate.writes))

	sender.Flush()
	require.Equal(t, 1, len(senderDelegate.writes))
	batch1Bytes := senderDelegate.writes[0]
	batch1Clone := make([]byte, len(batch1Bytes))
	copy(batch1Clone, batch1Bytes)

	// Immediate redundant flush should be a no-op
	sender.Flush()
	require.Equal(t, 1, len(senderDelegate.writes))

	// Second batch of 500 instructions
	const batch2Count = 500
	var batch2Ops []advDecoderOp
	for i := 0; i < batch2Count; i++ {
		op := advDecoderOp{
			opType:    advOpInsertCountIncrement,
			increment: uint64(i + 100),
		}
		batch2Ops = append(batch2Ops, op)
		sender.SendInsertCountIncrement(op.increment)
	}

	sender.Flush()
	require.Equal(t, 2, len(senderDelegate.writes))

	// Verify buffer isolation
	assert.True(t, bytes.Equal(batch1Clone, batch1Bytes), "Decoder sender buffer reset must prevent memory aliasing")

	// Decode batch1 through receiver over pathological chunk splits
	receiverDelegate1 := newAdvDecoderReceiverDelegate()
	receiver1 := NewDecoderStreamReceiver(receiverDelegate1)
	for _, chunk := range splitChunksPathological(batch1Bytes) {
		receiver1.Decode(chunk)
	}
	receiver1.EndDecoding()

	require.Equal(t, 0, len(receiverDelegate1.errorCalls))
	require.Equal(t, totalInstructions, len(receiverDelegate1.ops))
	for i := 0; i < totalInstructions; i++ {
		assert.Equal(t, batch1Ops[i], receiverDelegate1.ops[i])
	}
}

// =============================================================================
// Target 4: Error Latching
// =============================================================================

func TestM3StreamAdversarial_EncoderReceiver_ErrorLatching(t *testing.T) {
	t.Run("IgnoresSubsequentValidDataAfterVarintOverflow", func(t *testing.T) {
		delegate := newAdvEncoderReceiverDelegate()
		receiver := NewEncoderStreamReceiver(delegate)

		// 1. Trigger error: Duplicate with 11 continuation bytes
		malformed := []byte{0x1f}
		for i := 0; i < 11; i++ {
			malformed = append(malformed, 0x80)
		}
		malformed = append(malformed, 0x00)

		receiver.Decode(malformed)
		require.Equal(t, 1, len(delegate.errorCalls))
		require.Equal(t, 0, len(delegate.ops))

		// 2. Feed 50 valid instructions (Duplicate 0..49)
		validSenderDelegate := newAdvStreamSenderDelegate()
		validSender := NewEncoderStreamSender(validSenderDelegate)
		for i := 0; i < 50; i++ {
			validSender.SendDuplicate(uint64(i))
		}
		validSender.Flush()
		require.Equal(t, 1, len(validSenderDelegate.writes))

		// Feed valid instructions chunk by chunk
		for _, b := range splitChunksFixed(validSenderDelegate.writes[0], 1) {
			receiver.Decode(b)
		}

		// Feed more malformed data
		receiver.Decode([]byte{0xff, 0xff, 0xff})

		// Call EndDecoding()
		receiver.EndDecoding()

		// Invariant: Delegate.Error() called EXACTLY ONCE, and NO instructions processed
		assert.Equal(t, 1, len(delegate.errorCalls), "Error must be latched: no subsequent error calls")
		assert.Equal(t, 0, len(delegate.ops), "All subsequent instructions must be ignored after error")
	})

	t.Run("HaltsDecodingImmediatelyWithinSameDataSlice", func(t *testing.T) {
		delegate := newAdvEncoderReceiverDelegate()
		receiver := NewEncoderStreamReceiver(delegate)

		// Create a single slice that has:
		// 1. One valid instruction: Duplicate 42 (0x1f, 0x0b -> 31+11 = 42)
		// 2. An error sequence: Varint overflow
		// 3. Another valid instruction: Duplicate 5
		validSenderDelegate := newAdvStreamSenderDelegate()
		validSender := NewEncoderStreamSender(validSenderDelegate)
		validSender.SendDuplicate(42)
		validSender.Flush()
		part1 := validSenderDelegate.writes[0]

		malformedPart := []byte{0x1f}
		for i := 0; i < 11; i++ {
			malformedPart = append(malformedPart, 0x80)
		}
		malformedPart = append(malformedPart, 0x00)

		validSender.SendDuplicate(5)
		validSender.Flush()
		part3 := validSenderDelegate.writes[1]

		combined := append([]byte{}, part1...)
		combined = append(combined, malformedPart...)
		combined = append(combined, part3...)

		receiver.Decode(combined)

		// Part 1 should have been decoded
		require.Equal(t, 1, len(delegate.ops))
		assert.Equal(t, uint64(42), delegate.ops[0].index)

		// Error should have been detected
		require.Equal(t, 1, len(delegate.errorCalls))

		// Part 3 must NOT have been decoded
		assert.Equal(t, 1, len(delegate.ops))
	})

	t.Run("LatchingAfterTruncatedInstructionEOF", func(t *testing.T) {
		delegate := newAdvEncoderReceiverDelegate()
		receiver := NewEncoderStreamReceiver(delegate)

		// Feed truncated instruction
		receiver.Decode([]byte{0x80})
		require.Equal(t, 0, len(delegate.errorCalls))

		// EOF triggers error
		receiver.EndDecoding()
		require.Equal(t, 1, len(delegate.errorCalls))

		// Subsequent calls to Decode or EndDecoding must be no-ops
		receiver.Decode([]byte{0x00}) // valid duplicate
		receiver.EndDecoding()

		assert.Equal(t, 1, len(delegate.errorCalls))
		assert.Equal(t, 0, len(delegate.ops))
	})
}

func TestM3StreamAdversarial_DecoderReceiver_ErrorLatching(t *testing.T) {
	delegate := newAdvDecoderReceiverDelegate()
	receiver := NewDecoderStreamReceiver(delegate)

	// 1. Trigger error: SectionAck varint overflow
	malformed := []byte{0xff}
	for i := 0; i < 11; i++ {
		malformed = append(malformed, 0x80)
	}
	malformed = append(malformed, 0x00)

	receiver.Decode(malformed)
	require.Equal(t, 1, len(delegate.errorCalls))
	require.Equal(t, 0, len(delegate.ops))

	// 2. Feed valid decoder stream instructions
	validSenderDelegate := newAdvStreamSenderDelegate()
	validSender := NewDecoderStreamSender(validSenderDelegate)
	for i := 0; i < 50; i++ {
		validSender.SendSectionAcknowledgement(uint64(i))
		validSender.SendStreamCancellation(uint64(i))
		validSender.SendInsertCountIncrement(uint64(i + 1))
	}
	validSender.Flush()

	receiver.Decode(validSenderDelegate.writes[0])
	receiver.EndDecoding()

	assert.Equal(t, 1, len(delegate.errorCalls), "Error must remain latched")
	assert.Equal(t, 0, len(delegate.ops), "No instructions should be dispatched post-error")
}

// =============================================================================
// Additional Edge Cases: Sender Delegate Switching & String Literal Boundary
// =============================================================================

func TestM3StreamAdversarial_SenderDelegateSwitchingAndNilResilience(t *testing.T) {
	// 1. Encoder sender created with nil delegate
	sender := NewEncoderStreamSender(nil)
	assert.Nil(t, sender.Delegate())

	// Sending and flushing with nil delegate must not panic
	sender.SendDuplicate(10)
	sender.Flush()

	// Setting delegate and sending again works as expected
	mock1 := newAdvStreamSenderDelegate()
	sender.SetDelegate(mock1)
	assert.Equal(t, mock1, sender.Delegate())

	sender.SendDuplicate(20)
	sender.Flush()
	require.Equal(t, 1, len(mock1.writes))

	// Switching to another delegate
	mock2 := newAdvStreamSenderDelegate()
	sender.SetDelegate(mock2)
	sender.SendDuplicate(30)
	sender.Flush()
	require.Equal(t, 1, len(mock2.writes))
	// mock1 should not have received write 2
	require.Equal(t, 1, len(mock1.writes))

	// 2. Decoder sender created with nil delegate
	decoderSender := NewDecoderStreamSender(nil)
	assert.Nil(t, decoderSender.Delegate())
	decoderSender.SendSectionAcknowledgement(1)
	decoderSender.Flush() // no panic

	decoderMock := newAdvStreamSenderDelegate()
	decoderSender.SetDelegate(decoderMock)
	decoderSender.SendSectionAcknowledgement(2)
	decoderSender.Flush()
	require.Equal(t, 1, len(decoderMock.writes))
}

func TestM3StreamAdversarial_StringLiteralLengthBoundary(t *testing.T) {
	// String literal limit is 1 MB = 1048576 bytes.
	// Check exactly 1048576 (allowed) vs 1048577 (rejected).

	// 1. Boundary + 1 (1048577 bytes): Rejected with StringLiteralTooLong
	// InsertWithoutNameRef: 0x40 | 0x1f, then varint(1048577 - 31 = 1048546) = 0xe2, 0xff, 0x3f
	delegateOver := newAdvEncoderReceiverDelegate()
	receiverOver := NewEncoderStreamReceiver(delegateOver)
	receiverOver.Decode([]byte{0x5f, 0xe2, 0xff, 0x3f})

	require.Equal(t, 1, len(delegateOver.errorCalls))
	assert.Equal(t, "String literal too long.", delegateOver.errorCalls[0].message)

	// 2. Exact boundary (1048576 bytes): Accepted by length check
	// Varint(1048576 - 31 = 1048545) = 0xe1, 0xff, 0x3f
	delegateExact := newAdvEncoderReceiverDelegate()
	receiverExact := NewEncoderStreamReceiver(delegateExact)
	receiverExact.Decode([]byte{0x5f, 0xe1, 0xff, 0x3f})

	// Must NOT report StringLiteralTooLong
	require.Equal(t, 0, len(delegateExact.errorCalls))

	// Calling EndDecoding() now will report TruncatedInstruction (waiting for body bytes), NOT StringLiteralTooLong!
	receiverExact.EndDecoding()
	require.Equal(t, 1, len(delegateExact.errorCalls))
	assert.Equal(t, "Truncated instruction.", delegateExact.errorCalls[0].message)
}
