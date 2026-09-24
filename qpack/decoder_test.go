// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package qpack

import (
	"strings"
	"testing"

	"github.com/lemon4ksan/foundation/testing/assert"
	"github.com/lemon4ksan/foundation/testing/require"
)

const (
	headerAcknowledgement          = "\x81"
	defaultMaxDynamicTableCapacity = 1024
	defaultMaxBlockedStreams       = 1

	kHeaderAcknowledgement          = headerAcknowledgement
	kDefaultMaxDynamicTableCapacity = defaultMaxDynamicTableCapacity
	kDefaultMaxBlockedStreams       = defaultMaxBlockedStreams
)

type encoderStreamErrorCall struct {
	ErrorCode    uint64
	ErrorMessage string
}

type mockEncoderStreamErrorDelegate struct {
	calls []encoderStreamErrorCall
}

func newMockEncoderStreamErrorDelegate() *mockEncoderStreamErrorDelegate {
	return &mockEncoderStreamErrorDelegate{
		calls: make([]encoderStreamErrorCall, 0),
	}
}

func (m *mockEncoderStreamErrorDelegate) OnEncoderStreamError(errorCode uint64, errorMessage string) {
	m.calls = append(m.calls, encoderStreamErrorCall{
		ErrorCode:    errorCode,
		ErrorMessage: errorMessage,
	})
}

type qpackDecoderTestFixture struct {
	t                           *testing.T
	fragmentMode                FragmentMode
	qpackDecoder                *Decoder
	encoderStreamErrorDelegate  *mockEncoderStreamErrorDelegate
	decoderStreamSenderDelegate *mockStreamSenderDelegate
	handler                     *mockHeadersHandler
	progressiveDecoder          *ProgressiveDecoder
}

func newDecoderTestFixture(
	t *testing.T,
	mode FragmentMode,
	maxCapacity uint64,
	maxBlocked uint64,
) *qpackDecoderTestFixture {
	encoderError := newMockEncoderStreamErrorDelegate()
	senderDelegate := newMockStreamSenderDelegate()
	handler := newMockHeadersHandler()

	decoder := NewDecoder(maxCapacity, maxBlocked, encoderError.OnEncoderStreamError)
	decoder.SetStreamSenderDelegate(senderDelegate)

	f := &qpackDecoderTestFixture{
		t:                           t,
		fragmentMode:                mode,
		qpackDecoder:                decoder,
		encoderStreamErrorDelegate:  encoderError,
		decoderStreamSenderDelegate: senderDelegate,
		handler:                     handler,
	}

	// Regression hook for https://crbug.com/1025209:
	// ProgressiveDecoder must not crash if destroyed synchronously in error callback.
	handler.onErrorFunc = func(errorCode uint64, errorMessage string) {
		f.progressiveDecoder = nil
	}

	return f
}

func (f *qpackDecoderTestFixture) DecodeEncoderStreamData(data []byte) {
	f.qpackDecoder.EncoderStreamReceiver().Decode(data)
}

func (f *qpackDecoderTestFixture) CreateProgressiveDecoder(streamID uint64) *ProgressiveDecoder {
	return f.qpackDecoder.CreateProgressiveDecoder(streamID, f.handler)
}

func (f *qpackDecoderTestFixture) StartDecoding() {
	f.progressiveDecoder = f.CreateProgressiveDecoder(1)
}

func (f *qpackDecoderTestFixture) DecodeData(data []byte) {
	if f.fragmentMode == FragmentModeSingleChunk {
		if f.progressiveDecoder != nil && len(data) > 0 {
			f.progressiveDecoder.Decode(data)
		}
	} else {
		for len(data) > 0 && f.progressiveDecoder != nil {
			f.progressiveDecoder.Decode(data[:1])
			data = data[1:]
		}
	}
}

func (f *qpackDecoderTestFixture) EndDecoding() {
	if f.progressiveDecoder != nil {
		f.progressiveDecoder.EndHeaderBlock()
	}
}

func (f *qpackDecoderTestFixture) DecodeHeaderBlock(data []byte) {
	f.StartDecoding()
	f.DecodeData(data)
	f.EndDecoding()
}

func (f *qpackDecoderTestFixture) FlushDecoderStream() {
	f.qpackDecoder.FlushDecoderStream()
}

func (f *qpackDecoderTestFixture) WrittenData() []byte {
	var total []byte
	for _, w := range f.decoderStreamSenderDelegate.writes {
		total = append(total, w...)
	}
	return total
}

func (f *qpackDecoderTestFixture) ClearWrittenData() {
	f.decoderStreamSenderDelegate.writes = nil
}

func runDecoderTest(t *testing.T, testFn func(t *testing.T, f *qpackDecoderTestFixture)) {
	t.Run("SingleChunk", func(t *testing.T) {
		f := newDecoderTestFixture(
			t,
			FragmentModeSingleChunk,
			kDefaultMaxDynamicTableCapacity,
			kDefaultMaxBlockedStreams,
		)
		testFn(t, f)
	})
	t.Run("OctetByOctet", func(t *testing.T) {
		f := newDecoderTestFixture(
			t,
			FragmentModeOctetByOctet,
			kDefaultMaxDynamicTableCapacity,
			kDefaultMaxBlockedStreams,
		)
		testFn(t, f)
	})
}

func runDecoderTestWithParams(
	t *testing.T,
	maxCapacity, maxBlocked uint64,
	testFn func(t *testing.T, f *qpackDecoderTestFixture),
) {
	t.Run("SingleChunk", func(t *testing.T) {
		f := newDecoderTestFixture(t, FragmentModeSingleChunk, maxCapacity, maxBlocked)
		testFn(t, f)
	})
	t.Run("OctetByOctet", func(t *testing.T) {
		f := newDecoderTestFixture(t, FragmentModeOctetByOctet, maxCapacity, maxBlocked)
		testFn(t, f)
	})
}

// -----------------------------------------------------------------------------
// Chromium DecoderTest Suite Port (33 tests)
// -----------------------------------------------------------------------------

// TEST_P(DecoderTest, NoPrefix)
func TestDecoder_NoPrefix(t *testing.T) {
	runDecoderTest(t, func(t *testing.T, f *qpackDecoderTestFixture) {
		f.DecodeHeaderBlock(decodeHexOrPanic("00"))
		assert.True(t, f.handler.errorDetected)
		assert.Equal(t, uint64(QPACK_DECOMPRESSION_FAILED), f.handler.errorCode)
		assert.Equal(t, "Incomplete header data prefix.", f.handler.errorMessage)
	})
}

// TEST_P(DecoderTest, InvalidPrefix)
func TestDecoder_InvalidPrefix(t *testing.T) {
	runDecoderTest(t, func(t *testing.T, f *qpackDecoderTestFixture) {
		f.StartDecoding()
		f.DecodeData(decodeHexOrPanic("ffffffffffffffffffffffffffff"))
		assert.True(t, f.handler.errorDetected)
		assert.Equal(t, uint64(QPACK_DECOMPRESSION_FAILED), f.handler.errorCode)
		assert.Equal(t, "Encoded integer too large.", f.handler.errorMessage)
	})
}

// TEST_P(DecoderTest, EmptyHeaderBlock)
func TestDecoder_EmptyHeaderBlock(t *testing.T) {
	runDecoderTest(t, func(t *testing.T, f *qpackDecoderTestFixture) {
		f.DecodeHeaderBlock(decodeHexOrPanic("0000"))
		assert.False(t, f.handler.errorDetected)
		assert.True(t, f.handler.decodingCompleted)
		assert.Equal(t, 0, len(f.handler.headers))
	})
}

// TEST_P(DecoderTest, LiteralEntryEmptyName)
func TestDecoder_LiteralEntryEmptyName(t *testing.T) {
	runDecoderTest(t, func(t *testing.T, f *qpackDecoderTestFixture) {
		f.DecodeHeaderBlock(decodeHexOrPanic("00002003666f6f"))
		assert.False(t, f.handler.errorDetected)
		assert.True(t, f.handler.decodingCompleted)
		require.Equal(t, 1, len(f.handler.headers))
		assert.Equal(t, "", f.handler.headers[0].Name)
		assert.Equal(t, "foo", f.handler.headers[0].Value)
	})
}

// TEST_P(DecoderTest, LiteralEntryEmptyValue)
func TestDecoder_LiteralEntryEmptyValue(t *testing.T) {
	runDecoderTest(t, func(t *testing.T, f *qpackDecoderTestFixture) {
		f.DecodeHeaderBlock(decodeHexOrPanic("000023666f6f00"))
		assert.False(t, f.handler.errorDetected)
		assert.True(t, f.handler.decodingCompleted)
		require.Equal(t, 1, len(f.handler.headers))
		assert.Equal(t, "foo", f.handler.headers[0].Name)
		assert.Equal(t, "", f.handler.headers[0].Value)
	})
}

// TEST_P(DecoderTest, LiteralEntryEmptyNameAndValue)
func TestDecoder_LiteralEntryEmptyNameAndValue(t *testing.T) {
	runDecoderTest(t, func(t *testing.T, f *qpackDecoderTestFixture) {
		f.DecodeHeaderBlock(decodeHexOrPanic("00002000"))
		assert.False(t, f.handler.errorDetected)
		assert.True(t, f.handler.decodingCompleted)
		require.Equal(t, 1, len(f.handler.headers))
		assert.Equal(t, "", f.handler.headers[0].Name)
		assert.Equal(t, "", f.handler.headers[0].Value)
	})
}

// TEST_P(DecoderTest, SimpleLiteralEntry)
func TestDecoder_SimpleLiteralEntry(t *testing.T) {
	runDecoderTest(t, func(t *testing.T, f *qpackDecoderTestFixture) {
		f.DecodeHeaderBlock(decodeHexOrPanic("000023666f6f03626172"))
		assert.False(t, f.handler.errorDetected)
		assert.True(t, f.handler.decodingCompleted)
		require.Equal(t, 1, len(f.handler.headers))
		assert.Equal(t, "foo", f.handler.headers[0].Name)
		assert.Equal(t, "bar", f.handler.headers[0].Value)
	})
}

// TEST_P(DecoderTest, MultipleLiteralEntries)
func TestDecoder_MultipleLiteralEntries(t *testing.T) {
	runDecoderTest(t, func(t *testing.T, f *qpackDecoderTestFixture) {
		payload := "0000" +
			"23666f6f03626172" +
			"2700666f6f62616172" +
			"7f00" + strings.Repeat("61", 127)

		f.DecodeHeaderBlock(decodeHexOrPanic(payload))
		assert.False(t, f.handler.errorDetected)
		assert.True(t, f.handler.decodingCompleted)
		require.Equal(t, 2, len(f.handler.headers))
		assert.Equal(t, "foo", f.handler.headers[0].Name)
		assert.Equal(t, "bar", f.handler.headers[0].Value)
		assert.Equal(t, "foobaar", f.handler.headers[1].Name)
		assert.Equal(t, strings.Repeat("a", 127), f.handler.headers[1].Value)
	})
}

// TEST_P(DecoderTest, NameLenTooLargeForVarintDecoder)
func TestDecoder_NameLenTooLargeForVarintDecoder(t *testing.T) {
	runDecoderTest(t, func(t *testing.T, f *qpackDecoderTestFixture) {
		f.DecodeHeaderBlock(decodeHexOrPanic("000027ffffffffffffffffffff"))
		assert.True(t, f.handler.errorDetected)
		assert.Equal(t, uint64(QPACK_DECOMPRESSION_FAILED), f.handler.errorCode)
		assert.Equal(t, "Encoded integer too large.", f.handler.errorMessage)
	})
}

// TEST_P(DecoderTest, NameLenExceedsLimit)
func TestDecoder_NameLenExceedsLimit(t *testing.T) {
	runDecoderTest(t, func(t *testing.T, f *qpackDecoderTestFixture) {
		f.DecodeHeaderBlock(decodeHexOrPanic("000027ffff7f"))
		assert.True(t, f.handler.errorDetected)
		assert.Equal(t, uint64(QPACK_DECOMPRESSION_FAILED), f.handler.errorCode)
		assert.Equal(t, "String literal too long.", f.handler.errorMessage)
	})
}

// TEST_P(DecoderTest, ValueLenTooLargeForVarintDecoder)
func TestDecoder_ValueLenTooLargeForVarintDecoder(t *testing.T) {
	runDecoderTest(t, func(t *testing.T, f *qpackDecoderTestFixture) {
		f.DecodeHeaderBlock(decodeHexOrPanic("000023666f6f7fffffffffffffffffffff"))
		assert.True(t, f.handler.errorDetected)
		assert.Equal(t, uint64(QPACK_DECOMPRESSION_FAILED), f.handler.errorCode)
		assert.Equal(t, "Encoded integer too large.", f.handler.errorMessage)
	})
}

// TEST_P(DecoderTest, ValueLenExceedsLimit)
func TestDecoder_ValueLenExceedsLimit(t *testing.T) {
	runDecoderTest(t, func(t *testing.T, f *qpackDecoderTestFixture) {
		f.DecodeHeaderBlock(decodeHexOrPanic("000023666f6f7fffff7f"))
		assert.True(t, f.handler.errorDetected)
		assert.Equal(t, uint64(QPACK_DECOMPRESSION_FAILED), f.handler.errorCode)
		assert.Equal(t, "String literal too long.", f.handler.errorMessage)
	})
}

// TEST_P(DecoderTest, LineFeedInValue)
func TestDecoder_LineFeedInValue(t *testing.T) {
	runDecoderTest(t, func(t *testing.T, f *qpackDecoderTestFixture) {
		f.DecodeHeaderBlock(decodeHexOrPanic("000023666f6f0462610a72"))
		assert.False(t, f.handler.errorDetected)
		assert.True(t, f.handler.decodingCompleted)
		require.Equal(t, 1, len(f.handler.headers))
		assert.Equal(t, "foo", f.handler.headers[0].Name)
		assert.Equal(t, "ba\nr", f.handler.headers[0].Value)
	})
}

// TEST_P(DecoderTest, IncompleteHeaderBlock)
func TestDecoder_IncompleteHeaderBlock(t *testing.T) {
	runDecoderTest(t, func(t *testing.T, f *qpackDecoderTestFixture) {
		f.DecodeHeaderBlock(decodeHexOrPanic("00002366"))
		assert.True(t, f.handler.errorDetected)
		assert.Equal(t, uint64(QPACK_DECOMPRESSION_FAILED), f.handler.errorCode)
		assert.Equal(t, "Incomplete header block.", f.handler.errorMessage)
	})
}

// TEST_P(DecoderTest, HuffmanSimple)
func TestDecoder_HuffmanSimple(t *testing.T) {
	runDecoderTest(t, func(t *testing.T, f *qpackDecoderTestFixture) {
		f.DecodeHeaderBlock(decodeHexOrPanic("00002f0125a849e95ba97d7f8925a849e95bb8e8b4bf"))
		assert.False(t, f.handler.errorDetected)
		assert.True(t, f.handler.decodingCompleted)
		require.Equal(t, 1, len(f.handler.headers))
		assert.Equal(t, "custom-key", f.handler.headers[0].Name)
		assert.Equal(t, "custom-value", f.handler.headers[0].Value)
	})
}

// TEST_P(DecoderTest, AlternatingHuffmanNonHuffman)
func TestDecoder_AlternatingHuffmanNonHuffman(t *testing.T) {
	runDecoderTest(t, func(t *testing.T, f *qpackDecoderTestFixture) {
		payload := "0000" +
			"2f0125a849e95ba97d7f" +
			"8925a849e95bb8e8b4bf" +
			"2703637573746f6d2d6b6579" +
			"0c637573746f6d2d76616c7565" +
			"2f0125a849e95ba97d7f" +
			"0c637573746f6d2d76616c7565" +
			"2703637573746f6d2d6b6579" +
			"8925a849e95bb8e8b4bf"

		f.DecodeHeaderBlock(decodeHexOrPanic(payload))
		assert.False(t, f.handler.errorDetected)
		assert.True(t, f.handler.decodingCompleted)
		require.Equal(t, 4, len(f.handler.headers))
		for i := 0; i < 4; i++ {
			assert.Equal(t, "custom-key", f.handler.headers[i].Name)
			assert.Equal(t, "custom-value", f.handler.headers[i].Value)
		}
	})
}

// TEST_P(DecoderTest, HuffmanNameDoesNotHaveEOSPrefix)
func TestDecoder_HuffmanNameDoesNotHaveEOSPrefix(t *testing.T) {
	runDecoderTest(t, func(t *testing.T, f *qpackDecoderTestFixture) {
		f.DecodeHeaderBlock(decodeHexOrPanic("00002f0125a849e95ba97d7e8925a849e95bb8e8b4bf"))
		assert.True(t, f.handler.errorDetected)
		assert.Equal(t, "Error in Huffman-encoded string.", f.handler.errorMessage)
	})
}

// TEST_P(DecoderTest, HuffmanValueDoesNotHaveEOSPrefix)
func TestDecoder_HuffmanValueDoesNotHaveEOSPrefix(t *testing.T) {
	runDecoderTest(t, func(t *testing.T, f *qpackDecoderTestFixture) {
		f.DecodeHeaderBlock(decodeHexOrPanic("00002f0125a849e95ba97d7f8925a849e95bb8e8b4be"))
		assert.True(t, f.handler.errorDetected)
		assert.Equal(t, "Error in Huffman-encoded string.", f.handler.errorMessage)
	})
}

// TEST_P(DecoderTest, HuffmanNameEOSPrefixTooLong)
func TestDecoder_HuffmanNameEOSPrefixTooLong(t *testing.T) {
	runDecoderTest(t, func(t *testing.T, f *qpackDecoderTestFixture) {
		f.DecodeHeaderBlock(decodeHexOrPanic("00002f0225a849e95ba97d7fff8925a849e95bb8e8b4bf"))
		assert.True(t, f.handler.errorDetected)
		assert.Equal(t, "Error in Huffman-encoded string.", f.handler.errorMessage)
	})
}

// TEST_P(DecoderTest, HuffmanValueEOSPrefixTooLong)
func TestDecoder_HuffmanValueEOSPrefixTooLong(t *testing.T) {
	runDecoderTest(t, func(t *testing.T, f *qpackDecoderTestFixture) {
		f.DecodeHeaderBlock(decodeHexOrPanic("00002f0125a849e95ba97d7f8a25a849e95bb8e8b4bfff"))
		assert.True(t, f.handler.errorDetected)
		assert.Equal(t, "Error in Huffman-encoded string.", f.handler.errorMessage)
	})
}

// TEST_P(DecoderTest, StaticTable)
func TestDecoder_StaticTable(t *testing.T) {
	runDecoderTest(t, func(t *testing.T, f *qpackDecoderTestFixture) {
		f.DecodeHeaderBlock(decodeHexOrPanic("0000d1dfccd45f108621e9aec2a11f5c8294e75f000554524143455f1000"))
		assert.False(t, f.handler.errorDetected)
		assert.True(t, f.handler.decodingCompleted)

		require.Equal(t, 8, len(f.handler.headers))
		assert.Equal(t, ":method", f.handler.headers[0].Name)
		assert.Equal(t, "GET", f.handler.headers[0].Value)
		assert.Equal(t, "accept-encoding", f.handler.headers[1].Name)
		assert.Equal(t, "gzip, deflate, br", f.handler.headers[1].Value)
		assert.Equal(t, "location", f.handler.headers[2].Name)
		assert.Equal(t, "", f.handler.headers[2].Value)
		assert.Equal(t, ":method", f.handler.headers[3].Name)
		assert.Equal(t, "POST", f.handler.headers[3].Value)
		assert.Equal(t, "accept-encoding", f.handler.headers[4].Name)
		assert.Equal(t, "compress", f.handler.headers[4].Value)
		assert.Equal(t, "location", f.handler.headers[5].Name)
		assert.Equal(t, "foo", f.handler.headers[5].Value)
		assert.Equal(t, ":method", f.handler.headers[6].Name)
		assert.Equal(t, "TRACE", f.handler.headers[6].Value)
		assert.Equal(t, "accept-encoding", f.handler.headers[7].Name)
		assert.Equal(t, "", f.handler.headers[7].Value)
	})
}

// TEST_P(DecoderTest, TooHighStaticTableIndex)
func TestDecoder_TooHighStaticTableIndex(t *testing.T) {
	runDecoderTest(t, func(t *testing.T, f *qpackDecoderTestFixture) {
		f.DecodeHeaderBlock(decodeHexOrPanic("0000ff23ff24"))
		assert.True(t, f.handler.errorDetected)
		assert.Equal(t, "Static table entry not found.", f.handler.errorMessage)
		require.Equal(t, 1, len(f.handler.headers))
		assert.Equal(t, "x-frame-options", f.handler.headers[0].Name)
		assert.Equal(t, "sameorigin", f.handler.headers[0].Value)
	})
}

// TEST_P(DecoderTest, DynamicTable)
func TestDecoder_DynamicTable(t *testing.T) {
	runDecoderTest(t, func(t *testing.T, f *qpackDecoderTestFixture) {
		// Populate dynamic table:
		// 3fe107: Set dynamic table capacity to 1024
		// 6294e703626172: Insert "foo", "bar"
		// 80035a5a5a: Insert with name of dynamic index 0, value "ZZZ" -> "foo", "ZZZ"
		// cf8294e7: Insert with name of static index 15 (:method), value "foo" -> ":method", "foo"
		// 01: Duplicate relative index 1 ("foo", "ZZZ")
		f.DecodeEncoderStreamData(decodeHexOrPanic("3fe1076294e70362617280035a5a5acf8294e701"))
		assert.Equal(t, 0, len(f.encoderStreamErrorDelegate.calls))

		// Header Block 1: Base = 4 + 0 = 4
		f.DecodeHeaderBlock(decodeHexOrPanic("05008382818041025a5a"))
		assert.False(t, f.handler.errorDetected)
		assert.True(t, f.handler.decodingCompleted)
		require.Equal(t, 5, len(f.handler.headers))
		assert.Equal(t, "foo", f.handler.headers[0].Name)
		assert.Equal(t, "bar", f.handler.headers[0].Value)
		assert.Equal(t, "foo", f.handler.headers[1].Name)
		assert.Equal(t, "ZZZ", f.handler.headers[1].Value)
		assert.Equal(t, ":method", f.handler.headers[2].Name)
		assert.Equal(t, "foo", f.handler.headers[2].Value)
		assert.Equal(t, "foo", f.handler.headers[3].Name)
		assert.Equal(t, "ZZZ", f.handler.headers[3].Value)
		assert.Equal(t, ":method", f.handler.headers[4].Name)
		assert.Equal(t, "ZZ", f.handler.headers[4].Value)

		f.FlushDecoderStream()
		assert.Equal(t, decodeHexOrPanic("81"), f.WrittenData())
		f.ClearWrittenData()

		// Header Block 2: Base = 4 + 2 = 6
		f.handler.Reset()
		f.DecodeHeaderBlock(decodeHexOrPanic("05028584838243025a5a"))
		assert.False(t, f.handler.errorDetected)
		assert.True(t, f.handler.decodingCompleted)
		require.Equal(t, 5, len(f.handler.headers))
		assert.Equal(t, "foo", f.handler.headers[0].Name)
		assert.Equal(t, "bar", f.handler.headers[0].Value)
		assert.Equal(t, "foo", f.handler.headers[1].Name)
		assert.Equal(t, "ZZZ", f.handler.headers[1].Value)
		assert.Equal(t, ":method", f.handler.headers[2].Name)
		assert.Equal(t, "foo", f.handler.headers[2].Value)
		assert.Equal(t, "foo", f.handler.headers[3].Name)
		assert.Equal(t, "ZZZ", f.handler.headers[3].Value)
		assert.Equal(t, ":method", f.handler.headers[4].Name)
		assert.Equal(t, "ZZ", f.handler.headers[4].Value)

		f.FlushDecoderStream()
		assert.Equal(t, decodeHexOrPanic("81"), f.WrittenData())
		f.ClearWrittenData()

		// Header Block 3: Base = 4 - 2 - 1 = 1
		f.handler.Reset()
		f.DecodeHeaderBlock(decodeHexOrPanic("05828010111201025a5a"))
		assert.False(t, f.handler.errorDetected)
		assert.True(t, f.handler.decodingCompleted)
		require.Equal(t, 5, len(f.handler.headers))
		assert.Equal(t, "foo", f.handler.headers[0].Name)
		assert.Equal(t, "bar", f.handler.headers[0].Value)
		assert.Equal(t, "foo", f.handler.headers[1].Name)
		assert.Equal(t, "ZZZ", f.handler.headers[1].Value)
		assert.Equal(t, ":method", f.handler.headers[2].Name)
		assert.Equal(t, "foo", f.handler.headers[2].Value)
		assert.Equal(t, "foo", f.handler.headers[3].Name)
		assert.Equal(t, "ZZZ", f.handler.headers[3].Value)
		assert.Equal(t, ":method", f.handler.headers[4].Name)
		assert.Equal(t, "ZZ", f.handler.headers[4].Value)

		f.FlushDecoderStream()
		assert.Equal(t, decodeHexOrPanic("81"), f.WrittenData())
	})
}

// TEST_P(DecoderTest, DecreasingDynamicTableCapacityEvictsEntries)
func TestDecoder_DecreasingDynamicTableCapacityEvictsEntries(t *testing.T) {
	runDecoderTest(t, func(t *testing.T, f *qpackDecoderTestFixture) {
		f.DecodeEncoderStreamData(decodeHexOrPanic("3fe107"))
		f.DecodeEncoderStreamData(decodeHexOrPanic("6294e703626172"))

		f.DecodeHeaderBlock(decodeHexOrPanic("020080"))
		assert.False(t, f.handler.errorDetected)
		require.Equal(t, 1, len(f.handler.headers))
		assert.Equal(t, "foo", f.handler.headers[0].Name)
		assert.Equal(t, "bar", f.handler.headers[0].Value)

		f.FlushDecoderStream()
		assert.Equal(t, decodeHexOrPanic("81"), f.WrittenData())
		f.ClearWrittenData()

		// Change capacity to 32 bytes -> evicts entry
		f.DecodeEncoderStreamData(decodeHexOrPanic("3f01"))

		f.handler.Reset()
		f.DecodeHeaderBlock(decodeHexOrPanic("020080"))
		assert.True(t, f.handler.errorDetected)
		assert.Equal(t, "Dynamic table entry already evicted.", f.handler.errorMessage)
	})
}

// TEST_P(DecoderTest, EncoderStreamErrorEntryTooLarge)
func TestDecoder_EncoderStreamErrorEntryTooLarge(t *testing.T) {
	runDecoderTest(t, func(t *testing.T, f *qpackDecoderTestFixture) {
		// Set capacity 34
		f.DecodeEncoderStreamData(decodeHexOrPanic("3f03"))
		// Add entry of size 38
		f.DecodeEncoderStreamData(decodeHexOrPanic("6294e703626172"))

		require.Equal(t, 1, len(f.encoderStreamErrorDelegate.calls))
		assert.Equal(
			t,
			QUIC_QPACK_ENCODER_STREAM_ERROR_INSERTING_LITERAL,
			f.encoderStreamErrorDelegate.calls[0].ErrorCode,
		)
		assert.Equal(t, "Error inserting literal entry.", f.encoderStreamErrorDelegate.calls[0].ErrorMessage)
	})
}

// TEST_P(DecoderTest, EncoderStreamErrorInvalidStaticTableEntry)
func TestDecoder_EncoderStreamErrorInvalidStaticTableEntry(t *testing.T) {
	runDecoderTest(t, func(t *testing.T, f *qpackDecoderTestFixture) {
		// Address static index 99
		f.DecodeEncoderStreamData(decodeHexOrPanic("ff2400"))

		require.Equal(t, 1, len(f.encoderStreamErrorDelegate.calls))
		assert.Equal(t, QUIC_QPACK_ENCODER_STREAM_INVALID_STATIC_ENTRY, f.encoderStreamErrorDelegate.calls[0].ErrorCode)
		assert.Equal(t, "Invalid static table entry.", f.encoderStreamErrorDelegate.calls[0].ErrorMessage)
	})
}

// TEST_P(DecoderTest, EncoderStreamErrorInvalidDynamicTableEntry)
func TestDecoder_EncoderStreamErrorInvalidDynamicTableEntry(t *testing.T) {
	runDecoderTest(t, func(t *testing.T, f *qpackDecoderTestFixture) {
		f.DecodeEncoderStreamData(decodeHexOrPanic("3fe1076294e7036261728100"))

		require.Equal(t, 1, len(f.encoderStreamErrorDelegate.calls))
		assert.Equal(
			t,
			QUIC_QPACK_ENCODER_STREAM_INSERTION_INVALID_RELATIVE_INDEX,
			f.encoderStreamErrorDelegate.calls[0].ErrorCode,
		)
		assert.Equal(t, "Invalid relative index.", f.encoderStreamErrorDelegate.calls[0].ErrorMessage)
	})
}

// TEST_P(DecoderTest, EncoderStreamErrorDuplicateInvalidEntry)
func TestDecoder_EncoderStreamErrorDuplicateInvalidEntry(t *testing.T) {
	runDecoderTest(t, func(t *testing.T, f *qpackDecoderTestFixture) {
		f.DecodeEncoderStreamData(decodeHexOrPanic("3fe1076294e70362617201"))

		require.Equal(t, 1, len(f.encoderStreamErrorDelegate.calls))
		assert.Equal(
			t,
			QUIC_QPACK_ENCODER_STREAM_DUPLICATE_INVALID_RELATIVE_INDEX,
			f.encoderStreamErrorDelegate.calls[0].ErrorCode,
		)
		assert.Equal(t, "Invalid relative index.", f.encoderStreamErrorDelegate.calls[0].ErrorMessage)
	})
}

// TEST_P(DecoderTest, EncoderStreamErrorTooLargeInteger)
func TestDecoder_EncoderStreamErrorTooLargeInteger(t *testing.T) {
	runDecoderTest(t, func(t *testing.T, f *qpackDecoderTestFixture) {
		f.DecodeEncoderStreamData(decodeHexOrPanic("3fffffffffffffffffffff"))

		require.Equal(t, 1, len(f.encoderStreamErrorDelegate.calls))
		assert.Equal(t, QUIC_QPACK_ENCODER_STREAM_INTEGER_TOO_LARGE, f.encoderStreamErrorDelegate.calls[0].ErrorCode)
		assert.Equal(t, "Encoded integer too large.", f.encoderStreamErrorDelegate.calls[0].ErrorMessage)
	})
}

// TEST_P(DecoderTest, InvalidDynamicEntryWhenBaseIsZero)
func TestDecoder_InvalidDynamicEntryWhenBaseIsZero(t *testing.T) {
	runDecoderTest(t, func(t *testing.T, f *qpackDecoderTestFixture) {
		f.DecodeEncoderStreamData(decodeHexOrPanic("3fe1076294e703626172"))

		f.DecodeHeaderBlock(decodeHexOrPanic("028080"))
		assert.True(t, f.handler.errorDetected)
		assert.Equal(t, "Invalid relative index.", f.handler.errorMessage)
	})
}

// TEST_P(DecoderTest, InvalidNegativeBase)
func TestDecoder_InvalidNegativeBase(t *testing.T) {
	runDecoderTest(t, func(t *testing.T, f *qpackDecoderTestFixture) {
		f.DecodeHeaderBlock(decodeHexOrPanic("0281"))
		assert.True(t, f.handler.errorDetected)
		assert.Equal(t, "Error calculating Base.", f.handler.errorMessage)
	})
}

// TEST_P(DecoderTest, InvalidDynamicEntryByRelativeIndex)
func TestDecoder_InvalidDynamicEntryByRelativeIndex(t *testing.T) {
	runDecoderTest(t, func(t *testing.T, f *qpackDecoderTestFixture) {
		f.DecodeEncoderStreamData(decodeHexOrPanic("3fe1076294e703626172"))

		f.DecodeHeaderBlock(decodeHexOrPanic("020081"))
		assert.True(t, f.handler.errorDetected)
		assert.Equal(t, "Invalid relative index.", f.handler.errorMessage)

		f.handler.Reset()
		f.DecodeHeaderBlock(decodeHexOrPanic("02004100"))
		assert.True(t, f.handler.errorDetected)
		assert.Equal(t, "Invalid relative index.", f.handler.errorMessage)
	})
}

// TEST_P(DecoderTest, EvictedDynamicTableEntry)
func TestDecoder_EvictedDynamicTableEntry(t *testing.T) {
	runDecoderTest(t, func(t *testing.T, f *qpackDecoderTestFixture) {
		// Set capacity 128, insert 1 entry, duplicate 4 times -> first 2 instances evicted
		f.DecodeEncoderStreamData(decodeHexOrPanic("3f61"))
		f.DecodeEncoderStreamData(decodeHexOrPanic("6294e703626172"))
		f.DecodeEncoderStreamData(decodeHexOrPanic("00000000"))

		// 1. Indexed dynamic relative index 2
		f.DecodeHeaderBlock(decodeHexOrPanic("050082"))
		assert.True(t, f.handler.errorDetected)
		assert.Equal(t, "Dynamic table entry already evicted.", f.handler.errorMessage)

		// 2. Literal name ref relative index 2
		f.handler.Reset()
		f.DecodeHeaderBlock(decodeHexOrPanic("05004200"))
		assert.True(t, f.handler.errorDetected)
		assert.Equal(t, "Dynamic table entry already evicted.", f.handler.errorMessage)

		// 3. Indexed post-base 0 (Base = 2 - 0 - 1 = 1 -> abs index 1)
		f.handler.Reset()
		f.DecodeHeaderBlock(decodeHexOrPanic("038010"))
		assert.True(t, f.handler.errorDetected)
		assert.Equal(t, "Dynamic table entry already evicted.", f.handler.errorMessage)

		// 4. Literal post-base name ref 0
		f.handler.Reset()
		f.DecodeHeaderBlock(decodeHexOrPanic("03800000"))
		assert.True(t, f.handler.errorDetected)
		assert.Equal(t, "Dynamic table entry already evicted.", f.handler.errorMessage)
	})
}

// TEST_P(DecoderTest, TableCapacityMustNotExceedMaximum)
func TestDecoder_TableCapacityMustNotExceedMaximum(t *testing.T) {
	runDecoderTest(t, func(t *testing.T, f *qpackDecoderTestFixture) {
		// Update to 2048, which exceeds 1024
		f.DecodeEncoderStreamData(decodeHexOrPanic("3fe10f"))
		require.Equal(t, 1, len(f.encoderStreamErrorDelegate.calls))
		assert.Equal(
			t,
			QUIC_QPACK_ENCODER_STREAM_SET_DYNAMIC_TABLE_CAPACITY,
			f.encoderStreamErrorDelegate.calls[0].ErrorCode,
		)
		assert.Equal(t, "Error updating dynamic table capacity.", f.encoderStreamErrorDelegate.calls[0].ErrorMessage)
	})
}

// TEST_P(DecoderTest, SetDynamicTableCapacity)
func TestDecoder_SetDynamicTableCapacity(t *testing.T) {
	runDecoderTest(t, func(t *testing.T, f *qpackDecoderTestFixture) {
		f.DecodeEncoderStreamData(decodeHexOrPanic("3f61"))
		assert.Equal(t, 0, len(f.encoderStreamErrorDelegate.calls))
		assert.Equal(t, uint64(128), f.qpackDecoder.DynamicTableCapacity())
	})
}

// TEST_P(DecoderTest, InvalidEncodedRequiredInsertCount)
func TestDecoder_InvalidEncodedRequiredInsertCount(t *testing.T) {
	runDecoderTest(t, func(t *testing.T, f *qpackDecoderTestFixture) {
		f.DecodeHeaderBlock(decodeHexOrPanic("4100"))
		assert.True(t, f.handler.errorDetected)
		assert.Equal(t, "Error decoding Required Insert Count.", f.handler.errorMessage)
	})
}

// TEST_P(DecoderTest, DataAfterInvalidEncodedRequiredInsertCount)
func TestDecoder_DataAfterInvalidEncodedRequiredInsertCount(t *testing.T) {
	runDecoderTest(t, func(t *testing.T, f *qpackDecoderTestFixture) {
		f.DecodeHeaderBlock(decodeHexOrPanic("410000"))
		assert.True(t, f.handler.errorDetected)
		assert.Equal(t, "Error decoding Required Insert Count.", f.handler.errorMessage)
	})
}

// TEST_P(DecoderTest, WrappedRequiredInsertCount)
func TestDecoder_WrappedRequiredInsertCount(t *testing.T) {
	runDecoderTest(t, func(t *testing.T, f *qpackDecoderTestFixture) {
		f.DecodeEncoderStreamData(decodeHexOrPanic("3fe107"))
		f.DecodeEncoderStreamData(decodeHexOrPanic("6294e77fd903"))
		f.DecodeEncoderStreamData([]byte(strings.Repeat("Z", 600)))
		f.DecodeEncoderStreamData([]byte(strings.Repeat("\x00", 200)))

		f.DecodeHeaderBlock(decodeHexOrPanic("0a0080"))
		assert.False(t, f.handler.errorDetected)
		assert.True(t, f.handler.decodingCompleted)
		require.Equal(t, 1, len(f.handler.headers))
		assert.Equal(t, "foo", f.handler.headers[0].Name)
		assert.Equal(t, strings.Repeat("Z", 600), f.handler.headers[0].Value)

		f.FlushDecoderStream()
		assert.Equal(t, decodeHexOrPanic("81"), f.WrittenData())
	})
}

// TEST_P(DecoderTest, NonZeroRequiredInsertCountButNoDynamicEntries)
func TestDecoder_NonZeroRequiredInsertCountButNoDynamicEntries(t *testing.T) {
	runDecoderTest(t, func(t *testing.T, f *qpackDecoderTestFixture) {
		f.DecodeEncoderStreamData(decodeHexOrPanic("3fe1076294e703626172"))

		f.DecodeHeaderBlock(decodeHexOrPanic("0200d1"))
		assert.True(t, f.handler.errorDetected)
		assert.Equal(t, "Required Insert Count too large.", f.handler.errorMessage)
		require.Equal(t, 1, len(f.handler.headers))
		assert.Equal(t, ":method", f.handler.headers[0].Name)
		assert.Equal(t, "GET", f.handler.headers[0].Value)
	})
}

// TEST_P(DecoderTest, AddressEntryNotAllowedByRequiredInsertCount)
func TestDecoder_AddressEntryNotAllowedByRequiredInsertCount(t *testing.T) {
	runDecoderTest(t, func(t *testing.T, f *qpackDecoderTestFixture) {
		f.DecodeEncoderStreamData(decodeHexOrPanic("3fe1076294e703626172"))

		// 1. Indexed Header Field relative 0, Base 2 -> abs index 1 >= RIC 1
		f.DecodeHeaderBlock(decodeHexOrPanic("020180"))
		assert.True(t, f.handler.errorDetected)
		assert.Equal(t, "Absolute Index must be smaller than Required Insert Count.", f.handler.errorMessage)

		// 2. Literal Header Field with Name Ref relative 0, Base 2 -> abs index 1 >= RIC 1
		f.handler.Reset()
		f.DecodeHeaderBlock(decodeHexOrPanic("02014000"))
		assert.True(t, f.handler.errorDetected)
		assert.Equal(t, "Absolute Index must be smaller than Required Insert Count.", f.handler.errorMessage)

		// 3. Indexed Header Field with Post-Base 0, Base 1 -> abs index 1 >= RIC 1
		f.handler.Reset()
		f.DecodeHeaderBlock(decodeHexOrPanic("020010"))
		assert.True(t, f.handler.errorDetected)
		assert.Equal(t, "Absolute Index must be smaller than Required Insert Count.", f.handler.errorMessage)

		// 4. Literal Header Field with Post-Base Name Ref 0, Base 1 -> abs index 1 >= RIC 1
		f.handler.Reset()
		f.DecodeHeaderBlock(decodeHexOrPanic("02000000"))
		assert.True(t, f.handler.errorDetected)
		assert.Equal(t, "Absolute Index must be smaller than Required Insert Count.", f.handler.errorMessage)
	})
}

// TEST_P(DecoderTest, PromisedRequiredInsertCountLargerThanActual)
func TestDecoder_PromisedRequiredInsertCountLargerThanActual(t *testing.T) {
	runDecoderTest(t, func(t *testing.T, f *qpackDecoderTestFixture) {
		f.DecodeEncoderStreamData(decodeHexOrPanic("3fe1076294e7036261720000"))

		// 1. Indexed relative index 1 -> abs 0. Header requires 1, promised 2.
		f.DecodeHeaderBlock(decodeHexOrPanic("030081"))
		assert.True(t, f.handler.errorDetected)
		assert.Equal(t, "Required Insert Count too large.", f.handler.errorMessage)
		require.Equal(t, 1, len(f.handler.headers))
		assert.Equal(t, "foo", f.handler.headers[0].Name)
		assert.Equal(t, "bar", f.handler.headers[0].Value)

		// 2. Literal with Name Ref relative 1 -> abs 0. Requires 1, promised 2.
		f.handler.Reset()
		f.DecodeHeaderBlock(decodeHexOrPanic("03004100"))
		assert.True(t, f.handler.errorDetected)
		assert.Equal(t, "Required Insert Count too large.", f.handler.errorMessage)
		require.Equal(t, 1, len(f.handler.headers))
		assert.Equal(t, "foo", f.handler.headers[0].Name)
		assert.Equal(t, "", f.handler.headers[0].Value)

		// 3. Post-base index 0, Base 1 -> abs 1. Requires 2, promised 3.
		f.handler.Reset()
		f.DecodeHeaderBlock(decodeHexOrPanic("048110"))
		assert.True(t, f.handler.errorDetected)
		assert.Equal(t, "Required Insert Count too large.", f.handler.errorMessage)
		require.Equal(t, 1, len(f.handler.headers))
		assert.Equal(t, "foo", f.handler.headers[0].Name)
		assert.Equal(t, "bar", f.handler.headers[0].Value)

		// 4. Post-base name ref 0, Base 1 -> abs 1. Requires 2, promised 3.
		f.handler.Reset()
		f.DecodeHeaderBlock(decodeHexOrPanic("04810000"))
		assert.True(t, f.handler.errorDetected)
		assert.Equal(t, "Required Insert Count too large.", f.handler.errorMessage)
		require.Equal(t, 1, len(f.handler.headers))
		assert.Equal(t, "foo", f.handler.headers[0].Name)
		assert.Equal(t, "", f.handler.headers[0].Value)
	})
}

// TEST_P(DecoderTest, BlockedDecoding)
func TestDecoder_BlockedDecoding(t *testing.T) {
	runDecoderTest(t, func(t *testing.T, f *qpackDecoderTestFixture) {
		// Header block arrives first (RIC=1) -> blocks
		f.DecodeHeaderBlock(decodeHexOrPanic("020080"))
		assert.False(t, f.handler.decodingCompleted)
		assert.Equal(t, 0, len(f.handler.headers))

		// Dynamic insert arrives
		f.DecodeEncoderStreamData(decodeHexOrPanic("3fe1076294e703626172"))

		assert.False(t, f.handler.errorDetected)
		assert.True(t, f.handler.decodingCompleted)
		require.Equal(t, 1, len(f.handler.headers))
		assert.Equal(t, "foo", f.handler.headers[0].Name)
		assert.Equal(t, "bar", f.handler.headers[0].Value)

		f.FlushDecoderStream()
		assert.Equal(t, decodeHexOrPanic("81"), f.WrittenData())
	})
}

// TEST_P(DecoderTest, BlockedDecodingUnblockedBeforeEndOfHeaderBlock)
func TestDecoder_BlockedDecodingUnblockedBeforeEndOfHeaderBlock(t *testing.T) {
	runDecoderTest(t, func(t *testing.T, f *qpackDecoderTestFixture) {
		f.StartDecoding()
		f.DecodeData(decodeHexOrPanic("020080d1"))

		// Dynamic table capacity 1024
		f.DecodeEncoderStreamData(decodeHexOrPanic("3fe107"))
		// Add entry "foo: bar" -> unblocks and emits early headers
		f.DecodeEncoderStreamData(decodeHexOrPanic("6294e703626172"))

		require.Equal(t, 2, len(f.handler.headers))
		assert.Equal(t, "foo", f.handler.headers[0].Name)
		assert.Equal(t, "bar", f.handler.headers[0].Value)
		assert.Equal(t, ":method", f.handler.headers[1].Name)
		assert.Equal(t, "GET", f.handler.headers[1].Value)

		// Remaining data decoded in unblocked state
		f.DecodeData(decodeHexOrPanic("80d7"))
		require.Equal(t, 4, len(f.handler.headers))
		assert.Equal(t, "foo", f.handler.headers[2].Name)
		assert.Equal(t, "bar", f.handler.headers[2].Value)
		assert.Equal(t, ":scheme", f.handler.headers[3].Name)
		assert.Equal(t, "https", f.handler.headers[3].Value)

		f.EndDecoding()
		assert.True(t, f.handler.decodingCompleted)

		f.FlushDecoderStream()
		assert.Equal(t, decodeHexOrPanic("81"), f.WrittenData())
	})
}

// TEST_P(DecoderTest, BlockedDecodingUnblockedAndErrorBeforeEndOfHeaderBlock)
func TestDecoder_BlockedDecodingUnblockedAndErrorBeforeEndOfHeaderBlock(t *testing.T) {
	runDecoderTest(t, func(t *testing.T, f *qpackDecoderTestFixture) {
		f.StartDecoding()
		f.DecodeData(decodeHexOrPanic("02008081"))

		f.DecodeEncoderStreamData(decodeHexOrPanic("3fe107"))
		f.DecodeEncoderStreamData(decodeHexOrPanic("6294e703626172"))

		require.Equal(t, 1, len(f.handler.headers))
		assert.Equal(t, "foo", f.handler.headers[0].Name)
		assert.Equal(t, "bar", f.handler.headers[0].Value)

		assert.True(t, f.handler.errorDetected)
		assert.Equal(t, "Invalid relative index.", f.handler.errorMessage)
	})
}

// TEST_P(DecoderTest, BlockedDecodingAndEvictedEntries)
func TestDecoder_BlockedDecodingAndEvictedEntries(t *testing.T) {
	runDecoderTest(t, func(t *testing.T, f *qpackDecoderTestFixture) {
		f.DecodeEncoderStreamData(decodeHexOrPanic("3f61"))

		// Header block with RIC=6
		f.DecodeHeaderBlock(decodeHexOrPanic("070080"))
		assert.False(t, f.handler.decodingCompleted)

		// Add 1 entry, duplicate 4 times -> 5 entries total, first 2 evicted
		f.DecodeEncoderStreamData(decodeHexOrPanic("6294e703626172"))
		f.DecodeEncoderStreamData(decodeHexOrPanic("00000000"))
		assert.False(t, f.handler.decodingCompleted)

		// 6th entry arrives -> InsertCount reaches 6 -> unblocks!
		f.DecodeEncoderStreamData(decodeHexOrPanic("6294e70362617a"))

		assert.False(t, f.handler.errorDetected)
		assert.True(t, f.handler.decodingCompleted)
		require.Equal(t, 1, len(f.handler.headers))
		assert.Equal(t, "foo", f.handler.headers[0].Name)
		assert.Equal(t, "baz", f.handler.headers[0].Value)

		f.FlushDecoderStream()
		assert.Equal(t, decodeHexOrPanic("81"), f.WrittenData())
	})
}

// TEST_P(DecoderTest, TooManyBlockedStreams)
func TestDecoder_TooManyBlockedStreams(t *testing.T) {
	runDecoderTest(t, func(t *testing.T, f *qpackDecoderTestFixture) {
		data := decodeHexOrPanic("0200")

		d1 := f.CreateProgressiveDecoder(1)
		d1.Decode(data)

		d2 := f.CreateProgressiveDecoder(2)
		d2.Decode(data)

		assert.True(t, f.handler.errorDetected)
		assert.Equal(t, uint64(QPACK_DECOMPRESSION_FAILED), f.handler.errorCode)
		assert.Equal(t, "Limit on number of blocked streams exceeded.", f.handler.errorMessage)
	})
}

// TEST_P(DecoderTest, InsertCountIncrement)
func TestDecoder_InsertCountIncrement(t *testing.T) {
	runDecoderTest(t, func(t *testing.T, f *qpackDecoderTestFixture) {
		f.DecodeEncoderStreamData(decodeHexOrPanic("3fe1076294e70362617200"))

		f.DecodeHeaderBlock(decodeHexOrPanic("020080"))
		assert.False(t, f.handler.errorDetected)
		assert.True(t, f.handler.decodingCompleted)
		require.Equal(t, 1, len(f.handler.headers))
		assert.Equal(t, "foo", f.handler.headers[0].Name)
		assert.Equal(t, "bar", f.handler.headers[0].Value)

		f.FlushDecoderStream()
		// Section Ack on stream 1 ("\x81") + Insert Count Increment 1 ("\x01")
		assert.Equal(t, decodeHexOrPanic("8101"), f.WrittenData())
	})
}

// Stream Reset Parity Tests
func TestDecoder_StreamReset_EmitsStreamCancellation(t *testing.T) {
	encoderError := newMockEncoderStreamErrorDelegate()
	sender := newMockStreamSenderDelegate()

	decoder := NewDecoder(1024, 1, encoderError.OnEncoderStreamError)
	decoder.SetStreamSenderDelegate(sender)

	decoder.OnStreamReset(1)
	decoder.FlushDecoderStream()

	// Stream cancellation for stream 1: opcode 0x40 | 1 = 0x41
	var written []byte
	for _, w := range sender.writes {
		written = append(written, w...)
	}
	assert.Equal(t, decodeHexOrPanic("41"), written)
}

func TestDecoder_StreamReset_ZeroMaxCapacity_NoCancellation(t *testing.T) {
	encoderError := newMockEncoderStreamErrorDelegate()
	sender := newMockStreamSenderDelegate()

	decoder := NewDecoder(0, 1, encoderError.OnEncoderStreamError)
	decoder.SetStreamSenderDelegate(sender)

	decoder.OnStreamReset(1)
	decoder.FlushDecoderStream()

	assert.Equal(t, 0, len(sender.writes))
}
