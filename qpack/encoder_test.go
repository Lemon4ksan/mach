// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package qpack

import (
	"math"
	"strings"
	"testing"

	"github.com/lemon4ksan/foundation/testing/assert"
	"github.com/lemon4ksan/foundation/testing/require"
)

// A number larger than maxBytesBufferedByStream (64KB).
// Returning this value from NumBytesBuffered() will instruct Encoder
// not to generate any instructions for the encoder stream.
const (
	tooManyBytesBuffered  uint64 = 1024 * 1024
	kTooManyBytesBuffered        = tooManyBytesBuffered
)

// mockDecoderStreamErrorDelegate records invocations of OnDecoderStreamError.
type decoderStreamErrorCall struct {
	ErrorCode    uint64
	ErrorMessage string
}

type mockDecoderStreamErrorDelegate struct {
	calls []decoderStreamErrorCall
}

func newMockDecoderStreamErrorDelegate() *mockDecoderStreamErrorDelegate {
	return &mockDecoderStreamErrorDelegate{
		calls: make([]decoderStreamErrorCall, 0),
	}
}

func (m *mockDecoderStreamErrorDelegate) OnDecoderStreamError(errorCode uint64, errorMessage string) {
	m.calls = append(m.calls, decoderStreamErrorCall{
		ErrorCode:    errorCode,
		ErrorMessage: errorMessage,
	})
}

// qpackEncoderTestFixture mimics Chromium's EncoderTest fixture.
type qpackEncoderTestFixture struct {
	t                           *testing.T
	huffman                     HuffmanEncoding
	cookieCrumbling             CookieCrumbling
	decoderStreamErrorDelegate  *mockDecoderStreamErrorDelegate
	encoderStreamSenderDelegate *mockStreamSenderDelegate
	encoder                     *Encoder
	encoderStreamSentByteCount  uint64
}

func newEncoderTestFixture(t *testing.T, huffman HuffmanEncoding, cookie CookieCrumbling) *qpackEncoderTestFixture {
	decoderError := newMockDecoderStreamErrorDelegate()
	senderDelegate := newMockStreamSenderDelegate()

	encoder := NewEncoder(decoderError.OnDecoderStreamError, huffman, cookie)
	encoder.SetStreamSenderDelegate(senderDelegate)
	encoder.SetMaximumBlockedStreams(1)

	return &qpackEncoderTestFixture{
		t:                           t,
		huffman:                     huffman,
		cookieCrumbling:             cookie,
		decoderStreamErrorDelegate:  decoderError,
		encoderStreamSenderDelegate: senderDelegate,
		encoder:                     encoder,
		encoderStreamSentByteCount:  0,
	}
}

func (f *qpackEncoderTestFixture) HuffmanEnabled() bool {
	return f.huffman == HuffmanEncodingEnabled
}

func (f *qpackEncoderTestFixture) Encode(fields []HeaderField) []byte {
	return f.encoder.EncodeHeaderList(1, fields, &f.encoderStreamSentByteCount)
}

func (f *qpackEncoderTestFixture) EncodeStream(streamID uint64, fields []HeaderField) []byte {
	return f.encoder.EncodeHeaderList(streamID, fields, &f.encoderStreamSentByteCount)
}

func (f *qpackEncoderTestFixture) WrittenData() []byte {
	var total []byte
	for _, w := range f.encoderStreamSenderDelegate.writes {
		total = append(total, w...)
	}
	return total
}

func (f *qpackEncoderTestFixture) ClearWrites() {
	f.encoderStreamSenderDelegate.writes = nil
}

func runEncoderTest(t *testing.T, testFn func(t *testing.T, f *qpackEncoderTestFixture)) {
	t.Run("HuffmanEnabled", func(t *testing.T) {
		f := newEncoderTestFixture(t, HuffmanEncodingEnabled, CookieCrumblingEnabled)
		testFn(t, f)
	})
	t.Run("HuffmanDisabled", func(t *testing.T) {
		f := newEncoderTestFixture(t, HuffmanEncodingDisabled, CookieCrumblingEnabled)
		testFn(t, f)
	})
}

func runEncoderTestWithCookie(
	t *testing.T,
	cookie CookieCrumbling,
	testFn func(t *testing.T, f *qpackEncoderTestFixture),
) {
	t.Run("HuffmanEnabled", func(t *testing.T) {
		f := newEncoderTestFixture(t, HuffmanEncodingEnabled, cookie)
		testFn(t, f)
	})
	t.Run("HuffmanDisabled", func(t *testing.T) {
		f := newEncoderTestFixture(t, HuffmanEncodingDisabled, cookie)
		testFn(t, f)
	})
}

// =============================================================================
// Chromium Unit Tests (27 test cases x 2 modes = 54 parameterized tests)
// =============================================================================

// TEST_P(EncoderTest, Empty)
func TestEncoder_Empty(t *testing.T) {
	runEncoderTest(t, func(t *testing.T, f *qpackEncoderTestFixture) {
		headerList := []HeaderField{}
		output := f.Encode(headerList)

		expected := decodeHexOrPanic("0000")
		require.Equal(t, expected, output)
		assert.Equal(t, uint64(0), f.encoderStreamSentByteCount)
		assert.Equal(t, 0, len(f.encoderStreamSenderDelegate.writes))
	})
}

// TEST_P(EncoderTest, EmptyName)
func TestEncoder_EmptyName(t *testing.T) {
	runEncoderTest(t, func(t *testing.T, f *qpackEncoderTestFixture) {
		headerList := []HeaderField{{Name: "", Value: "foo"}}
		output := f.Encode(headerList)

		var expected []byte
		if f.HuffmanEnabled() {
			expected = decodeHexOrPanic("0000208294e7")
		} else {
			expected = decodeHexOrPanic("00002003666f6f")
		}
		require.Equal(t, expected, output)
		assert.Equal(t, uint64(0), f.encoderStreamSentByteCount)
	})
}

// TEST_P(EncoderTest, EmptyValue)
func TestEncoder_EmptyValue(t *testing.T) {
	runEncoderTest(t, func(t *testing.T, f *qpackEncoderTestFixture) {
		headerList := []HeaderField{{Name: "foo", Value: ""}}
		output := f.Encode(headerList)

		var expected []byte
		if f.HuffmanEnabled() {
			expected = decodeHexOrPanic("00002a94e700")
		} else {
			expected = decodeHexOrPanic("000023666f6f00")
		}
		require.Equal(t, expected, output)
		assert.Equal(t, uint64(0), f.encoderStreamSentByteCount)
	})
}

// TEST_P(EncoderTest, EmptyNameAndValue)
func TestEncoder_EmptyNameAndValue(t *testing.T) {
	runEncoderTest(t, func(t *testing.T, f *qpackEncoderTestFixture) {
		headerList := []HeaderField{{Name: "", Value: ""}}
		output := f.Encode(headerList)

		expected := decodeHexOrPanic("00002000")
		require.Equal(t, expected, output)
		assert.Equal(t, uint64(0), f.encoderStreamSentByteCount)
	})
}

// TEST_P(EncoderTest, Simple)
func TestEncoder_Simple(t *testing.T) {
	runEncoderTest(t, func(t *testing.T, f *qpackEncoderTestFixture) {
		headerList := []HeaderField{{Name: "foo", Value: "bar"}}
		output := f.Encode(headerList)

		var expected []byte
		if f.HuffmanEnabled() {
			expected = decodeHexOrPanic("00002a94e703626172")
		} else {
			expected = decodeHexOrPanic("000023666f6f03626172")
		}
		require.Equal(t, expected, output)
		assert.Equal(t, uint64(0), f.encoderStreamSentByteCount)
	})
}

// TEST_P(EncoderTest, Multiple)
func TestEncoder_Multiple(t *testing.T) {
	runEncoderTest(t, func(t *testing.T, f *qpackEncoderTestFixture) {
		headerList := []HeaderField{
			{Name: "foo", Value: "bar"},
			{Name: "ZZZZZZZ", Value: strings.Repeat("Z", 127)},
		}
		output := f.Encode(headerList)

		var expectedHex string
		if f.HuffmanEnabled() {
			expectedHex = "0000" + "2a94e703626172"
		} else {
			expectedHex = "0000" + "23666f6f03626172"
		}
		expectedHex += "27005a5a5a5a5a5a5a" + "7f00" + strings.Repeat("5a", 127)
		expected := decodeHexOrPanic(expectedHex)
		require.Equal(t, expected, output)
		assert.Equal(t, uint64(0), f.encoderStreamSentByteCount)
	})
}

// TEST_P(EncoderTest, StaticTable)
func TestEncoder_StaticTable(t *testing.T) {
	runEncoderTest(t, func(t *testing.T, f *qpackEncoderTestFixture) {
		// Sub-block 1
		{
			headerList := []HeaderField{
				{Name: ":method", Value: "GET"},
				{Name: "accept-encoding", Value: "gzip, deflate, br"},
				{Name: "location", Value: ""},
			}
			output := f.Encode(headerList)
			expected := decodeHexOrPanic("0000d1dfcc")
			require.Equal(t, expected, output)
			assert.Equal(t, uint64(0), f.encoderStreamSentByteCount)
		}

		// Sub-block 2
		{
			headerList := []HeaderField{
				{Name: ":method", Value: "POST"},
				{Name: "accept-encoding", Value: "compress"},
				{Name: "location", Value: "foo"},
			}
			output := f.Encode(headerList)
			var expected []byte
			if f.HuffmanEnabled() {
				expected = decodeHexOrPanic("0000d45f108621e9aec2a11f5c8294e7")
			} else {
				expected = decodeHexOrPanic("0000d45f1008636f6d70726573735c03666f6f")
			}
			require.Equal(t, expected, output)
			assert.Equal(t, uint64(0), f.encoderStreamSentByteCount)
		}

		// Sub-block 3
		{
			headerList := []HeaderField{
				{Name: ":method", Value: "TRACE"},
				{Name: "accept-encoding", Value: ""},
			}
			output := f.Encode(headerList)
			expected := decodeHexOrPanic("00005f000554524143455f1000")
			require.Equal(t, expected, output)
			assert.Equal(t, uint64(0), f.encoderStreamSentByteCount)
		}
	})
}

// TEST_P(EncoderTest, DecoderStreamError)
func TestEncoder_DecoderStreamError(t *testing.T) {
	runEncoderTest(t, func(t *testing.T, f *qpackEncoderTestFixture) {
		decoderErrorDelegate := newMockDecoderStreamErrorDelegate()
		senderDelegate := newMockStreamSenderDelegate()

		encoder := NewEncoder(decoderErrorDelegate.OnDecoderStreamError, f.huffman, CookieCrumblingEnabled)
		encoder.SetStreamSenderDelegate(senderDelegate)

		input := decodeHexOrPanic("ffffffffffffffffffffff")
		encoder.DecoderStreamReceiver().Decode(input)

		require.Equal(t, 1, len(decoderErrorDelegate.calls))
		assert.Equal(t, QUIC_QPACK_DECODER_STREAM_INTEGER_TOO_LARGE, decoderErrorDelegate.calls[0].ErrorCode)
		assert.Equal(t, "Encoded integer too large.", decoderErrorDelegate.calls[0].ErrorMessage)
	})
}

// TEST_P(EncoderTest, SplitAlongNullCharacter)
func TestEncoder_SplitAlongNullCharacter(t *testing.T) {
	runEncoderTest(t, func(t *testing.T, f *qpackEncoderTestFixture) {
		headerList := []HeaderField{
			{Name: "foo", Value: "bar\x00bar\x00baz"},
		}
		output := f.Encode(headerList)

		var expected []byte
		if f.HuffmanEnabled() {
			expected = decodeHexOrPanic("0000" +
				"2a94e703626172" +
				"2a94e703626172" +
				"2a94e70362617a")
		} else {
			expected = decodeHexOrPanic("0000" +
				"23666f6f03626172" +
				"23666f6f03626172" +
				"23666f6f0362617a")
		}
		require.Equal(t, expected, output)
		assert.Equal(t, uint64(0), f.encoderStreamSentByteCount)
	})
}

// TEST_P(EncoderTest, ZeroInsertCountIncrement)
func TestEncoder_ZeroInsertCountIncrement(t *testing.T) {
	runEncoderTest(t, func(t *testing.T, f *qpackEncoderTestFixture) {
		f.encoder.OnInsertCountIncrement(0)

		require.Equal(t, 1, len(f.decoderStreamErrorDelegate.calls))
		assert.Equal(
			t,
			QUIC_QPACK_DECODER_STREAM_INVALID_ZERO_INCREMENT,
			f.decoderStreamErrorDelegate.calls[0].ErrorCode,
		)
		assert.Equal(t, "Invalid increment value 0.", f.decoderStreamErrorDelegate.calls[0].ErrorMessage)
	})
}

// TEST_P(EncoderTest, TooLargeInsertCountIncrement)
func TestEncoder_TooLargeInsertCountIncrement(t *testing.T) {
	runEncoderTest(t, func(t *testing.T, f *qpackEncoderTestFixture) {
		f.encoder.OnInsertCountIncrement(1)

		require.Equal(t, 1, len(f.decoderStreamErrorDelegate.calls))
		assert.Equal(
			t,
			QUIC_QPACK_DECODER_STREAM_IMPOSSIBLE_INSERT_COUNT,
			f.decoderStreamErrorDelegate.calls[0].ErrorCode,
		)
		assert.Equal(
			t,
			"Increment value 1 raises known received count to 1 exceeding inserted entry count 0",
			f.decoderStreamErrorDelegate.calls[0].ErrorMessage,
		)
	})
}

// TEST_P(EncoderTest, InsertCountIncrementOverflow)
func TestEncoder_InsertCountIncrementOverflow(t *testing.T) {
	runEncoderTest(t, func(t *testing.T, f *qpackEncoderTestFixture) {
		headerTable := EncoderPeerHeaderTable(f.encoder)
		headerTable.SetMaximumDynamicTableCapacity(4096)
		headerTable.SetDynamicTableCapacity(4096)
		headerTable.InsertEntry("foo", "bar")

		f.encoder.OnInsertCountIncrement(1)
		assert.Equal(t, 0, len(f.decoderStreamErrorDelegate.calls))

		f.encoder.OnInsertCountIncrement(math.MaxUint64)
		require.Equal(t, 1, len(f.decoderStreamErrorDelegate.calls))
		assert.Equal(t, QUIC_QPACK_DECODER_STREAM_INCREMENT_OVERFLOW, f.decoderStreamErrorDelegate.calls[0].ErrorCode)
		assert.Equal(
			t,
			"Insert Count Increment instruction causes overflow.",
			f.decoderStreamErrorDelegate.calls[0].ErrorMessage,
		)
	})
}

// TEST_P(EncoderTest, InvalidHeaderAcknowledgement)
func TestEncoder_InvalidHeaderAcknowledgement(t *testing.T) {
	runEncoderTest(t, func(t *testing.T, f *qpackEncoderTestFixture) {
		f.encoder.OnHeaderAcknowledgement(0)

		require.Equal(t, 1, len(f.decoderStreamErrorDelegate.calls))
		assert.Equal(
			t,
			QUIC_QPACK_DECODER_STREAM_INCORRECT_ACKNOWLEDGEMENT,
			f.decoderStreamErrorDelegate.calls[0].ErrorCode,
		)
		assert.Equal(
			t,
			"Header Acknowledgement received for stream 0 with no outstanding header blocks.",
			f.decoderStreamErrorDelegate.calls[0].ErrorMessage,
		)
	})
}

// TEST_P(EncoderTest, DynamicTable)
func TestEncoder_DynamicTable(t *testing.T) {
	runEncoderTest(t, func(t *testing.T, f *qpackEncoderTestFixture) {
		f.encoder.SetMaximumBlockedStreams(1)
		f.encoder.SetMaximumDynamicTableCapacity(4096)
		f.encoder.SetDynamicTableCapacity(4096)

		headerList := []HeaderField{
			{Name: "foo", Value: "bar"},
			{Name: "foo", Value: "baz"},
			{Name: "cookie", Value: "baz"},
		}

		setDynamicTableCapacity := decodeHexOrPanic("3fe11f")
		var insertEntriesHex string
		if f.HuffmanEnabled() {
			insertEntriesHex = "62" + "94e7"
		} else {
			insertEntriesHex = "43" + "666f6f"
		}
		insertEntriesHex += "03626172" + "80" + "0362617a" + "c5" + "0362617a"
		insertEntries := decodeHexOrPanic(insertEntriesHex)

		expectedStreamData := append(setDynamicTableCapacity, insertEntries...)

		output := f.Encode(headerList)
		expectedOutput := decodeHexOrPanic("0400" + "828180")
		require.Equal(t, expectedOutput, output)

		assert.Equal(t, expectedStreamData, f.WrittenData())
		assert.Equal(t, uint64(len(insertEntries)), f.encoderStreamSentByteCount)
	})
}

// TEST_P(EncoderTest, SmallDynamicTable)
func TestEncoder_SmallDynamicTable(t *testing.T) {
	runEncoderTest(t, func(t *testing.T, f *qpackEncoderTestFixture) {
		entrySize := EntrySize("foo", "bar")
		f.encoder.SetMaximumBlockedStreams(1)
		f.encoder.SetMaximumDynamicTableCapacity(entrySize)
		f.encoder.SetDynamicTableCapacity(entrySize)

		headerList := []HeaderField{
			{Name: "foo", Value: "bar"},
			{Name: "foo", Value: "baz"},
			{Name: "cookie", Value: "baz"},
			{Name: "bar", Value: "baz"},
		}

		setDynamicTableCapacity := decodeHexOrPanic("3f07")
		var insertEntry []byte
		if f.HuffmanEnabled() {
			insertEntry = decodeHexOrPanic("62" + "94e7" + "03626172")
		} else {
			insertEntry = decodeHexOrPanic("43" + "666f6f" + "03626172")
		}
		expectedStreamData := append(setDynamicTableCapacity, insertEntry...)

		output := f.Encode(headerList)
		expectedOutput := decodeHexOrPanic("0200" +
			"80" +
			"40" + "0362617a" +
			"55" + "0362617a" +
			"23626172" + "0362617a")
		require.Equal(t, expectedOutput, output)

		assert.Equal(t, expectedStreamData, f.WrittenData())
		assert.Equal(t, uint64(len(insertEntry)), f.encoderStreamSentByteCount)
	})
}

// TEST_P(EncoderTest, BlockedStream)
func TestEncoder_BlockedStream(t *testing.T) {
	runEncoderTest(t, func(t *testing.T, f *qpackEncoderTestFixture) {
		f.encoder.SetMaximumBlockedStreams(1)
		f.encoder.SetMaximumDynamicTableCapacity(4096)
		f.encoder.SetDynamicTableCapacity(4096)

		headerList1 := []HeaderField{{Name: "foo", Value: "bar"}}

		setDynamicTableCapacity := decodeHexOrPanic("3fe11f")
		var insertEntry1 []byte
		if f.HuffmanEnabled() {
			insertEntry1 = decodeHexOrPanic("62" + "94e7" + "03626172")
		} else {
			insertEntry1 = decodeHexOrPanic("43" + "666f6f" + "03626172")
		}
		expectedStreamData1 := append(setDynamicTableCapacity, insertEntry1...)

		output1 := f.EncodeStream(1, headerList1)
		require.Equal(t, decodeHexOrPanic("020080"), output1)
		assert.Equal(t, expectedStreamData1, f.WrittenData())
		assert.Equal(t, uint64(len(insertEntry1)), f.encoderStreamSentByteCount)

		// Stream 1 is blocked. Stream 2 is not allowed to block.
		headerList2 := []HeaderField{
			{Name: "foo", Value: "bar"},
			{Name: "foo", Value: "baz"},
			{Name: "cookie", Value: "baz"},
			{Name: "bar", Value: "baz"},
		}

		var entries []byte
		if f.HuffmanEnabled() {
			entries = decodeHexOrPanic("0000" +
				"2a94e7" + "03626172" +
				"2a94e7" + "0362617a" +
				"55" + "0362617a" +
				"23626172" + "0362617a")
		} else {
			entries = decodeHexOrPanic("0000" +
				"23666f6f" + "03626172" +
				"23666f6f" + "0362617a" +
				"55" + "0362617a" +
				"23626172" + "0362617a")
		}
		output2 := f.EncodeStream(2, headerList2)
		require.Equal(t, entries, output2)
		assert.Equal(t, uint64(0), f.encoderStreamSentByteCount)

		// Peer acknowledges receipt of one dynamic table entry.
		// Stream 1 is no longer blocked.
		f.encoder.OnInsertCountIncrement(1)
		f.ClearWrites()

		// Stream 3 can now insert entries.
		insertEntries := decodeHexOrPanic("80" + "0362617a" +
			"c5" + "0362617a" +
			"43" + "626172" + "0362617a")
		output3 := f.EncodeStream(3, headerList2)
		require.Equal(t, decodeHexOrPanic("050083828180"), output3)
		assert.Equal(t, insertEntries, f.WrittenData())
		assert.Equal(t, uint64(len(insertEntries)), f.encoderStreamSentByteCount)

		// Stream 3 is blocked. Stream 4 is not allowed to block, but it can reference already acknowledged dynamic entry 0.
		f.ClearWrites()
		var expected2 []byte
		if f.HuffmanEnabled() {
			expected2 = decodeHexOrPanic("0200" +
				"80" +
				"2a94e7" + "0362617a" +
				"55" + "0362617a" +
				"23626172" + "0362617a")
		} else {
			expected2 = decodeHexOrPanic("0200" +
				"80" +
				"23666f6f" + "0362617a" +
				"55" + "0362617a" +
				"23626172" + "0362617a")
		}
		output4 := f.EncodeStream(4, headerList2)
		require.Equal(t, expected2, output4)
		assert.Equal(t, uint64(0), f.encoderStreamSentByteCount)

		// Peer acknowledges receipt of two more dynamic table entries.
		f.encoder.OnInsertCountIncrement(2)

		// Stream 5 is not allowed to block, but it can reference already acknowledged dynamic entries 0, 1, and 2.
		f.ClearWrites()
		expected3 := decodeHexOrPanic("0400" +
			"828180" +
			"23626172" + "0362617a")
		output5 := f.EncodeStream(5, headerList2)
		require.Equal(t, expected3, output5)
		assert.Equal(t, uint64(0), f.encoderStreamSentByteCount)

		// Peer acknowledges decoding header block on stream 3.
		// Stream 3 is not blocked any longer.
		f.encoder.OnHeaderAcknowledgement(3)

		f.ClearWrites()
		expected4 := decodeHexOrPanic("050083828180")
		output6 := f.EncodeStream(6, headerList2)
		require.Equal(t, expected4, output6)
		assert.Equal(t, uint64(0), f.encoderStreamSentByteCount)
	})
}

// TEST_P(EncoderTest, Draining)
func TestEncoder_Draining(t *testing.T) {
	runEncoderTest(t, func(t *testing.T, f *qpackEncoderTestFixture) {
		headerList1 := []HeaderField{
			{Name: "one", Value: "foo"},
			{Name: "two", Value: "foo"},
			{Name: "three", Value: "foo"},
			{Name: "four", Value: "foo"},
			{Name: "five", Value: "foo"},
			{Name: "six", Value: "foo"},
			{Name: "seven", Value: "foo"},
			{Name: "eight", Value: "foo"},
			{Name: "nine", Value: "foo"},
			{Name: "ten", Value: "foo"},
		}

		var maxCapacity uint64
		for _, hf := range headerList1 {
			maxCapacity += EntrySize(hf.Name, hf.Value)
		}
		maxCapacity += EntrySize("one", "foo")

		f.encoder.SetMaximumDynamicTableCapacity(maxCapacity)
		f.encoder.SetDynamicTableCapacity(maxCapacity)

		output1 := f.Encode(headerList1)
		expected1 := decodeHexOrPanic("0b0089888786858483828180")
		require.Equal(t, expected1, output1)

		// Entry is identical to oldest one, which is draining. It will be duplicated and referenced.
		f.ClearWrites()
		headerList2 := []HeaderField{{Name: "one", Value: "foo"}}
		output2 := f.Encode(headerList2)
		expected2 := decodeHexOrPanic("0c0080")
		require.Equal(t, expected2, output2)
		assert.Equal(t, decodeHexOrPanic("09"), f.WrittenData())

		// Entry is identical to second oldest one, which is draining. There is no room to duplicate, it will be encoded with string literals.
		f.ClearWrites()
		headerList3 := []HeaderField{
			{Name: "two", Value: "foo"},
			{Name: "two", Value: "bar"},
		}
		output3 := f.Encode(headerList3)

		entriesHex := "0000" + "2374776f"
		if f.HuffmanEnabled() {
			entriesHex += "8294e7"
		} else {
			entriesHex += "03666f6f"
		}
		entriesHex += "2374776f" + "03626172"
		expected3 := decodeHexOrPanic(entriesHex)
		require.Equal(t, expected3, output3)
	})
}

// TEST_P(EncoderTest, DynamicTableCapacityLessThanMaximum)
func TestEncoder_DynamicTableCapacityLessThanMaximum(t *testing.T) {
	runEncoderTest(t, func(t *testing.T, f *qpackEncoderTestFixture) {
		f.encoder.SetMaximumDynamicTableCapacity(1024)
		f.encoder.SetDynamicTableCapacity(30)

		headerTable := EncoderPeerHeaderTable(f.encoder)
		assert.Equal(t, uint64(1024), headerTable.MaximumDynamicTableCapacity())
		assert.Equal(t, uint64(30), headerTable.DynamicTableCapacity())
	})
}

// TEST_P(EncoderTest, EncoderStreamWritesDisallowedThenAllowed)
func TestEncoder_EncoderStreamWritesDisallowedThenAllowed(t *testing.T) {
	runEncoderTest(t, func(t *testing.T, f *qpackEncoderTestFixture) {
		f.encoderStreamSenderDelegate.numBytesBuffered = kTooManyBytesBuffered
		f.encoder.SetMaximumBlockedStreams(1)
		f.encoder.SetMaximumDynamicTableCapacity(4096)
		f.encoder.SetDynamicTableCapacity(4096)

		headerList1 := []HeaderField{
			{Name: "foo", Value: "bar"},
			{Name: "foo", Value: "baz"},
			{Name: "cookie", Value: "baz"},
		}

		var entries []byte
		if f.HuffmanEnabled() {
			entries = decodeHexOrPanic("0000" +
				"2a94e7" + "03626172" +
				"2a94e7" + "0362617a" +
				"55" + "0362617a")
		} else {
			entries = decodeHexOrPanic("0000" +
				"23666f6f" + "03626172" +
				"23666f6f" + "0362617a" +
				"55" + "0362617a")
		}
		output1 := f.Encode(headerList1)
		require.Equal(t, entries, output1)
		assert.Equal(t, uint64(0), f.encoderStreamSentByteCount)

		// Resuming writes when buffer drains below threshold
		f.encoderStreamSenderDelegate.numBytesBuffered = 0
		f.ClearWrites()

		headerList2 := []HeaderField{
			{Name: "foo", Value: "bar"},
			{Name: "foo", Value: "baz"},
			{Name: "cookie", Value: "baz"},
		}

		setDynamicTableCapacity := decodeHexOrPanic("3fe11f")
		var insertEntriesHex string
		if f.HuffmanEnabled() {
			insertEntriesHex = "62" + "94e7"
		} else {
			insertEntriesHex = "43" + "666f6f"
		}
		insertEntriesHex += "03626172" + "80" + "0362617a" + "c5" + "0362617a"
		insertEntries := decodeHexOrPanic(insertEntriesHex)
		expectedStreamData := append(setDynamicTableCapacity, insertEntries...)

		output2 := f.Encode(headerList2)
		require.Equal(t, decodeHexOrPanic("0400828180"), output2)
		assert.Equal(t, expectedStreamData, f.WrittenData())
		assert.Equal(t, uint64(len(insertEntries)), f.encoderStreamSentByteCount)
	})
}

// TEST_P(EncoderTest, EncoderStreamWritesAllowedThenDisallowed)
func TestEncoder_EncoderStreamWritesAllowedThenDisallowed(t *testing.T) {
	runEncoderTest(t, func(t *testing.T, f *qpackEncoderTestFixture) {
		f.encoder.SetMaximumBlockedStreams(1)
		f.encoder.SetMaximumDynamicTableCapacity(4096)
		f.encoder.SetDynamicTableCapacity(4096)

		headerList1 := []HeaderField{
			{Name: "foo", Value: "bar"},
			{Name: "foo", Value: "baz"},
			{Name: "cookie", Value: "baz"},
		}

		setDynamicTableCapacity := decodeHexOrPanic("3fe11f")
		var insertEntriesHex string
		if f.HuffmanEnabled() {
			insertEntriesHex = "62" + "94e7"
		} else {
			insertEntriesHex = "43" + "666f6f"
		}
		insertEntriesHex += "03626172" + "80" + "0362617a" + "c5" + "0362617a"
		insertEntries := decodeHexOrPanic(insertEntriesHex)
		expectedStreamData := append(setDynamicTableCapacity, insertEntries...)

		output1 := f.Encode(headerList1)
		require.Equal(t, decodeHexOrPanic("0400828180"), output1)
		assert.Equal(t, expectedStreamData, f.WrittenData())
		assert.Equal(t, uint64(len(insertEntries)), f.encoderStreamSentByteCount)

		// Transition to disallowed
		f.encoderStreamSenderDelegate.numBytesBuffered = kTooManyBytesBuffered
		f.ClearWrites()

		headerList2 := []HeaderField{
			{Name: "foo", Value: "bar"},
			{Name: "bar", Value: "baz"},
			{Name: "cookie", Value: "baz"},
		}

		output2 := f.Encode(headerList2)
		expectedOutput2 := decodeHexOrPanic("0400" +
			"82" +
			"23626172" + "0362617a" +
			"80")
		require.Equal(t, expectedOutput2, output2)
		assert.Equal(t, uint64(0), f.encoderStreamSentByteCount)
	})
}

// TEST_P(EncoderTest, UnackedEntryCannotBeEvicted)
func TestEncoder_UnackedEntryCannotBeEvicted(t *testing.T) {
	runEncoderTest(t, func(t *testing.T, f *qpackEncoderTestFixture) {
		f.encoder.SetMaximumBlockedStreams(2)
		f.encoder.SetMaximumDynamicTableCapacity(40)
		f.encoder.SetDynamicTableCapacity(40)

		headerTable := EncoderPeerHeaderTable(f.encoder)
		assert.Equal(t, uint64(0), headerTable.InsertedEntryCount())
		assert.Equal(t, uint64(0), headerTable.DroppedEntryCount())

		headerList1 := []HeaderField{{Name: "foo", Value: "bar"}}

		setDynamicTableCapacity := decodeHexOrPanic("3f09")
		var insertEntries1 []byte
		if f.HuffmanEnabled() {
			insertEntries1 = decodeHexOrPanic("62" + "94e7" + "03626172")
		} else {
			insertEntries1 = decodeHexOrPanic("43" + "666f6f" + "03626172")
		}
		expectedStreamData := append(setDynamicTableCapacity, insertEntries1...)

		output1 := f.EncodeStream(1, headerList1)
		require.Equal(t, decodeHexOrPanic("020080"), output1)
		assert.Equal(t, expectedStreamData, f.WrittenData())
		assert.Equal(t, uint64(len(insertEntries1)), f.encoderStreamSentByteCount)
		assert.Equal(t, uint64(1), headerTable.InsertedEntryCount())
		assert.Equal(t, uint64(0), headerTable.DroppedEntryCount())

		// Stream 1 cancelled. Entry 0 has no references, but is unacknowledged.
		f.encoder.OnStreamCancellation(1)

		headerList2 := []HeaderField{{Name: "bar", Value: "baz"}}
		output2 := f.EncodeStream(2, headerList2)
		require.Equal(t, decodeHexOrPanic("0000236261720362617a"), output2)
		assert.Equal(t, uint64(1), headerTable.InsertedEntryCount())
		assert.Equal(t, uint64(0), headerTable.DroppedEntryCount())
	})
}

// TEST_P(EncoderTest, UseStaticTableNameOnlyMatch)
func TestEncoder_UseStaticTableNameOnlyMatch(t *testing.T) {
	runEncoderTest(t, func(t *testing.T, f *qpackEncoderTestFixture) {
		f.encoder.SetMaximumBlockedStreams(2)
		f.encoder.SetMaximumDynamicTableCapacity(4096)
		f.encoder.SetDynamicTableCapacity(4096)

		headerList := []HeaderField{{Name: ":method", Value: "bar"}}

		setDynamicTableCapacity := decodeHexOrPanic("3fe11f")
		insertEntry1 := decodeHexOrPanic("cf03626172")
		expectedStreamData := append(setDynamicTableCapacity, insertEntry1...)

		output1 := f.EncodeStream(1, headerList)
		require.Equal(t, decodeHexOrPanic("020080"), output1)
		assert.Equal(t, expectedStreamData, f.WrittenData())
		assert.Equal(t, uint64(len(insertEntry1)), f.encoderStreamSentByteCount)

		// Stream 2 uses the same dynamic entry.
		output2 := f.EncodeStream(2, headerList)
		require.Equal(t, decodeHexOrPanic("020080"), output2)
		assert.Equal(t, uint64(0), f.encoderStreamSentByteCount)

		// Streams 1 and 2 are blocked, so stream 3 cannot reference or add dynamic entries.
		// Uses static table name index 15.
		output3 := f.EncodeStream(3, headerList)
		require.Equal(t, decodeHexOrPanic("00005f0003626172"), output3)
	})
}

// TEST_P(EncoderTest, UseDynamicTableNameOnlyMatch)
func TestEncoder_UseDynamicTableNameOnlyMatch(t *testing.T) {
	runEncoderTest(t, func(t *testing.T, f *qpackEncoderTestFixture) {
		headerList1 := []HeaderField{
			{Name: "one", Value: "foo"},
			{Name: "two", Value: "foo"},
			{Name: "three", Value: "foo"},
			{Name: "four", Value: "foo"},
			{Name: "five", Value: "foo"},
			{Name: "six", Value: "foo"},
			{Name: "seven", Value: "foo"},
			{Name: "eight", Value: "foo"},
			{Name: "nine", Value: "foo"},
			{Name: "ten", Value: "foo"},
		}

		var maxCapacity uint64
		for _, hf := range headerList1 {
			maxCapacity += EntrySize(hf.Name, hf.Value)
		}
		maxCapacity += EntrySize("one", "bar")

		f.encoder.SetMaximumDynamicTableCapacity(maxCapacity)
		f.encoder.SetDynamicTableCapacity(maxCapacity)

		output1 := f.Encode(headerList1)
		require.Equal(t, decodeHexOrPanic("0b0089888786858483828180"), output1)

		// Entry has the same name as the first one.
		f.ClearWrites()
		headerList2 := []HeaderField{{Name: "one", Value: "bar"}}
		output2 := f.Encode(headerList2)
		require.Equal(t, decodeHexOrPanic("0c0080"), output2)
		assert.Equal(t, decodeHexOrPanic("8903626172"), f.WrittenData())

		// Entry matches name and value of oldest dynamic table entry, which is draining.
		// Use the name of the most recent dynamic table entry instead.
		f.ClearWrites()
		headerList3 := []HeaderField{{Name: "one", Value: "foo"}}
		output3 := f.Encode(headerList3)

		var expected3 []byte
		if f.HuffmanEnabled() {
			expected3 = decodeHexOrPanic("0c00" + "40" + "8294e7")
		} else {
			expected3 = decodeHexOrPanic("0c00" + "40" + "03666f6f")
		}
		require.Equal(t, expected3, output3)
	})
}

// TEST_P(EncoderTest, CookieCrumblingEnabledNoDynamicTable)
func TestEncoder_CookieCrumblingEnabledNoDynamicTable(t *testing.T) {
	runEncoderTestWithCookie(t, CookieCrumblingEnabled, func(t *testing.T, f *qpackEncoderTestFixture) {
		headerList := []HeaderField{{Name: "cookie", Value: "foo; bar"}}
		output := f.Encode(headerList)

		var expected []byte
		if f.HuffmanEnabled() {
			expected = decodeHexOrPanic("0000" +
				"55" + "8294e7" +
				"55" + "03626172")
		} else {
			expected = decodeHexOrPanic("0000" +
				"55" + "03666f6f" +
				"55" + "03626172")
		}
		require.Equal(t, expected, output)
		assert.Equal(t, uint64(0), f.encoderStreamSentByteCount)
	})
}

// TEST_P(EncoderTest, CookieCrumblingEnabledDynamicTable)
func TestEncoder_CookieCrumblingEnabledDynamicTable(t *testing.T) {
	runEncoderTestWithCookie(t, CookieCrumblingEnabled, func(t *testing.T, f *qpackEncoderTestFixture) {
		f.encoder.SetMaximumBlockedStreams(1)
		f.encoder.SetMaximumDynamicTableCapacity(4096)
		f.encoder.SetDynamicTableCapacity(4096)

		headerList := []HeaderField{{Name: "cookie", Value: "foo; bar"}}

		setDynamicTableCapacity := decodeHexOrPanic("3fe11f")
		var insertEntries []byte
		if f.HuffmanEnabled() {
			insertEntries = decodeHexOrPanic("c5" + "8294e7" + "c5" + "03626172")
		} else {
			insertEntries = decodeHexOrPanic("c5" + "03666f6f" + "c5" + "03626172")
		}
		expectedStreamData := append(setDynamicTableCapacity, insertEntries...)

		output := f.Encode(headerList)
		require.Equal(t, decodeHexOrPanic("03008180"), output)
		assert.Equal(t, expectedStreamData, f.WrittenData())
		assert.Equal(t, uint64(len(insertEntries)), f.encoderStreamSentByteCount)
	})
}

// TEST_P(EncoderTest, CookieCrumblingDisabledNoDynamicTable)
func TestEncoder_CookieCrumblingDisabledNoDynamicTable(t *testing.T) {
	runEncoderTestWithCookie(t, CookieCrumblingDisabled, func(t *testing.T, f *qpackEncoderTestFixture) {
		headerList := []HeaderField{{Name: "cookie", Value: "foo; bar"}}
		output := f.Encode(headerList)

		var expected []byte
		if f.HuffmanEnabled() {
			expected = decodeHexOrPanic("0000" + "55" + "8694e7fb5231d9")
		} else {
			expected = decodeHexOrPanic("0000" + "55" + "08666f6f3b20626172")
		}
		require.Equal(t, expected, output)
		assert.Equal(t, uint64(0), f.encoderStreamSentByteCount)
	})
}

// TEST_P(EncoderTest, CookieCrumblingDisabledDynamicTable)
func TestEncoder_CookieCrumblingDisabledDynamicTable(t *testing.T) {
	runEncoderTestWithCookie(t, CookieCrumblingDisabled, func(t *testing.T, f *qpackEncoderTestFixture) {
		f.encoder.SetMaximumBlockedStreams(1)
		f.encoder.SetMaximumDynamicTableCapacity(4096)
		f.encoder.SetDynamicTableCapacity(4096)

		headerList := []HeaderField{{Name: "cookie", Value: "foo; bar"}}

		setDynamicTableCapacity := decodeHexOrPanic("3fe11f")
		var insertEntries []byte
		if f.HuffmanEnabled() {
			insertEntries = decodeHexOrPanic("c5" + "8694e7fb5231d9")
		} else {
			insertEntries = decodeHexOrPanic("c5" + "08666f6f3b20626172")
		}
		expectedStreamData := append(setDynamicTableCapacity, insertEntries...)

		output := f.Encode(headerList)
		require.Equal(t, decodeHexOrPanic("020080"), output)
		assert.Equal(t, expectedStreamData, f.WrittenData())
		assert.Equal(t, uint64(len(insertEntries)), f.encoderStreamSentByteCount)
	})
}
