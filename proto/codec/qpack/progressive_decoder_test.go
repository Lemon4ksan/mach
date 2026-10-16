// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package qpack

import (
	"math"
	"testing"

	"github.com/lemon4ksan/foundation/testing/assert"
	"github.com/lemon4ksan/foundation/testing/require"
)

type headerCall struct {
	Name  string
	Value string
}

type mockHeadersHandler struct {
	headers              []headerCall
	decodingCompleted    bool
	decodingCompletedCnt int
	errorDetected        bool
	errorCode            uint64
	errorMessage         string

	onErrorFunc func(errorCode uint64, errorMessage string)
}

func newMockHeadersHandler() *mockHeadersHandler {
	return &mockHeadersHandler{
		headers: make([]headerCall, 0),
	}
}

func (h *mockHeadersHandler) OnHeaderDecoded(name, value string) {
	h.headers = append(h.headers, headerCall{Name: name, Value: value})
}

func (h *mockHeadersHandler) OnDecodingCompleted() {
	h.decodingCompleted = true
	h.decodingCompletedCnt++
}

func (h *mockHeadersHandler) OnDecodingErrorDetected(errorCode uint64, errorMessage string) {
	h.errorDetected = true
	h.errorCode = errorCode
	h.errorMessage = errorMessage
	if h.onErrorFunc != nil {
		h.onErrorFunc(errorCode, errorMessage)
	}
}

func (h *mockHeadersHandler) Reset() {
	h.headers = h.headers[:0]
	h.decodingCompleted = false
	h.decodingCompletedCnt = 0
	h.errorDetected = false
	h.errorCode = 0
	h.errorMessage = ""
}

type mockBlockedStreamLimitEnforcer struct {
	maxBlocked     uint64
	blockedStreams map[uint64]bool
}

func newMockBlockedStreamLimitEnforcer(maxBlocked uint64) *mockBlockedStreamLimitEnforcer {
	return &mockBlockedStreamLimitEnforcer{
		maxBlocked:     maxBlocked,
		blockedStreams: make(map[uint64]bool),
	}
}

func (e *mockBlockedStreamLimitEnforcer) OnStreamBlocked(streamID uint64) bool {
	e.blockedStreams[streamID] = true
	return uint64(len(e.blockedStreams)) <= e.maxBlocked
}

func (e *mockBlockedStreamLimitEnforcer) OnStreamUnblocked(streamID uint64) {
	delete(e.blockedStreams, streamID)
}

type completedCall struct {
	StreamID            uint64
	RequiredInsertCount uint64
}

type mockDecodingCompletedVisitor struct {
	completedCalls []completedCall
}

func newMockDecodingCompletedVisitor() *mockDecodingCompletedVisitor {
	return &mockDecodingCompletedVisitor{
		completedCalls: make([]completedCall, 0),
	}
}

func (v *mockDecodingCompletedVisitor) OnDecodingCompleted(streamID, requiredInsertCount uint64) {
	v.completedCalls = append(v.completedCalls, completedCall{
		StreamID:            streamID,
		RequiredInsertCount: requiredInsertCount,
	})
}

type progressiveDecoderTestFixture struct {
	t            *testing.T
	fragmentMode FragmentMode
	headerTable  *DecoderHeaderTable
	enforcer     *mockBlockedStreamLimitEnforcer
	visitor      *mockDecodingCompletedVisitor
	handler      *mockHeadersHandler
	decoder      *ProgressiveDecoder
}

func newProgressiveDecoderTestFixture(
	t *testing.T,
	mode FragmentMode,
	maxCapacity uint64,
	maxBlocked uint64,
	maxBufferedData uint64,
) *progressiveDecoderTestFixture {
	table := NewDecoderHeaderTable()
	table.SetMaximumDynamicTableCapacity(maxCapacity)

	enforcer := newMockBlockedStreamLimitEnforcer(maxBlocked)
	visitor := newMockDecodingCompletedVisitor()
	handler := newMockHeadersHandler()

	decoder := NewProgressiveDecoder(
		1,
		maxBufferedData,
		enforcer,
		visitor,
		table,
		handler,
	)

	return &progressiveDecoderTestFixture{
		t:            t,
		fragmentMode: mode,
		headerTable:  table,
		enforcer:     enforcer,
		visitor:      visitor,
		handler:      handler,
		decoder:      decoder,
	}
}

func (f *progressiveDecoderTestFixture) DecodeData(data []byte) {
	if f.fragmentMode == FragmentModeSingleChunk {
		if f.decoder != nil && len(data) > 0 {
			f.decoder.Decode(data)
		}
	} else {
		for len(data) > 0 && f.decoder != nil {
			f.decoder.Decode(data[:1])
			data = data[1:]
		}
	}
}

func (f *progressiveDecoderTestFixture) EndDecoding() {
	if f.decoder != nil {
		f.decoder.EndHeaderBlock()
	}
}

func (f *progressiveDecoderTestFixture) DecodeHeaderBlock(data []byte) {
	f.DecodeData(data)
	f.EndDecoding()
}

func runProgressiveDecoderTests(
	t *testing.T,
	maxCapacity, maxBlocked, maxBufferedData uint64,
	testFn func(t *testing.T, f *progressiveDecoderTestFixture),
) {
	t.Run("SingleChunk", func(t *testing.T) {
		f := newProgressiveDecoderTestFixture(t, FragmentModeSingleChunk, maxCapacity, maxBlocked, maxBufferedData)
		testFn(t, f)
	})
	t.Run("OctetByOctet", func(t *testing.T) {
		f := newProgressiveDecoderTestFixture(t, FragmentModeOctetByOctet, maxCapacity, maxBlocked, maxBufferedData)
		testFn(t, f)
	})
}

// -----------------------------------------------------------------------------
// Prefix Tests
// -----------------------------------------------------------------------------

func TestProgressiveDecoder_PrefixDecoding(t *testing.T) {
	runProgressiveDecoderTests(t, 1024, 1, 0, func(t *testing.T, f *progressiveDecoderTestFixture) {
		// Populate dynamic table with 4 entries so RIC=4 does not block
		f.headerTable.SetDynamicTableCapacity(1024)
		f.headerTable.InsertEntry("foo", "bar")
		f.headerTable.InsertEntry("foo", "ZZZ")
		f.headerTable.InsertEntry(":method", "foo")
		f.headerTable.InsertEntry("foo", "ZZZ")

		// 1. RIC = 0, DeltaBase = 0 -> Base = 0
		f.DecodeHeaderBlock(decodeHexOrPanic("0000"))
		assert.False(t, f.handler.errorDetected)
		assert.True(t, f.handler.decodingCompleted)
		assert.Equal(t, uint64(0), f.decoder.requiredInsertCount)
		assert.Equal(t, uint64(0), f.decoder.base)

		// 2. RIC = 4 (encoded 5), DeltaBase = 0 (S=0) -> Base = 4
		f.handler.Reset()
		f.decoder = NewProgressiveDecoder(2, 0, f.enforcer, f.visitor, f.headerTable, f.handler)
		// Empty field lines, but promises RIC=4 -> requires dynamic entries up to 4
		// If no dynamic entries referenced, finishDecoding checks RIC == RICSoFar.
		// So let's include reference to relative index 0 (abs index 3 -> RICSoFar = 4).
		f.DecodeHeaderBlock(decodeHexOrPanic("050080"))
		assert.False(t, f.handler.errorDetected)
		assert.True(t, f.handler.decodingCompleted)
		assert.Equal(t, uint64(4), f.decoder.requiredInsertCount)
		assert.Equal(t, uint64(4), f.decoder.base)

		// 3. RIC = 4 (encoded 5), DeltaBase = 2 (S=0) -> Base = 4 + 2 = 6
		f.handler.Reset()
		f.decoder = NewProgressiveDecoder(3, 0, f.enforcer, f.visitor, f.headerTable, f.handler)
		// Relative index 2 -> abs index = 6 - 2 - 1 = 3 -> RICSoFar = 4
		f.DecodeHeaderBlock(decodeHexOrPanic("050282"))
		assert.False(t, f.handler.errorDetected)
		assert.True(t, f.handler.decodingCompleted)
		assert.Equal(t, uint64(4), f.decoder.requiredInsertCount)
		assert.Equal(t, uint64(6), f.decoder.base)

		// 4. RIC = 4 (encoded 5), DeltaBase = 2 (S=1, 0x82) -> Base = 4 - 2 - 1 = 1
		f.handler.Reset()
		f.decoder = NewProgressiveDecoder(4, 0, f.enforcer, f.visitor, f.headerTable, f.handler)
		// Post-base index 2 -> abs index = 1 + 2 = 3 -> RICSoFar = 4
		f.DecodeHeaderBlock(decodeHexOrPanic("058212"))
		assert.False(t, f.handler.errorDetected)
		assert.True(t, f.handler.decodingCompleted)
		assert.Equal(t, uint64(4), f.decoder.requiredInsertCount)
		assert.Equal(t, uint64(1), f.decoder.base)
	})
}

func TestProgressiveDecoder_PrefixErrors(t *testing.T) {
	// Truncated prefix: only 1 byte
	runProgressiveDecoderTests(t, 1024, 1, 0, func(t *testing.T, f *progressiveDecoderTestFixture) {
		f.DecodeHeaderBlock(decodeHexOrPanic("00"))
		assert.True(t, f.handler.errorDetected)
		assert.Equal(t, uint64(QPACK_DECOMPRESSION_FAILED), f.handler.errorCode)
		assert.Equal(t, "Incomplete header data prefix.", f.handler.errorMessage)
	})

	// Varint too large in prefix
	runProgressiveDecoderTests(t, 1024, 1, 0, func(t *testing.T, f *progressiveDecoderTestFixture) {
		f.handler.onErrorFunc = func(errorCode uint64, errorMessage string) {
			f.decoder = nil
		}
		f.DecodeData(decodeHexOrPanic("ffffffffffffffffffffffffffff"))
		assert.True(t, f.handler.errorDetected)
		assert.Equal(t, uint64(QPACK_DECOMPRESSION_FAILED), f.handler.errorCode)
		assert.Equal(t, "Encoded integer too large.", f.handler.errorMessage)
	})

	// Delta Base underflow (negative Base)
	runProgressiveDecoderTests(t, 1024, 1, 0, func(t *testing.T, f *progressiveDecoderTestFixture) {
		f.DecodeHeaderBlock(decodeHexOrPanic("0281"))
		assert.True(t, f.handler.errorDetected)
		assert.Equal(t, uint64(QPACK_DECOMPRESSION_FAILED), f.handler.errorCode)
		assert.Equal(t, "Error calculating Base.", f.handler.errorMessage)
	})

	// Delta Base calculation directly tests boundary conditions
	t.Run("DeltaBaseHelperBoundaries", func(t *testing.T) {
		d := NewProgressiveDecoder(1, 0, nil, nil, nil, nil)

		// Sign=1, DeltaBase=MaxUint64 -> underflow
		d.requiredInsertCount = 10
		_, ok := d.deltaBaseToBase(true, math.MaxUint64)
		assert.False(t, ok)

		// Sign=1, RIC < DeltaBase + 1 -> underflow
		d.requiredInsertCount = 5
		_, ok = d.deltaBaseToBase(true, 5)
		assert.False(t, ok)

		// Sign=1, RIC == DeltaBase + 1 -> Base = 0 (allowed)
		base, ok := d.deltaBaseToBase(true, 4)
		assert.True(t, ok)
		assert.Equal(t, uint64(0), base)

		// Sign=0, RIC + DeltaBase overflows MaxUint64
		d.requiredInsertCount = 10
		_, ok = d.deltaBaseToBase(false, math.MaxUint64-5)
		assert.False(t, ok)

		// Sign=0, RIC + DeltaBase == MaxUint64
		base, ok = d.deltaBaseToBase(false, math.MaxUint64-10)
		assert.True(t, ok)
		assert.Equal(t, uint64(math.MaxUint64), base)
	})
}

// -----------------------------------------------------------------------------
// Field Line Tests
// -----------------------------------------------------------------------------

func TestProgressiveDecoder_IndexedFields(t *testing.T) {
	runProgressiveDecoderTests(t, 1024, 1, 0, func(t *testing.T, f *progressiveDecoderTestFixture) {
		f.headerTable.SetDynamicTableCapacity(1024)
		f.headerTable.InsertEntry("custom-k", "custom-v")

		// 1. Static table indexed field: index 17 (:method: GET) -> opcode 0x80 | 0x40 | 17 = 0xd1
		f.DecodeHeaderBlock(decodeHexOrPanic("0000d1"))
		assert.False(t, f.handler.errorDetected)
		assert.True(t, f.handler.decodingCompleted)
		require.Equal(t, 1, len(f.handler.headers))
		assert.Equal(t, ":method", f.handler.headers[0].Name)
		assert.Equal(t, "GET", f.handler.headers[0].Value)

		// 2. Dynamic table indexed field: relative index 0 -> opcode 0x80 | 0 = 0x80
		f.handler.Reset()
		f.decoder = NewProgressiveDecoder(2, 0, f.enforcer, f.visitor, f.headerTable, f.handler)
		f.DecodeHeaderBlock(decodeHexOrPanic("020080"))
		assert.False(t, f.handler.errorDetected)
		assert.True(t, f.handler.decodingCompleted)
		require.Equal(t, 1, len(f.handler.headers))
		assert.Equal(t, "custom-k", f.handler.headers[0].Name)
		assert.Equal(t, "custom-v", f.handler.headers[0].Value)

		// 3. Dynamic table indexed post-base: Base 0, post-base index 0 -> opcode 0x10 | 0 = 0x10
		f.handler.Reset()
		f.decoder = NewProgressiveDecoder(3, 0, f.enforcer, f.visitor, f.headerTable, f.handler)
		// RIC=1, DeltaBase=0 with sign bit set (0x80) -> Base = 1 - 0 - 1 = 0
		// Post-base 0 -> abs index 0
		f.DecodeHeaderBlock(decodeHexOrPanic("028010"))
		assert.False(t, f.handler.errorDetected)
		assert.True(t, f.handler.decodingCompleted)
		require.Equal(t, 1, len(f.handler.headers))
		assert.Equal(t, "custom-k", f.handler.headers[0].Name)
		assert.Equal(t, "custom-v", f.handler.headers[0].Value)
	})
}

func TestProgressiveDecoder_LiteralFields(t *testing.T) {
	runProgressiveDecoderTests(t, 1024, 1, 0, func(t *testing.T, f *progressiveDecoderTestFixture) {
		f.headerTable.SetDynamicTableCapacity(1024)
		f.headerTable.InsertEntry("dyn-name", "dyn-val")

		// 1. Literal with static name reference: name from static index 0 (:authority), value "example.com"
		// 0101 T(1) NNNN(0) = 0x50, value: length 11 ("example.com")
		f.DecodeHeaderBlock(decodeHexOrPanic("0000500b6578616d706c652e636f6d"))
		assert.False(t, f.handler.errorDetected)
		assert.True(t, f.handler.decodingCompleted)
		require.Equal(t, 1, len(f.handler.headers))
		assert.Equal(t, ":authority", f.handler.headers[0].Name)
		assert.Equal(t, "example.com", f.handler.headers[0].Value)

		// 2. Literal with dynamic name reference: relative index 0, value "my-val"
		f.handler.Reset()
		f.decoder = NewProgressiveDecoder(2, 0, f.enforcer, f.visitor, f.headerTable, f.handler)
		// 0100 T(0) NNNN(0) = 0x40, value: length 6 ("my-val")
		f.DecodeHeaderBlock(decodeHexOrPanic("020040066d792d76616c"))
		assert.False(t, f.handler.errorDetected)
		assert.True(t, f.handler.decodingCompleted)
		require.Equal(t, 1, len(f.handler.headers))
		assert.Equal(t, "dyn-name", f.handler.headers[0].Name)
		assert.Equal(t, "my-val", f.handler.headers[0].Value)

		// 3. Literal with dynamic post-base name reference: Base 0, post-base 0
		f.handler.Reset()
		f.decoder = NewProgressiveDecoder(3, 0, f.enforcer, f.visitor, f.headerTable, f.handler)
		// RIC=1, DeltaBase=0 sign=1 -> Base 0
		// PostBase Name Ref: 0000 NNN(0) = 0x00, value: length 4 ("test")
		f.DecodeHeaderBlock(decodeHexOrPanic("0280000474657374"))
		assert.False(t, f.handler.errorDetected)
		assert.True(t, f.handler.decodingCompleted)
		require.Equal(t, 1, len(f.handler.headers))
		assert.Equal(t, "dyn-name", f.handler.headers[0].Name)
		assert.Equal(t, "test", f.handler.headers[0].Value)

		// 4. Literal without name reference: name "my-header", value "my-value"
		f.handler.Reset()
		f.decoder = NewProgressiveDecoder(4, 0, f.enforcer, f.visitor, f.headerTable, f.handler)
		f.DecodeHeaderBlock(decodeHexOrPanic("000027026d792d686561646572086d792d76616c7565"))
		assert.False(t, f.handler.errorDetected)
		assert.True(t, f.handler.decodingCompleted)
		require.Equal(t, 1, len(f.handler.headers))
		assert.Equal(t, "my-header", f.handler.headers[0].Name)
		assert.Equal(t, "my-value", f.handler.headers[0].Value)
	})
}

// -----------------------------------------------------------------------------
// Huffman Edge Cases
// -----------------------------------------------------------------------------

func TestProgressiveDecoder_HuffmanEdgeCases(t *testing.T) {
	// Valid Huffman
	runProgressiveDecoderTests(t, 1024, 1, 0, func(t *testing.T, f *progressiveDecoderTestFixture) {
		f.DecodeHeaderBlock(decodeHexOrPanic("00002f0125a849e95ba97d7f8925a849e95bb8e8b4bf"))
		assert.False(t, f.handler.errorDetected)
		assert.True(t, f.handler.decodingCompleted)
		require.Equal(t, 1, len(f.handler.headers))
		assert.Equal(t, "custom-key", f.handler.headers[0].Name)
		assert.Equal(t, "custom-value", f.handler.headers[0].Value)
	})

	// Name does not have EOS prefix
	runProgressiveDecoderTests(t, 1024, 1, 0, func(t *testing.T, f *progressiveDecoderTestFixture) {
		f.DecodeHeaderBlock(decodeHexOrPanic("00002f0125a849e95ba97d7e8925a849e95bb8e8b4bf"))
		assert.True(t, f.handler.errorDetected)
		assert.Equal(t, "Error in Huffman-encoded string.", f.handler.errorMessage)
	})

	// Value does not have EOS prefix
	runProgressiveDecoderTests(t, 1024, 1, 0, func(t *testing.T, f *progressiveDecoderTestFixture) {
		f.DecodeHeaderBlock(decodeHexOrPanic("00002f0125a849e95ba97d7f8925a849e95bb8e8b4be"))
		assert.True(t, f.handler.errorDetected)
		assert.Equal(t, "Error in Huffman-encoded string.", f.handler.errorMessage)
	})

	// Name EOS prefix too long (> 7 bits)
	runProgressiveDecoderTests(t, 1024, 1, 0, func(t *testing.T, f *progressiveDecoderTestFixture) {
		f.DecodeHeaderBlock(decodeHexOrPanic("00002f0225a849e95ba97d7fff8925a849e95bb8e8b4bf"))
		assert.True(t, f.handler.errorDetected)
		assert.Equal(t, "Error in Huffman-encoded string.", f.handler.errorMessage)
	})

	// Value EOS prefix too long (> 7 bits)
	runProgressiveDecoderTests(t, 1024, 1, 0, func(t *testing.T, f *progressiveDecoderTestFixture) {
		f.DecodeHeaderBlock(decodeHexOrPanic("00002f0125a849e95ba97d7f8a25a849e95bb8e8b4bfff"))
		assert.True(t, f.handler.errorDetected)
		assert.Equal(t, "Error in Huffman-encoded string.", f.handler.errorMessage)
	})
}

// -----------------------------------------------------------------------------
// Streaming and Chunk Boundaries
// -----------------------------------------------------------------------------

func TestProgressiveDecoder_ChunkBoundaryStreaming(t *testing.T) {
	// Full wire data with prefix + 2 headers
	fullData := decodeHexOrPanic("000023666f6f036261722362617a03717578")

	chunkSizes := []int{1, 2, 3, 5, 7, 11, len(fullData)}

	for _, chunkSize := range chunkSizes {
		t.Run("ChunkSize", func(t *testing.T) {
			table := NewDecoderHeaderTable()
			enforcer := newMockBlockedStreamLimitEnforcer(1)
			visitor := newMockDecodingCompletedVisitor()
			handler := newMockHeadersHandler()

			decoder := NewProgressiveDecoder(1, 0, enforcer, visitor, table, handler)

			for offset := 0; offset < len(fullData); offset += chunkSize {
				end := min(offset+chunkSize, len(fullData))
				decoder.Decode(fullData[offset:end])
			}
			decoder.EndHeaderBlock()

			assert.False(t, handler.errorDetected)
			assert.True(t, handler.decodingCompleted)
			require.Equal(t, 2, len(handler.headers))
			assert.Equal(t, "foo", handler.headers[0].Name)
			assert.Equal(t, "bar", handler.headers[0].Value)
			assert.Equal(t, "baz", handler.headers[1].Name)
			assert.Equal(t, "qux", handler.headers[1].Value)
		})
	}
}

// -----------------------------------------------------------------------------
// Blocking, Resumption, and Buffer Limit
// -----------------------------------------------------------------------------

func TestProgressiveDecoder_BlockingAndResumption(t *testing.T) {
	runProgressiveDecoderTests(t, 1024, 1, 0, func(t *testing.T, f *progressiveDecoderTestFixture) {
		f.headerTable.SetDynamicTableCapacity(1024)

		// Feed header block with RIC=1. Currently 0 entries, so blocks.
		f.DecodeData(decodeHexOrPanic("020080"))
		f.EndDecoding()

		assert.True(t, f.decoder.blocked)
		assert.False(t, f.handler.decodingCompleted)
		assert.Equal(t, 0, len(f.handler.headers))

		// Dynamic insert arrives
		f.headerTable.InsertEntry("foo", "bar")

		assert.False(t, f.decoder.blocked)
		assert.True(t, f.handler.decodingCompleted)
		require.Equal(t, 1, len(f.handler.headers))
		assert.Equal(t, "foo", f.handler.headers[0].Name)
		assert.Equal(t, "bar", f.handler.headers[0].Value)
		require.Equal(t, 1, len(f.visitor.completedCalls))
		assert.Equal(t, uint64(1), f.visitor.completedCalls[0].RequiredInsertCount)
	})
}

func TestProgressiveDecoder_MaxBufferedDataLimit(t *testing.T) {
	// Max buffered data is 10 bytes
	runProgressiveDecoderTests(t, 1024, 1, 10, func(t *testing.T, f *progressiveDecoderTestFixture) {
		f.headerTable.SetDynamicTableCapacity(1024)

		// Feed prefix (RIC=1) -> blocks
		f.DecodeData(decodeHexOrPanic("0200"))
		assert.True(t, f.decoder.blocked)

		// Feed 15 bytes of data while blocked -> exceeds 10 byte limit
		f.DecodeData(decodeHexOrPanic("23666f6f036261722362617a03717578"))
		assert.True(t, f.handler.errorDetected)
		assert.Equal(t, uint64(QPACK_DECOMPRESSION_FAILED), f.handler.errorCode)
		assert.Equal(t, "Too much buffered data.", f.handler.errorMessage)
	})
}

func TestProgressiveDecoder_IncompleteHeaderBlock(t *testing.T) {
	runProgressiveDecoderTests(t, 1024, 1, 0, func(t *testing.T, f *progressiveDecoderTestFixture) {
		// Cut off mid-instruction
		f.DecodeHeaderBlock(decodeHexOrPanic("00002366"))
		assert.True(t, f.handler.errorDetected)
		assert.Equal(t, "Incomplete header block.", f.handler.errorMessage)
	})
}

func TestProgressiveDecoder_RICValidation_TooLarge(t *testing.T) {
	runProgressiveDecoderTests(t, 1024, 1, 0, func(t *testing.T, f *progressiveDecoderTestFixture) {
		f.headerTable.SetDynamicTableCapacity(1024)
		f.headerTable.InsertEntry("foo", "bar")

		// Declares RIC=1, but references only static table
		f.DecodeHeaderBlock(decodeHexOrPanic("0200d1"))
		assert.True(t, f.handler.errorDetected)
		assert.Equal(t, "Required Insert Count too large.", f.handler.errorMessage)
	})
}
