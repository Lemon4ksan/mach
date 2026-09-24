// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package qpack

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/lemon4ksan/foundation/testing/assert"
	"github.com/lemon4ksan/foundation/testing/require"
)

// mockStreamSenderDelegate records calls to WriteStreamData.
type mockStreamSenderDelegate struct {
	writes           [][]byte
	numBytesBuffered uint64
}

func newMockStreamSenderDelegate() *mockStreamSenderDelegate {
	return &mockStreamSenderDelegate{
		writes: make([][]byte, 0),
	}
}

func (d *mockStreamSenderDelegate) WriteStreamData(data []byte) {
	cp := make([]byte, len(data))
	copy(cp, data)
	d.writes = append(d.writes, cp)
}

func (d *mockStreamSenderDelegate) NumBytesBuffered() uint64 {
	return d.numBytesBuffered
}

func decodeHexOrPanic(h string) []byte {
	b, err := hex.DecodeString(h)
	if err != nil {
		panic(err)
	}
	return b
}

// =============================================================================
// EncoderStreamSender Tests (Chromium qpack_encoder_stream_sender_test.cc)
// =============================================================================

// TEST_P(EncoderStreamSenderTest, InsertWithNameReference)
func TestEncoderStreamSender_InsertWithNameReference(t *testing.T) {
	modes := []struct {
		name        string
		huffman     HuffmanEncoding
		fooValueHex string
	}{
		{"HuffmanDisabled", HuffmanEncodingDisabled, "03666f6f"},
		{"HuffmanEnabled", HuffmanEncodingEnabled, "8294e7"},
	}

	for _, tc := range modes {
		t.Run(tc.name, func(t *testing.T) {
			delegate := newMockStreamSenderDelegate()
			sender := NewEncoderStreamSenderWithHuffman(tc.huffman, delegate)

			// 1. Static entry reference, small index: isStatic = true, nameIndex = 5, value = "foo"
			// First byte: 0x80 | 0x40 | 0x05 = 0xc5
			sender.SendInsertWithNameReference(true, 5, "foo")
			sender.Flush()
			require.Equal(t, 1, len(delegate.writes))
			expected := append([]byte{0xc5}, decodeHexOrPanic(tc.fooValueHex)...)
			assert.True(t, bytes.Equal(expected, delegate.writes[0]))

			// 2. Dynamic entry reference, medium index: isStatic = false, nameIndex = 42, value = "bar"
			// First byte: 0x80 | 0x00 | 0x2a = 0xaa
			// "bar" Huffman length (3) is not strictly less than raw length (3), so literal 03626172 in both modes.
			sender.SendInsertWithNameReference(false, 42, "bar")
			sender.Flush()
			require.Equal(t, 2, len(delegate.writes))
			expectedBar := decodeHexOrPanic("aa03626172")
			assert.True(t, bytes.Equal(expectedBar, delegate.writes[1]))

			// 3. Multi-byte prefix integer overflow: isStatic = false, nameIndex = 63 (prefix 6 bits: 63 -> 0x3f followed by 0x00)
			// First bytes: 0x80 | 0x3f = 0xbf, then 0x00. Value "baz" = 0362617a
			sender.SendInsertWithNameReference(false, 63, "baz")
			sender.Flush()
			require.Equal(t, 3, len(delegate.writes))
			expectedBaz := decodeHexOrPanic("bf000362617a")
			assert.True(t, bytes.Equal(expectedBaz, delegate.writes[2]))
		})
	}
}

// TEST_P(EncoderStreamSenderTest, InsertWithoutNameReference)
func TestEncoderStreamSender_InsertWithoutNameReference(t *testing.T) {
	modes := []struct {
		name        string
		huffman     HuffmanEncoding
		expectedHex string
	}{
		// Raw: name 0x40 | 0x03 = 0x43, "foo" = 666f6f, value 0x03, "bar" = 626172
		{"HuffmanDisabled", HuffmanEncodingDisabled, "43666f6f03626172"},
		// Huffman: name 0x40 | 0x20 | 0x02 = 0x62, 94e7, value 0x03, "bar" = 626172
		{"HuffmanEnabled", HuffmanEncodingEnabled, "6294e703626172"},
	}

	for _, tc := range modes {
		t.Run(tc.name, func(t *testing.T) {
			delegate := newMockStreamSenderDelegate()
			sender := NewEncoderStreamSenderWithHuffman(tc.huffman, delegate)

			sender.SendInsertWithoutNameReference("foo", "bar")
			sender.Flush()

			require.Equal(t, 1, len(delegate.writes))
			assert.True(t, bytes.Equal(decodeHexOrPanic(tc.expectedHex), delegate.writes[0]))
		})
	}
}

// TEST_F(EncoderStreamSenderTest, Duplicate)
func TestEncoderStreamSender_Duplicate(t *testing.T) {
	testCases := []struct {
		index       uint64
		expectedHex string
	}{
		{0, "00"},
		{17, "11"},
		{31, "1f00"},  // 5-bit prefix boundary
		{150, "1f77"}, // 150 - 31 = 119 = 0x77
	}

	for _, tc := range testCases {
		delegate := newMockStreamSenderDelegate()
		sender := NewEncoderStreamSender(delegate)

		sender.SendDuplicate(tc.index)
		sender.Flush()

		require.Equal(t, 1, len(delegate.writes))
		assert.True(t, bytes.Equal(decodeHexOrPanic(tc.expectedHex), delegate.writes[0]))
	}
}

// TEST_F(EncoderStreamSenderTest, SetDynamicTableCapacity)
func TestEncoderStreamSender_SetDynamicTableCapacity(t *testing.T) {
	testCases := []struct {
		capacity    uint64
		expectedHex string
	}{
		{0, "20"},
		{30, "3e"},
		{31, "3f00"},
		{1024, "3fe107"}, // 1024 - 31 = 993 -> 0xe1, 0x07
	}

	for _, tc := range testCases {
		delegate := newMockStreamSenderDelegate()
		sender := NewEncoderStreamSender(delegate)

		sender.SendSetDynamicTableCapacity(tc.capacity)
		sender.Flush()

		require.Equal(t, 1, len(delegate.writes))
		assert.True(t, bytes.Equal(decodeHexOrPanic(tc.expectedHex), delegate.writes[0]))
	}
}

// TEST_F(EncoderStreamSenderTest, Coalesce)
func TestEncoderStreamSender_Coalesce(t *testing.T) {
	delegate := newMockStreamSenderDelegate()
	sender := NewEncoderStreamSenderWithHuffman(HuffmanEncodingDisabled, delegate)

	sender.SendInsertWithNameReference(true, 5, "foo")
	sender.SendDuplicate(17)
	sender.SendSetDynamicTableCapacity(1024)

	// Before Flush, nothing should be written
	assert.Equal(t, 0, len(delegate.writes))

	sender.Flush()

	// After Flush, exactly one consolidated write
	require.Equal(t, 1, len(delegate.writes))
	expected := decodeHexOrPanic("c503666f6f" + "11" + "3fe107")
	assert.True(t, bytes.Equal(expected, delegate.writes[0]))

	// Calling Flush again on an empty buffer does nothing
	sender.Flush()
	assert.Equal(t, 1, len(delegate.writes))
}

// TEST_F(EncoderStreamSenderTest, FlushEmpty)
func TestEncoderStreamSender_FlushEmpty(t *testing.T) {
	delegate := newMockStreamSenderDelegate()
	sender := NewEncoderStreamSender(delegate)

	sender.Flush()
	assert.Equal(t, 0, len(delegate.writes))
}

// =============================================================================
// DecoderStreamSender Tests (Chromium qpack_decoder_stream_sender_test.cc)
// =============================================================================

// TEST_F(DecoderStreamSenderTest, InsertCountIncrement)
func TestDecoderStreamSender_InsertCountIncrement(t *testing.T) {
	testCases := []struct {
		increment   uint64
		expectedHex string
	}{
		{1, "01"},
		{42, "2a"},
		{63, "3f00"},  // 6-bit prefix boundary
		{100, "3f25"}, // 100 - 63 = 37 = 0x25
	}

	for _, tc := range testCases {
		delegate := newMockStreamSenderDelegate()
		sender := NewDecoderStreamSender(delegate)

		sender.SendInsertCountIncrement(tc.increment)
		sender.Flush()

		require.Equal(t, 1, len(delegate.writes))
		assert.True(t, bytes.Equal(decodeHexOrPanic(tc.expectedHex), delegate.writes[0]))
	}
}

// TEST_F(DecoderStreamSenderTest, HeaderAcknowledgement)
func TestDecoderStreamSender_SectionAcknowledgement(t *testing.T) {
	testCases := []struct {
		streamId    uint64
		expectedHex string
	}{
		{0, "80"},
		{37, "a5"},
		{127, "ff00"}, // 7-bit prefix boundary
		{128, "ff01"}, // 128 - 127 = 1
	}

	for _, tc := range testCases {
		// Test SendSectionAcknowledgement
		delegate := newMockStreamSenderDelegate()
		sender := NewDecoderStreamSender(delegate)

		sender.SendSectionAcknowledgement(tc.streamId)
		sender.Flush()

		require.Equal(t, 1, len(delegate.writes))
		assert.True(t, bytes.Equal(decodeHexOrPanic(tc.expectedHex), delegate.writes[0]))

		// Test SendHeaderAcknowledgement alias
		delegate2 := newMockStreamSenderDelegate()
		sender2 := NewDecoderStreamSender(delegate2)

		sender2.SendHeaderAcknowledgement(tc.streamId)
		sender2.Flush()

		require.Equal(t, 1, len(delegate2.writes))
		assert.True(t, bytes.Equal(decodeHexOrPanic(tc.expectedHex), delegate2.writes[0]))
	}
}

// TEST_F(DecoderStreamSenderTest, StreamCancellation)
func TestDecoderStreamSender_StreamCancellation(t *testing.T) {
	testCases := []struct {
		streamId    uint64
		expectedHex string
	}{
		{0, "40"},
		{10, "4a"},
		{63, "7f00"},  // 6-bit prefix boundary
		{100, "7f25"}, // 100 - 63 = 37 = 0x25
	}

	for _, tc := range testCases {
		delegate := newMockStreamSenderDelegate()
		sender := NewDecoderStreamSender(delegate)

		sender.SendStreamCancellation(tc.streamId)
		sender.Flush()

		require.Equal(t, 1, len(delegate.writes))
		assert.True(t, bytes.Equal(decodeHexOrPanic(tc.expectedHex), delegate.writes[0]))
	}
}

// TEST_F(DecoderStreamSenderTest, Coalesce)
func TestDecoderStreamSender_Coalesce(t *testing.T) {
	delegate := newMockStreamSenderDelegate()
	sender := NewDecoderStreamSender(delegate)

	sender.SendInsertCountIncrement(42)   // 2a
	sender.SendSectionAcknowledgement(37) // a5
	sender.SendStreamCancellation(10)     // 4a

	// Before Flush, nothing is written
	assert.Equal(t, 0, len(delegate.writes))

	sender.Flush()

	// Consolidated single write
	require.Equal(t, 1, len(delegate.writes))
	expected := decodeHexOrPanic("2aa54a")
	assert.True(t, bytes.Equal(expected, delegate.writes[0]))

	// Calling Flush again on an empty buffer does nothing
	sender.Flush()
	assert.Equal(t, 1, len(delegate.writes))
}

// TEST_F(DecoderStreamSenderTest, FlushEmpty)
func TestDecoderStreamSender_FlushEmpty(t *testing.T) {
	delegate := newMockStreamSenderDelegate()
	sender := NewDecoderStreamSender(delegate)

	sender.Flush()
	assert.Equal(t, 0, len(delegate.writes))
}

// Delegate setter and getter verification
func TestStreamSenders_DelegateAccessors(t *testing.T) {
	delegate1 := newMockStreamSenderDelegate()
	delegate2 := newMockStreamSenderDelegate()

	encSender := NewEncoderStreamSender(delegate1)
	assert.Equal(t, delegate1, encSender.Delegate())
	encSender.SetDelegate(delegate2)
	assert.Equal(t, delegate2, encSender.Delegate())

	decSender := NewDecoderStreamSender(delegate1)
	assert.Equal(t, delegate1, decSender.Delegate())
	decSender.SetDelegate(delegate2)
	assert.Equal(t, delegate2, decSender.Delegate())
}

// =============================================================================
// Roundtrip Verification Tests (Sender -> InstructionDecoder)
// =============================================================================

func TestStreamSenders_DecoderRoundtrip(t *testing.T) {
	// Verifies that data produced by EncoderStreamSender is fully decoded by
	// InstructionDecoder using EncoderStreamLanguage().
	encoderDelegate := newMockStreamSenderDelegate()
	encoderSender := NewEncoderStreamSenderWithHuffman(HuffmanEncodingDisabled, encoderDelegate)

	encoderSender.SendInsertWithNameReference(true, 5, "foo")
	encoderSender.SendDuplicate(17)
	encoderSender.Flush()

	require.Equal(t, 1, len(encoderDelegate.writes))

	decoderMock := newMockInstructionDecoderDelegate()
	instructionDecoder := NewInstructionDecoder(EncoderStreamLanguage(), decoderMock)

	ok := instructionDecoder.Decode(encoderDelegate.writes[0])
	assert.True(t, ok)
	assert.False(t, instructionDecoder.HasError())
	instructionDecoder.EndDecoding()
	assert.Equal(t, 2, decoderMock.decodedCount)

	// Verifies that data produced by DecoderStreamSender is fully decoded by
	// InstructionDecoder using DecoderStreamLanguage().
	decoderDelegate := newMockStreamSenderDelegate()
	decoderSender := NewDecoderStreamSender(decoderDelegate)

	decoderSender.SendSectionAcknowledgement(37)
	decoderSender.SendStreamCancellation(10)
	decoderSender.SendInsertCountIncrement(42)
	decoderSender.Flush()

	require.Equal(t, 1, len(decoderDelegate.writes))

	decoderStreamMock := newMockInstructionDecoderDelegate()
	decoderInstructionDecoder := NewInstructionDecoder(DecoderStreamLanguage(), decoderStreamMock)

	ok = decoderInstructionDecoder.Decode(decoderDelegate.writes[0])
	assert.True(t, ok)
	assert.False(t, decoderInstructionDecoder.HasError())
	decoderInstructionDecoder.EndDecoding()
	assert.Equal(t, 3, decoderStreamMock.decodedCount)
}
