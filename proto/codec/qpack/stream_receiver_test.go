// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package qpack

import (
	"encoding/hex"
	"strings"
	"testing"

	"github.com/lemon4ksan/foundation/testing/assert"
	"github.com/lemon4ksan/foundation/testing/require"
)

func hexToBytes(h string) []byte {
	b, err := hex.DecodeString(h)
	if err != nil {
		panic(err)
	}
	return b
}

// -----------------------------------------------------------------------------
// Mock Delegates
// -----------------------------------------------------------------------------

type insertWithNameRefRecord struct {
	isStatic  bool
	nameIndex uint64
	value     string
}

type insertWithoutNameRefRecord struct {
	name  string
	value string
}

type errorRecord struct {
	qpackError uint64
	message    string
}

type mockEncoderReceiverDelegate struct {
	insertWithNameRefCalls    []insertWithNameRefRecord
	insertWithoutNameRefCalls []insertWithoutNameRefRecord
	duplicateCalls            []uint64
	capacityCalls             []uint64
	errorCalls                []errorRecord
}

func (m *mockEncoderReceiverDelegate) InsertWithNameReference(isStatic bool, nameIndex uint64, value string) {
	m.insertWithNameRefCalls = append(m.insertWithNameRefCalls, insertWithNameRefRecord{
		isStatic:  isStatic,
		nameIndex: nameIndex,
		value:     value,
	})
}

func (m *mockEncoderReceiverDelegate) InsertWithoutNameReference(name, value string) {
	m.insertWithoutNameRefCalls = append(m.insertWithoutNameRefCalls, insertWithoutNameRefRecord{
		name:  name,
		value: value,
	})
}

func (m *mockEncoderReceiverDelegate) Duplicate(index uint64) {
	m.duplicateCalls = append(m.duplicateCalls, index)
}

func (m *mockEncoderReceiverDelegate) SetDynamicTableCapacity(capacity uint64) {
	m.capacityCalls = append(m.capacityCalls, capacity)
}

func (m *mockEncoderReceiverDelegate) Error(qpackError uint64, errorMessage string) {
	m.errorCalls = append(m.errorCalls, errorRecord{
		qpackError: qpackError,
		message:    errorMessage,
	})
}

type mockDecoderReceiverDelegate struct {
	incrementCalls    []uint64
	sectionAckCalls   []uint64
	cancellationCalls []uint64
	errorCalls        []errorRecord
}

func (m *mockDecoderReceiverDelegate) InsertCountIncrement(increment uint64) {
	m.incrementCalls = append(m.incrementCalls, increment)
}

func (m *mockDecoderReceiverDelegate) SectionAck(streamID uint64) {
	m.sectionAckCalls = append(m.sectionAckCalls, streamID)
}

func (m *mockDecoderReceiverDelegate) StreamCancellation(streamID uint64) {
	m.cancellationCalls = append(m.cancellationCalls, streamID)
}

func (m *mockDecoderReceiverDelegate) Error(qpackError uint64, errorMessage string) {
	m.errorCalls = append(m.errorCalls, errorRecord{
		qpackError: qpackError,
		message:    errorMessage,
	})
}

// -----------------------------------------------------------------------------
// Encoder Stream Receiver Tests (13 Chromium Suites)
// -----------------------------------------------------------------------------

// TEST_F(EncoderStreamReceiverTest, InsertWithNameReference)
func TestEncoderStreamReceiver_InsertWithNameReference(t *testing.T) {
	for _, mode := range []string{"FullChunk", "ByteByByte"} {
		t.Run(mode, func(t *testing.T) {
			delegate := &mockEncoderReceiverDelegate{}
			receiver := NewEncoderStreamReceiver(delegate)

			data := hexToBytes("c500" + "c28294e7" + "bf4a03626172" + "aa7f00")
			data = append(data, []byte(strings.Repeat("Z", 127))...)

			if mode == "FullChunk" {
				receiver.Decode(data)
			} else {
				for _, b := range data {
					receiver.Decode([]byte{b})
				}
			}

			require.Equal(t, 4, len(delegate.insertWithNameRefCalls))
			assert.Equal(t, 0, len(delegate.errorCalls))

			assert.Equal(t, true, delegate.insertWithNameRefCalls[0].isStatic)
			assert.Equal(t, uint64(5), delegate.insertWithNameRefCalls[0].nameIndex)
			assert.Equal(t, "", delegate.insertWithNameRefCalls[0].value)

			assert.Equal(t, true, delegate.insertWithNameRefCalls[1].isStatic)
			assert.Equal(t, uint64(2), delegate.insertWithNameRefCalls[1].nameIndex)
			assert.Equal(t, "foo", delegate.insertWithNameRefCalls[1].value)

			assert.Equal(t, false, delegate.insertWithNameRefCalls[2].isStatic)
			assert.Equal(t, uint64(137), delegate.insertWithNameRefCalls[2].nameIndex)
			assert.Equal(t, "bar", delegate.insertWithNameRefCalls[2].value)

			assert.Equal(t, false, delegate.insertWithNameRefCalls[3].isStatic)
			assert.Equal(t, uint64(42), delegate.insertWithNameRefCalls[3].nameIndex)
			assert.Equal(t, strings.Repeat("Z", 127), delegate.insertWithNameRefCalls[3].value)
		})
	}
}

// TEST_F(EncoderStreamReceiverTest, InsertWithNameReferenceIndexTooLarge)
func TestEncoderStreamReceiver_InsertWithNameReferenceIndexTooLarge(t *testing.T) {
	delegate := &mockEncoderReceiverDelegate{}
	receiver := NewEncoderStreamReceiver(delegate)

	receiver.Decode(hexToBytes("bfffffffffffffffffffffff"))

	require.Equal(t, 1, len(delegate.errorCalls))
	assert.Equal(t, uint64(QPACK_ENCODER_STREAM_ERROR), delegate.errorCalls[0].qpackError)
	assert.Equal(t, "Encoded integer too large.", delegate.errorCalls[0].message)
}

// TEST_F(EncoderStreamReceiverTest, InsertWithNameReferenceValueTooLong)
func TestEncoderStreamReceiver_InsertWithNameReferenceValueTooLong(t *testing.T) {
	delegate := &mockEncoderReceiverDelegate{}
	receiver := NewEncoderStreamReceiver(delegate)

	receiver.Decode(hexToBytes("c57fffffffffffffffffffff"))

	require.Equal(t, 1, len(delegate.errorCalls))
	assert.Equal(t, uint64(QPACK_ENCODER_STREAM_ERROR), delegate.errorCalls[0].qpackError)
	assert.Equal(t, "Encoded integer too large.", delegate.errorCalls[0].message)
}

// TEST_F(EncoderStreamReceiverTest, InsertWithoutNameReference)
func TestEncoderStreamReceiver_InsertWithoutNameReference(t *testing.T) {
	for _, mode := range []string{"FullChunk", "ByteByByte"} {
		t.Run(mode, func(t *testing.T) {
			delegate := &mockEncoderReceiverDelegate{}
			receiver := NewEncoderStreamReceiver(delegate)

			data := hexToBytes("4000" + "4362617203626172" + "6294e78294e7" + "5f00")
			data = append(data, []byte(strings.Repeat("Z", 31))...)
			data = append(data, hexToBytes("7f00")...)
			data = append(data, []byte(strings.Repeat("Z", 127))...)

			if mode == "FullChunk" {
				receiver.Decode(data)
			} else {
				for _, b := range data {
					receiver.Decode([]byte{b})
				}
			}

			require.Equal(t, 4, len(delegate.insertWithoutNameRefCalls))
			assert.Equal(t, 0, len(delegate.errorCalls))

			assert.Equal(t, "", delegate.insertWithoutNameRefCalls[0].name)
			assert.Equal(t, "", delegate.insertWithoutNameRefCalls[0].value)

			assert.Equal(t, "bar", delegate.insertWithoutNameRefCalls[1].name)
			assert.Equal(t, "bar", delegate.insertWithoutNameRefCalls[1].value)

			assert.Equal(t, "foo", delegate.insertWithoutNameRefCalls[2].name)
			assert.Equal(t, "foo", delegate.insertWithoutNameRefCalls[2].value)

			assert.Equal(t, strings.Repeat("Z", 31), delegate.insertWithoutNameRefCalls[3].name)
			assert.Equal(t, strings.Repeat("Z", 127), delegate.insertWithoutNameRefCalls[3].value)
		})
	}
}

// TEST_F(EncoderStreamReceiverTest, InsertWithoutNameReferenceNameTooLongForVarintDecoder)
func TestEncoderStreamReceiver_InsertWithoutNameReferenceNameTooLongForVarintDecoder(t *testing.T) {
	delegate := &mockEncoderReceiverDelegate{}
	receiver := NewEncoderStreamReceiver(delegate)

	receiver.Decode(hexToBytes("5fffffffffffffffffffff"))

	require.Equal(t, 1, len(delegate.errorCalls))
	assert.Equal(t, uint64(QPACK_ENCODER_STREAM_ERROR), delegate.errorCalls[0].qpackError)
	assert.Equal(t, "Encoded integer too large.", delegate.errorCalls[0].message)
}

// TEST_F(EncoderStreamReceiverTest, InsertWithoutNameReferenceNameExceedsLimit)
func TestEncoderStreamReceiver_InsertWithoutNameReferenceNameExceedsLimit(t *testing.T) {
	delegate := &mockEncoderReceiverDelegate{}
	receiver := NewEncoderStreamReceiver(delegate)

	receiver.Decode(hexToBytes("5fffff7f"))

	require.Equal(t, 1, len(delegate.errorCalls))
	assert.Equal(t, uint64(QPACK_ENCODER_STREAM_ERROR), delegate.errorCalls[0].qpackError)
	assert.Equal(t, "String literal too long.", delegate.errorCalls[0].message)
}

// TEST_F(EncoderStreamReceiverTest, InsertWithoutNameReferenceValueTooLongForVarintDecoder)
func TestEncoderStreamReceiver_InsertWithoutNameReferenceValueTooLongForVarintDecoder(t *testing.T) {
	delegate := &mockEncoderReceiverDelegate{}
	receiver := NewEncoderStreamReceiver(delegate)

	receiver.Decode(hexToBytes("436261727fffffffffffffffffffff"))

	require.Equal(t, 1, len(delegate.errorCalls))
	assert.Equal(t, uint64(QPACK_ENCODER_STREAM_ERROR), delegate.errorCalls[0].qpackError)
	assert.Equal(t, "Encoded integer too large.", delegate.errorCalls[0].message)
}

// TEST_F(EncoderStreamReceiverTest, InsertWithoutNameReferenceValueExceedsLimit)
func TestEncoderStreamReceiver_InsertWithoutNameReferenceValueExceedsLimit(t *testing.T) {
	delegate := &mockEncoderReceiverDelegate{}
	receiver := NewEncoderStreamReceiver(delegate)

	receiver.Decode(hexToBytes("436261727fffff7f"))

	require.Equal(t, 1, len(delegate.errorCalls))
	assert.Equal(t, uint64(QPACK_ENCODER_STREAM_ERROR), delegate.errorCalls[0].qpackError)
	assert.Equal(t, "String literal too long.", delegate.errorCalls[0].message)
}

// TEST_F(EncoderStreamReceiverTest, Duplicate)
func TestEncoderStreamReceiver_Duplicate(t *testing.T) {
	for _, mode := range []string{"FullChunk", "ByteByByte"} {
		t.Run(mode, func(t *testing.T) {
			delegate := &mockEncoderReceiverDelegate{}
			receiver := NewEncoderStreamReceiver(delegate)

			data := hexToBytes("111fd503")
			if mode == "FullChunk" {
				receiver.Decode(data)
			} else {
				for _, b := range data {
					receiver.Decode([]byte{b})
				}
			}

			require.Equal(t, 2, len(delegate.duplicateCalls))
			assert.Equal(t, 0, len(delegate.errorCalls))
			assert.Equal(t, uint64(17), delegate.duplicateCalls[0])
			assert.Equal(t, uint64(500), delegate.duplicateCalls[1])
		})
	}
}

// TEST_F(EncoderStreamReceiverTest, DuplicateIndexTooLarge)
func TestEncoderStreamReceiver_DuplicateIndexTooLarge(t *testing.T) {
	delegate := &mockEncoderReceiverDelegate{}
	receiver := NewEncoderStreamReceiver(delegate)

	receiver.Decode(hexToBytes("1fffffffffffffffffffff"))

	require.Equal(t, 1, len(delegate.errorCalls))
	assert.Equal(t, uint64(QPACK_ENCODER_STREAM_ERROR), delegate.errorCalls[0].qpackError)
	assert.Equal(t, "Encoded integer too large.", delegate.errorCalls[0].message)
}

// TEST_F(EncoderStreamReceiverTest, SetDynamicTableCapacity)
func TestEncoderStreamReceiver_SetDynamicTableCapacity(t *testing.T) {
	for _, mode := range []string{"FullChunk", "ByteByByte"} {
		t.Run(mode, func(t *testing.T) {
			delegate := &mockEncoderReceiverDelegate{}
			receiver := NewEncoderStreamReceiver(delegate)

			data := hexToBytes("313fd503")
			if mode == "FullChunk" {
				receiver.Decode(data)
			} else {
				for _, b := range data {
					receiver.Decode([]byte{b})
				}
			}

			require.Equal(t, 2, len(delegate.capacityCalls))
			assert.Equal(t, 0, len(delegate.errorCalls))
			assert.Equal(t, uint64(17), delegate.capacityCalls[0])
			assert.Equal(t, uint64(500), delegate.capacityCalls[1])
		})
	}
}

// TEST_F(EncoderStreamReceiverTest, SetDynamicTableCapacityTooLarge)
func TestEncoderStreamReceiver_SetDynamicTableCapacityTooLarge(t *testing.T) {
	delegate := &mockEncoderReceiverDelegate{}
	receiver := NewEncoderStreamReceiver(delegate)

	receiver.Decode(hexToBytes("3fffffffffffffffffffff"))

	require.Equal(t, 1, len(delegate.errorCalls))
	assert.Equal(t, uint64(QPACK_ENCODER_STREAM_ERROR), delegate.errorCalls[0].qpackError)
	assert.Equal(t, "Encoded integer too large.", delegate.errorCalls[0].message)
}

// TEST_F(EncoderStreamReceiverTest, InvalidHuffmanEncoding)
func TestEncoderStreamReceiver_InvalidHuffmanEncoding(t *testing.T) {
	delegate := &mockEncoderReceiverDelegate{}
	receiver := NewEncoderStreamReceiver(delegate)

	receiver.Decode(hexToBytes("c281ff"))

	require.Equal(t, 1, len(delegate.errorCalls))
	assert.Equal(t, uint64(QPACK_ENCODER_STREAM_ERROR), delegate.errorCalls[0].qpackError)
	assert.Equal(t, "Error in Huffman-encoded string.", delegate.errorCalls[0].message)
}

// Test edge cases: empty input, data after error, nil delegate
func TestEncoderStreamReceiver_EdgeCases(t *testing.T) {
	t.Run("EmptyData", func(t *testing.T) {
		delegate := &mockEncoderReceiverDelegate{}
		receiver := NewEncoderStreamReceiver(delegate)
		receiver.Decode([]byte{})
		receiver.Decode(nil)
		assert.Equal(t, 0, len(delegate.insertWithNameRefCalls))
		assert.Equal(t, 0, len(delegate.errorCalls))
	})

	t.Run("DataIgnoredAfterError", func(t *testing.T) {
		delegate := &mockEncoderReceiverDelegate{}
		receiver := NewEncoderStreamReceiver(delegate)
		// Trigger error
		receiver.Decode(hexToBytes("1fffffffffffffffffffff"))
		require.Equal(t, 1, len(delegate.errorCalls))

		// Subsequent valid data must be ignored
		receiver.Decode(hexToBytes("11"))
		assert.Equal(t, 0, len(delegate.duplicateCalls))
		assert.Equal(t, 1, len(delegate.errorCalls))
	})

	t.Run("NilDelegatePanic", func(t *testing.T) {
		assert.Panics(t, func() {
			NewEncoderStreamReceiver(nil)
		})
	})
}

// -----------------------------------------------------------------------------
// Decoder Stream Receiver Tests (3 Chromium Suites)
// -----------------------------------------------------------------------------

// TEST_F(DecoderStreamReceiverTest, InsertCountIncrement)
func TestDecoderStreamReceiver_InsertCountIncrement(t *testing.T) {
	testCases := []struct {
		hexBytes    string
		expectedInc uint64
	}{
		{"00", 0},
		{"0a", 10},
		{"3f00", 63},
		{"3f8901", 200},
	}

	for _, tc := range testCases {
		for _, mode := range []string{"FullChunk", "ByteByByte"} {
			t.Run(tc.hexBytes+"_"+mode, func(t *testing.T) {
				delegate := &mockDecoderReceiverDelegate{}
				receiver := NewDecoderStreamReceiver(delegate)

				data := hexToBytes(tc.hexBytes)
				if mode == "FullChunk" {
					receiver.Decode(data)
				} else {
					for _, b := range data {
						receiver.Decode([]byte{b})
					}
				}

				require.Equal(t, 1, len(delegate.incrementCalls))
				assert.Equal(t, tc.expectedInc, delegate.incrementCalls[0])
				assert.Equal(t, 0, len(delegate.errorCalls))
			})
		}
	}

	t.Run("OverflowError", func(t *testing.T) {
		delegate := &mockDecoderReceiverDelegate{}
		receiver := NewDecoderStreamReceiver(delegate)

		receiver.Decode(hexToBytes("3fffffffffffffffffffff"))

		require.Equal(t, 1, len(delegate.errorCalls))
		assert.Equal(t, uint64(QPACK_DECODER_STREAM_ERROR), delegate.errorCalls[0].qpackError)
		assert.Equal(t, "Encoded integer too large.", delegate.errorCalls[0].message)
	})
}

// TEST_F(DecoderStreamReceiverTest, HeaderAcknowledgement)
func TestDecoderStreamReceiver_HeaderAcknowledgement(t *testing.T) {
	testCases := []struct {
		hexBytes       string
		expectedStream uint64
	}{
		{"80", 0},
		{"a5", 37},
		{"ff00", 127},
		{"fff802", 503},
	}

	for _, tc := range testCases {
		for _, mode := range []string{"FullChunk", "ByteByByte"} {
			t.Run(tc.hexBytes+"_"+mode, func(t *testing.T) {
				delegate := &mockDecoderReceiverDelegate{}
				receiver := NewDecoderStreamReceiver(delegate)

				data := hexToBytes(tc.hexBytes)
				if mode == "FullChunk" {
					receiver.Decode(data)
				} else {
					for _, b := range data {
						receiver.Decode([]byte{b})
					}
				}

				require.Equal(t, 1, len(delegate.sectionAckCalls))
				assert.Equal(t, tc.expectedStream, delegate.sectionAckCalls[0])
				assert.Equal(t, 0, len(delegate.errorCalls))
			})
		}
	}

	t.Run("OverflowError", func(t *testing.T) {
		delegate := &mockDecoderReceiverDelegate{}
		receiver := NewDecoderStreamReceiver(delegate)

		receiver.Decode(hexToBytes("ffffffffffffffffffffff"))

		require.Equal(t, 1, len(delegate.errorCalls))
		assert.Equal(t, uint64(QPACK_DECODER_STREAM_ERROR), delegate.errorCalls[0].qpackError)
		assert.Equal(t, "Encoded integer too large.", delegate.errorCalls[0].message)
	})
}

// TEST_F(DecoderStreamReceiverTest, StreamCancellation)
func TestDecoderStreamReceiver_StreamCancellation(t *testing.T) {
	testCases := []struct {
		hexBytes       string
		expectedStream uint64
	}{
		{"40", 0},
		{"53", 19},
		{"7f00", 63},
		{"7f2f", 110},
	}

	for _, tc := range testCases {
		for _, mode := range []string{"FullChunk", "ByteByByte"} {
			t.Run(tc.hexBytes+"_"+mode, func(t *testing.T) {
				delegate := &mockDecoderReceiverDelegate{}
				receiver := NewDecoderStreamReceiver(delegate)

				data := hexToBytes(tc.hexBytes)
				if mode == "FullChunk" {
					receiver.Decode(data)
				} else {
					for _, b := range data {
						receiver.Decode([]byte{b})
					}
				}

				require.Equal(t, 1, len(delegate.cancellationCalls))
				assert.Equal(t, tc.expectedStream, delegate.cancellationCalls[0])
				assert.Equal(t, 0, len(delegate.errorCalls))
			})
		}
	}

	t.Run("OverflowError", func(t *testing.T) {
		delegate := &mockDecoderReceiverDelegate{}
		receiver := NewDecoderStreamReceiver(delegate)

		receiver.Decode(hexToBytes("7fffffffffffffffffffff"))

		require.Equal(t, 1, len(delegate.errorCalls))
		assert.Equal(t, uint64(QPACK_DECODER_STREAM_ERROR), delegate.errorCalls[0].qpackError)
		assert.Equal(t, "Encoded integer too large.", delegate.errorCalls[0].message)
	})
}

// Test edge cases: empty input, data after error, nil delegate
func TestDecoderStreamReceiver_EdgeCases(t *testing.T) {
	t.Run("EmptyData", func(t *testing.T) {
		delegate := &mockDecoderReceiverDelegate{}
		receiver := NewDecoderStreamReceiver(delegate)
		receiver.Decode([]byte{})
		receiver.Decode(nil)
		assert.Equal(t, 0, len(delegate.incrementCalls))
		assert.Equal(t, 0, len(delegate.errorCalls))
	})

	t.Run("DataIgnoredAfterError", func(t *testing.T) {
		delegate := &mockDecoderReceiverDelegate{}
		receiver := NewDecoderStreamReceiver(delegate)
		// Trigger error
		receiver.Decode(hexToBytes("7fffffffffffffffffffff"))
		require.Equal(t, 1, len(delegate.errorCalls))

		// Subsequent valid data must be ignored
		receiver.Decode(hexToBytes("00"))
		assert.Equal(t, 0, len(delegate.incrementCalls))
		assert.Equal(t, 1, len(delegate.errorCalls))
	})

	t.Run("NilDelegatePanic", func(t *testing.T) {
		assert.Panics(t, func() {
			NewDecoderStreamReceiver(nil)
		})
	})
}

// Test QuicErrorCode helper mapping
func TestQuicErrorCodeHelpers(t *testing.T) {
	assert.Equal(t, uint64(QUIC_QPACK_ENCODER_STREAM_INTEGER_TOO_LARGE),
		QuicErrorCodeFromEncoderInstructionError(InstructionDecoderIntegerTooLarge))
	assert.Equal(t, uint64(QUIC_QPACK_ENCODER_STREAM_STRING_LITERAL_TOO_LONG),
		QuicErrorCodeFromEncoderInstructionError(InstructionDecoderStringLiteralTooLong))
	assert.Equal(t, uint64(QUIC_QPACK_ENCODER_STREAM_HUFFMAN_ENCODING_ERROR),
		QuicErrorCodeFromEncoderInstructionError(InstructionDecoderHuffmanEncodingError))
	assert.Equal(t, uint64(0),
		QuicErrorCodeFromEncoderInstructionError(999))

	assert.Equal(t, uint64(QUIC_QPACK_DECODER_STREAM_INTEGER_TOO_LARGE),
		QuicErrorCodeFromDecoderInstructionError(InstructionDecoderIntegerTooLarge))
	assert.Equal(t, uint64(0),
		QuicErrorCodeFromDecoderInstructionError(InstructionDecoderStringLiteralTooLong))
}
