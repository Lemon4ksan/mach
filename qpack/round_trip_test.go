// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package qpack

import (
	"fmt"
	"sync"
	"testing"

	"github.com/lemon4ksan/foundation/testing/assert"
	"github.com/lemon4ksan/foundation/testing/require"
)

// -----------------------------------------------------------------------------
// Roundtrip Harness: Virtual Bidirectional Control Streams
// -----------------------------------------------------------------------------

type roundtripControlStreamMode int

const (
	modeImmediate roundtripControlStreamMode = iota
	modeManual
)

type roundtripHeadersHandler struct {
	mu                sync.Mutex
	headers           []HeaderField
	decodingCompleted bool
	errorDetected     bool
	errorCode         uint64
	errorMessage      string
}

func newRoundtripHeadersHandler() *roundtripHeadersHandler {
	return &roundtripHeadersHandler{
		headers: make([]HeaderField, 0),
	}
}

func (h *roundtripHeadersHandler) OnHeaderDecoded(name, value string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.headers = append(h.headers, HeaderField{Name: name, Value: value})
}

func (h *roundtripHeadersHandler) OnDecodingCompleted() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.decodingCompleted = true
}

func (h *roundtripHeadersHandler) OnDecodingErrorDetected(errorCode uint64, errorMessage string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.errorDetected = true
	h.errorCode = errorCode
	h.errorMessage = errorMessage
}

func (h *roundtripHeadersHandler) Headers() []HeaderField {
	h.mu.Lock()
	defer h.mu.Unlock()
	cp := make([]HeaderField, len(h.headers))
	copy(cp, h.headers)
	return cp
}

func (h *roundtripHeadersHandler) Completed() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.decodingCompleted
}

func (h *roundtripHeadersHandler) HasError() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.errorDetected
}

type virtualControlStreamSender struct {
	mu               sync.Mutex
	onWrite          func(data []byte)
	numBytesBuffered uint64
}

func (s *virtualControlStreamSender) WriteStreamData(data []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.onWrite != nil {
		s.onWrite(data)
	}
}

func (s *virtualControlStreamSender) NumBytesBuffered() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.numBytesBuffered
}

type roundtripHarness struct {
	t                       *testing.T
	mode                    roundtripControlStreamMode
	maxDynamicTableCapacity uint64
	maxBlockedStreams       uint64

	encoder *Encoder
	decoder *Decoder

	encSender *virtualControlStreamSender
	decSender *virtualControlStreamSender

	encStreamBuffer []byte
	decStreamBuffer []byte

	mu sync.Mutex
}

func newRoundtripHarness(
	t *testing.T,
	maxCapacity uint64,
	maxBlocked uint64,
	mode roundtripControlStreamMode,
) *roundtripHarness {
	return newRoundtripHarnessWithSettings(
		t,
		maxCapacity,
		maxBlocked,
		mode,
		HuffmanEncodingEnabled,
		CookieCrumblingEnabled,
	)
}

func newRoundtripHarnessWithSettings(
	t *testing.T,
	maxCapacity uint64,
	maxBlocked uint64,
	mode roundtripControlStreamMode,
	huffman HuffmanEncoding,
	cookie CookieCrumbling,
) *roundtripHarness {
	h := &roundtripHarness{
		t:                       t,
		mode:                    mode,
		maxDynamicTableCapacity: maxCapacity,
		maxBlockedStreams:       maxBlocked,
	}

	h.encSender = &virtualControlStreamSender{}
	h.decSender = &virtualControlStreamSender{}

	h.encoder = NewEncoder(nil, huffman, cookie)
	h.encoder.SetMaximumDynamicTableCapacity(maxCapacity)
	h.encoder.SetMaximumBlockedStreams(maxBlocked)
	h.encoder.SetStreamSenderDelegate(h.encSender)

	h.decoder = NewDecoder(maxCapacity, maxBlocked, nil)
	h.decoder.SetStreamSenderDelegate(h.decSender)

	if mode == modeImmediate {
		h.encSender.onWrite = func(data []byte) {
			h.decoder.EncoderStreamReceiver().Decode(data)
		}
		h.decSender.onWrite = func(data []byte) {
			h.encoder.DecoderStreamReceiver().Decode(data)
		}
	} else {
		h.encSender.onWrite = func(data []byte) {
			h.encStreamBuffer = append(h.encStreamBuffer, data...)
		}
		h.decSender.onWrite = func(data []byte) {
			h.decStreamBuffer = append(h.decStreamBuffer, data...)
		}
	}

	if maxCapacity > 0 {
		h.encoder.SetDynamicTableCapacity(maxCapacity)
	}

	return h
}

func (h *roundtripHarness) DeliverEncoderStream(n int) int {
	h.mu.Lock()
	defer h.mu.Unlock()

	if len(h.encStreamBuffer) == 0 {
		return 0
	}

	if n <= 0 || n >= len(h.encStreamBuffer) {
		toDeliver := h.encStreamBuffer
		h.encStreamBuffer = nil
		h.decoder.EncoderStreamReceiver().Decode(toDeliver)
		return len(toDeliver)
	}

	chunk := h.encStreamBuffer[:n]
	h.encStreamBuffer = h.encStreamBuffer[n:]
	h.decoder.EncoderStreamReceiver().Decode(chunk)
	return len(chunk)
}

func (h *roundtripHarness) DeliverDecoderStream(n int) int {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.decoder.FlushDecoderStream()

	if len(h.decStreamBuffer) == 0 {
		return 0
	}

	if n <= 0 || n >= len(h.decStreamBuffer) {
		toDeliver := h.decStreamBuffer
		h.decStreamBuffer = nil
		h.encoder.DecoderStreamReceiver().Decode(toDeliver)
		return len(toDeliver)
	}

	chunk := h.decStreamBuffer[:n]
	h.decStreamBuffer = h.decStreamBuffer[n:]
	h.encoder.DecoderStreamReceiver().Decode(chunk)
	return len(chunk)
}

func (h *roundtripHarness) FlushDecoderStream() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.decoder.FlushDecoderStream()
}

func (h *roundtripHarness) FlushEncoderStream() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.encoder.EncoderStreamSender().Flush()
}

func (h *roundtripHarness) Encode(streamID uint64, fields []HeaderField) []byte {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.encoder.EncodeHeaderList(streamID, fields, nil)
}

func (h *roundtripHarness) EncodeWithByteCount(streamID uint64, fields []HeaderField) ([]byte, uint64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	var sentBytes uint64
	block := h.encoder.EncodeHeaderList(streamID, fields, &sentBytes)
	return block, sentBytes
}

func (h *roundtripHarness) DecodeBlock(streamID uint64, block []byte) *roundtripHeadersHandler {
	handler := newRoundtripHeadersHandler()
	h.mu.Lock()
	defer h.mu.Unlock()

	progDec := h.decoder.CreateProgressiveDecoder(streamID, handler)
	progDec.Decode(block)
	progDec.EndHeaderBlock()

	if h.mode == modeImmediate {
		h.decoder.FlushDecoderStream()
	}

	return handler
}

func (h *roundtripHarness) Roundtrip(streamID uint64, fields []HeaderField) []HeaderField {
	return h.RoundtripWithExpected(streamID, fields, fields)
}

func (h *roundtripHarness) RoundtripWithExpected(streamID uint64, fields, expected []HeaderField) []HeaderField {
	h.t.Helper()
	block := h.Encode(streamID, fields)
	if h.mode == modeManual {
		h.DeliverEncoderStream(0)
	}

	handler := h.DecodeBlock(streamID, block)
	if h.mode == modeManual {
		h.DeliverDecoderStream(0)
	}

	require.True(h.t, handler.Completed(), "decoding must be completed")
	require.False(h.t, handler.HasError(), "decoding must not have error")

	decoded := handler.Headers()
	require.Equal(h.t, len(expected), len(decoded), "header count mismatch")
	for i := range expected {
		assert.Equalf(h.t, expected[i].Name, decoded[i].Name, "header name mismatch at index %d", i)
		assert.Equalf(h.t, expected[i].Value, decoded[i].Value, "header value mismatch at index %d", i)
	}
	return decoded
}

// -----------------------------------------------------------------------------
// Test Suite 1: Sequential HTTP/3 Request Streams & Dynamic Table Warmup
// -----------------------------------------------------------------------------

func TestRoundtrip_SequentialStreams_DynamicTableWarmup(t *testing.T) {
	t.Parallel()
	h := newRoundtripHarness(t, 4096, 10, modeImmediate)

	req1 := []HeaderField{
		{Name: ":method", Value: "GET"},
		{Name: ":scheme", Value: "https"},
		{Name: ":authority", Value: "api.example.com"},
		{Name: ":path", Value: "/v1/users"},
		{Name: "user-agent", Value: "CustomClient/1.0 (IntegrationTest)"},
		{Name: "accept", Value: "application/json"},
		{Name: "x-request-id", Value: "req-seq-001"},
	}

	block1, encBytes1 := h.EncodeWithByteCount(1, req1)
	handler1 := h.DecodeBlock(1, block1)
	require.True(t, handler1.Completed())
	assert.Equal(t, req1, handler1.Headers())

	assert.True(t, h.encoder.HeaderTable().InsertedEntryCount() > 0)
	assert.Equal(t, h.encoder.HeaderTable().InsertedEntryCount(), h.decoder.HeaderTable().InsertedEntryCount())
	assert.Equal(t, h.decoder.KnownReceivedCount(), h.encoder.BlockingManager().KnownReceivedCount())

	// Stream 2: Cache hit on authority, user-agent, and accept
	req2 := []HeaderField{
		{Name: ":method", Value: "GET"},
		{Name: ":scheme", Value: "https"},
		{Name: ":authority", Value: "api.example.com"},
		{Name: ":path", Value: "/v1/users/profile"},
		{Name: "user-agent", Value: "CustomClient/1.0 (IntegrationTest)"},
		{Name: "accept", Value: "application/json"},
		{Name: "x-request-id", Value: "req-seq-002"},
	}

	block2, encBytes2 := h.EncodeWithByteCount(2, req2)
	assert.Truef(
		t,
		encBytes2 < encBytes1,
		"stream 2 encoder stream bytes (%d) should be smaller than stream 1 (%d) due to dynamic table hits",
		encBytes2,
		encBytes1,
	)
	assert.Truef(
		t,
		len(block2)+int(encBytes2) < len(block1)+int(encBytes1),
		"stream 2 total wire size (%d) should be smaller than stream 1 (%d) due to dynamic table hits",
		len(block2)+int(encBytes2),
		len(block1)+int(encBytes1),
	)

	handler2 := h.DecodeBlock(2, block2)
	require.True(t, handler2.Completed())
	assert.Equal(t, req2, handler2.Headers())

	// Stream 3: Post with partial header reuse
	req3 := []HeaderField{
		{Name: ":method", Value: "POST"},
		{Name: ":scheme", Value: "https"},
		{Name: ":authority", Value: "api.example.com"},
		{Name: ":path", Value: "/v1/users"},
		{Name: "content-type", Value: "application/json"},
		{Name: "x-request-id", Value: "req-seq-003"},
	}

	h.Roundtrip(3, req3)

	assert.Equal(t, h.encoder.HeaderTable().InsertedEntryCount(), h.decoder.HeaderTable().InsertedEntryCount())
	assert.Equal(t, h.encoder.BlockingManager().KnownReceivedCount(), h.decoder.KnownReceivedCount())
}

// -----------------------------------------------------------------------------
// Test Suite 2: Concurrent HTTP/3 Request Streams & Multi-Worker Multiplexing
// -----------------------------------------------------------------------------

func TestRoundtrip_ConcurrentStreams_MultiWorkerMultiplexing(t *testing.T) {
	t.Parallel()
	h := newRoundtripHarness(t, 8192, 50, modeImmediate)

	const numWorkers = 20
	const streamsPerWorker = 10
	var wg sync.WaitGroup
	wg.Add(numWorkers)

	for w := 0; w < numWorkers; w++ {
		go func(workerID int) {
			defer wg.Done()
			for s := 0; s < streamsPerWorker; s++ {
				streamID := uint64(workerID*1000 + s*2 + 1)
				headers := []HeaderField{
					{Name: ":method", Value: "GET"},
					{Name: ":scheme", Value: "https"},
					{Name: ":authority", Value: "cluster.internal.api"},
					{Name: ":path", Value: fmt.Sprintf("/worker/%d/task/%d", workerID, s)},
					{Name: "user-agent", Value: "Go-QPACK-Integration/2.0"},
					{Name: "x-worker-id", Value: fmt.Sprintf("worker-%d", workerID)},
					{Name: "x-seq-num", Value: fmt.Sprintf("seq-%d", s)},
				}
				h.Roundtrip(streamID, headers)
			}
		}(w)
	}

	wg.Wait()

	assert.Equal(t, h.encoder.HeaderTable().InsertedEntryCount(), h.decoder.HeaderTable().InsertedEntryCount())
	assert.Equal(t, h.encoder.BlockingManager().KnownReceivedCount(), h.decoder.KnownReceivedCount())
}

// -----------------------------------------------------------------------------
// Test Suite 3: Dynamic Table Entry Deduplication, Eviction, and Barrier
// -----------------------------------------------------------------------------

func TestRoundtrip_DynamicTable_DeduplicationAndEviction(t *testing.T) {
	t.Parallel()
	// 250 bytes fits approximately 3 custom headers (len("custom-header-X")+len("value-X")+32 ~ 65 bytes)
	h := newRoundtripHarness(t, 250, 5, modeImmediate)

	// Step 1: Insert 3 entries
	req1 := []HeaderField{
		{Name: "x-custom-1", Value: "value-one-12345"},
		{Name: "x-custom-2", Value: "value-two-12345"},
		{Name: "x-custom-3", Value: "value-three-12345"},
	}
	h.Roundtrip(1, req1)
	initialInserts := h.encoder.HeaderTable().InsertedEntryCount()
	assert.Equal(t, uint64(3), initialInserts)
	assert.Equal(t, uint64(0), h.encoder.HeaderTable().DroppedEntryCount())

	// Step 2: Deduplication: re-sending the same entries must not insert duplicates
	h.Roundtrip(2, req1)
	assert.Equal(t, initialInserts, h.encoder.HeaderTable().InsertedEntryCount())
	assert.Equal(t, uint64(0), h.encoder.HeaderTable().DroppedEntryCount())

	// Step 3: Eviction: insert new entries exceeding capacity
	req3 := []HeaderField{
		{Name: "x-custom-4", Value: "value-four-12345"},
		{Name: "x-custom-5", Value: "value-five-12345"},
	}
	h.Roundtrip(3, req3)

	assert.True(t, h.encoder.HeaderTable().DroppedEntryCount() > 0)
	assert.Equal(t, h.encoder.HeaderTable().DroppedEntryCount(), h.decoder.HeaderTable().DroppedEntryCount())
	assert.Equal(t, h.encoder.HeaderTable().InsertedEntryCount(), h.decoder.HeaderTable().InsertedEntryCount())
}

// -----------------------------------------------------------------------------
// Test Suite 4: Delayed Decoder Feedback & Blocked Streams Quota
// -----------------------------------------------------------------------------

func TestRoundtrip_DelayedFeedback_BlockedStreamsQuota(t *testing.T) {
	t.Parallel()
	// Maximum blocked streams = 2
	h := newRoundtripHarness(t, 4096, 2, modeManual)

	// Stream 10: insert dynamic entry
	block1 := h.Encode(10, []HeaderField{{Name: "x-dyn-1", Value: "val-1"}})
	h.DeliverEncoderStream(0)
	handler1 := h.DecodeBlock(10, block1)
	require.True(t, handler1.Completed())

	// Stream 20: insert dynamic entry
	block2 := h.Encode(20, []HeaderField{{Name: "x-dyn-2", Value: "val-2"}})
	h.DeliverEncoderStream(0)
	handler2 := h.DecodeBlock(20, block2)
	require.True(t, handler2.Completed())

	// Notice: decoder feedback has NOT been delivered yet!
	// In-flight streams referencing unacknowledged entries = 2 (max reached)
	// Stream 30: encode headers. Because limit is reached, encoder must not create blocking references
	block3 := h.Encode(30, []HeaderField{{Name: "x-dyn-3", Value: "val-3"}})
	h.DeliverEncoderStream(0)
	handler3 := h.DecodeBlock(30, block3)
	require.True(t, handler3.Completed())

	// Now deliver all queued decoder feedback (Section Acks) to encoder
	h.DeliverDecoderStream(0)

	// Stream 40: can freely insert and reference dynamic entries again
	h.Roundtrip(40, []HeaderField{{Name: "x-dyn-4", Value: "val-4"}})

	assert.Equal(t, h.encoder.HeaderTable().InsertedEntryCount(), h.decoder.HeaderTable().InsertedEntryCount())
	assert.Equal(t, h.encoder.BlockingManager().KnownReceivedCount(), h.decoder.KnownReceivedCount())
}

// -----------------------------------------------------------------------------
// Test Suite 5: Out-of-Order Header Block Arrivals (Head-of-Line Blocking & Unblocking)
// -----------------------------------------------------------------------------

func TestRoundtrip_OutOfOrderArrival_StreamBlockingAndUnblocking(t *testing.T) {
	t.Parallel()
	h := newRoundtripHarness(t, 4096, 5, modeManual)

	// Encode Stream 100 with dynamic entry (generates encoder stream insert for entry 0)
	block100 := h.Encode(100, []HeaderField{{Name: "x-hol-key1", Value: "hol-value-1"}})

	// Encode Stream 200 with dynamic entry (generates encoder stream insert for entry 1)
	block200 := h.Encode(200, []HeaderField{{Name: "x-hol-key2", Value: "hol-value-2"}})

	// Deliver Stream 200 header block FIRST to decoder (encoder stream has NOT arrived)
	handler200 := newRoundtripHeadersHandler()
	pDec200 := h.decoder.CreateProgressiveDecoder(200, handler200)
	pDec200.Decode(block200)
	pDec200.EndHeaderBlock()

	// Stream 200 must be blocked waiting on Required Insert Count = 2
	assert.False(t, handler200.Completed(), "Stream 200 must block when RIC > InsertedEntryCount")

	// Deliver Stream 100 header block to decoder (waiting on RIC = 1)
	handler100 := newRoundtripHeadersHandler()
	pDec100 := h.decoder.CreateProgressiveDecoder(100, handler100)
	pDec100.Decode(block100)
	pDec100.EndHeaderBlock()

	assert.False(t, handler100.Completed(), "Stream 100 must block when RIC > InsertedEntryCount")

	// Deliver first encoder stream instruction (Insert entry 1)
	// This unblocks Stream 100 (RIC=1), but Stream 200 (RIC=2) must stay blocked
	instruction1Len := 1 + 1 + len("x-hol-key1") + len("hol-value-1") + 5
	h.DeliverEncoderStream(instruction1Len)

	// Stream 100 should now be completed
	assert.True(t, handler100.Completed(), "Stream 100 must unblock when RIC 1 is reached")
	assert.Equal(t, []HeaderField{{Name: "x-hol-key1", Value: "hol-value-1"}}, handler100.Headers())

	// Stream 200 must still be blocked
	assert.False(t, handler200.Completed(), "Stream 200 must remain blocked until RIC 2")

	// Deliver remaining encoder stream instructions
	h.DeliverEncoderStream(0)

	// Stream 200 should now cleanly unblock and complete
	assert.True(t, handler200.Completed(), "Stream 200 must unblock when RIC 2 is reached")
	assert.Equal(t, []HeaderField{{Name: "x-hol-key2", Value: "hol-value-2"}}, handler200.Headers())

	// Deliver decoder feedback
	h.DeliverDecoderStream(0)
	assert.Equal(t, h.encoder.HeaderTable().InsertedEntryCount(), h.decoder.HeaderTable().InsertedEntryCount())
}

// -----------------------------------------------------------------------------
// Test Suite 6: Chunked & Octet-By-Octet Streaming Delivery
// -----------------------------------------------------------------------------

func TestRoundtrip_ChunkedAndOctetByOctet_StreamingDelivery(t *testing.T) {
	t.Parallel()
	h := newRoundtripHarness(t, 4096, 5, modeManual)

	fields := []HeaderField{
		{Name: ":method", Value: "GET"},
		{Name: ":path", Value: "/stream/chunked"},
		{Name: "authorization", Value: "Bearer token-with-a-moderately-long-random-string-1234567890-abcdef"},
		{Name: "x-custom-streaming", Value: "payload-bytes-testing-octet-by-octet-parser-state-machine"},
	}

	block := h.Encode(1, fields)

	// Deliver encoder stream octet by octet
	for len(h.encStreamBuffer) > 0 {
		h.DeliverEncoderStream(1)
	}

	// Deliver request stream header block octet by octet
	handler := newRoundtripHeadersHandler()
	pDec := h.decoder.CreateProgressiveDecoder(1, handler)
	for len(block) > 0 {
		pDec.Decode(block[:1])
		block = block[1:]
	}
	pDec.EndHeaderBlock()

	require.True(t, handler.Completed())
	assert.Equal(t, fields, handler.Headers())

	// Deliver decoder feedback octet by octet
	h.FlushDecoderStream()
	for len(h.decStreamBuffer) > 0 {
		h.DeliverDecoderStream(1)
	}

	assert.Equal(t, h.encoder.HeaderTable().InsertedEntryCount(), h.decoder.HeaderTable().InsertedEntryCount())
}

// -----------------------------------------------------------------------------
// Test Suite 7: Stream Reset and Stream Cancellation Feedback
// -----------------------------------------------------------------------------

func TestRoundtrip_StreamReset_StreamCancellationFeedback(t *testing.T) {
	t.Parallel()
	h := newRoundtripHarness(t, 512, 5, modeManual)

	// Stream 50 encodes dynamic entry
	block50 := h.Encode(50, []HeaderField{{Name: "x-reset-header", Value: "reset-val"}})
	h.DeliverEncoderStream(0)

	// Decoder starts decoding Stream 50
	handler50 := newRoundtripHeadersHandler()
	pDec50 := h.decoder.CreateProgressiveDecoder(50, handler50)
	pDec50.Decode(block50)

	// Client cancels stream before completion
	h.decoder.OnStreamReset(50)
	pDec50.Close()

	// Deliver decoder feedback to encoder: should contain Stream Cancellation
	h.DeliverDecoderStream(0)

	// Verify encoder unblocked stream 50
	assert.False(t, h.encoder.BlockingManager().IsBlocked(50))
}

// -----------------------------------------------------------------------------
// Test Suite 8: Dynamic Table Capacity Changes (Mid-Connection Resizing)
// -----------------------------------------------------------------------------

func TestRoundtrip_DynamicTable_CapacityShrinkingAndGrowing(t *testing.T) {
	t.Parallel()
	h := newRoundtripHarness(t, 2048, 5, modeImmediate)

	// Insert entries filling ~800 bytes
	fields := []HeaderField{
		{Name: "x-fill-1", Value: "large-value-padding-string-123456789012345678901234567890"},
		{Name: "x-fill-2", Value: "large-value-padding-string-123456789012345678901234567890"},
		{Name: "x-fill-3", Value: "large-value-padding-string-123456789012345678901234567890"},
	}
	h.Roundtrip(1, fields)

	// Shrink dynamic table capacity to 200 bytes
	h.encoder.SetDynamicTableCapacity(200)
	h.FlushEncoderStream()
	assert.Equal(t, uint64(200), h.encoder.HeaderTable().DynamicTableCapacity())
	assert.Equal(t, uint64(200), h.decoder.HeaderTable().DynamicTableCapacity())
	assert.Equal(t, h.encoder.HeaderTable().DroppedEntryCount(), h.decoder.HeaderTable().DroppedEntryCount())

	// Roundtrip under reduced capacity
	h.Roundtrip(2, []HeaderField{{Name: "x-new-1", Value: "fit-in-200"}})

	// Grow dynamic table capacity back up to 2048 bytes
	h.encoder.SetDynamicTableCapacity(2048)
	h.FlushEncoderStream()
	assert.Equal(t, uint64(2048), h.encoder.HeaderTable().DynamicTableCapacity())
	assert.Equal(t, uint64(2048), h.decoder.HeaderTable().DynamicTableCapacity())
}

// -----------------------------------------------------------------------------
// Test Suite 9: Graceful Shutdown & Abortive Stream Draining
// -----------------------------------------------------------------------------

func TestRoundtrip_GracefulShutdown_PendingStreamDraining(t *testing.T) {
	t.Parallel()
	h := newRoundtripHarness(t, 2048, 5, modeManual)

	// Start 3 progressive decoders with partial data
	decoders := make([]*ProgressiveDecoder, 3)
	for i := 0; i < 3; i++ {
		streamID := uint64(10 + i)
		block := h.Encode(streamID, []HeaderField{{Name: fmt.Sprintf("x-term-%d", i), Value: "val"}})
		pDec := h.decoder.CreateProgressiveDecoder(streamID, newRoundtripHeadersHandler())
		pDec.Decode(block)
		decoders[i] = pDec
	}

	// Graceful shutdown: close all pending decoders
	for _, pDec := range decoders {
		pDec.Close()
	}

	// Verify no pending blocked streams remain on decoder
	assert.Equal(t, 0, len(h.decoder.blockedStreams))
}

// -----------------------------------------------------------------------------
// Test Suite 10: Huffman Encoding Combinations Matrix
// -----------------------------------------------------------------------------

func TestRoundtrip_HuffmanCombinations_Matrix(t *testing.T) {
	t.Parallel()

	huffmanModes := []struct {
		name string
		mode HuffmanEncoding
	}{
		{"HuffmanEnabled", HuffmanEncodingEnabled},
		{"HuffmanDisabled", HuffmanEncodingDisabled},
	}

	cookieModes := []struct {
		name string
		mode CookieCrumbling
	}{
		{"CookieCrumblingEnabled", CookieCrumblingEnabled},
		{"CookieCrumblingDisabled", CookieCrumblingDisabled},
	}

	for _, hm := range huffmanModes {
		for _, cm := range cookieModes {
			testName := fmt.Sprintf("%s_%s", hm.name, cm.name)
			t.Run(testName, func(t *testing.T) {
				h := newRoundtripHarnessWithSettings(t, 4096, 10, modeImmediate, hm.mode, cm.mode)

				headers := []HeaderField{
					{Name: ":method", Value: "GET"},
					{Name: ":path", Value: "/test/matrix"},
					{Name: "cookie", Value: "session=xyz123; theme=dark; tracking=off"},
					{Name: "x-custom-mixed", Value: "AlphaNumeric12345!@#$%^&*()_+~`"},
				}

				var expected []HeaderField
				if cm.mode == CookieCrumblingEnabled {
					expected = []HeaderField{
						{Name: ":method", Value: "GET"},
						{Name: ":path", Value: "/test/matrix"},
						{Name: "cookie", Value: "session=xyz123"},
						{Name: "cookie", Value: "theme=dark"},
						{Name: "cookie", Value: "tracking=off"},
						{Name: "x-custom-mixed", Value: "AlphaNumeric12345!@#$%^&*()_+~`"},
					}
				} else {
					expected = headers
				}

				h.RoundtripWithExpected(1, headers, expected)
				assert.Equal(
					t,
					h.encoder.HeaderTable().InsertedEntryCount(),
					h.decoder.HeaderTable().InsertedEntryCount(),
				)
			})
		}
	}
}
