// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package qpack

import (
	"bytes"
	"fmt"
	"math/rand"
	"sync"
	"testing"

	"github.com/lemon4ksan/foundation/testing/assert"
	"github.com/lemon4ksan/foundation/testing/require"
)

// -----------------------------------------------------------------------------
// Wire Helpers for Encoder Stream Instructions
// -----------------------------------------------------------------------------

func encodeEncoderCapacity(cap uint64) []byte {
	sender := NewEncoderStreamSenderWithHuffman(HuffmanEncodingDisabled, nil)
	sender.SendSetDynamicTableCapacity(cap)
	return sender.buffer
}

func encodeEncoderInsertWithoutNameRef(name, value string) []byte {
	sender := NewEncoderStreamSenderWithHuffman(HuffmanEncodingDisabled, nil)
	sender.SendInsertWithoutNameReference(name, value)
	return sender.buffer
}

// -----------------------------------------------------------------------------
// Thread-Safe Test Delegates for Concurrent Stress Tests
// -----------------------------------------------------------------------------

type threadSafeStreamSenderDelegate struct {
	mu     sync.Mutex
	writes [][]byte
}

func newThreadSafeStreamSenderDelegate() *threadSafeStreamSenderDelegate {
	return &threadSafeStreamSenderDelegate{
		writes: make([][]byte, 0),
	}
}

func (d *threadSafeStreamSenderDelegate) WriteStreamData(data []byte) {
	d.mu.Lock()
	defer d.mu.Unlock()
	cp := make([]byte, len(data))
	copy(cp, data)
	d.writes = append(d.writes, cp)
}

func (d *threadSafeStreamSenderDelegate) NumBytesBuffered() uint64 {
	return 0
}

func (d *threadSafeStreamSenderDelegate) WrittenData() []byte {
	d.mu.Lock()
	defer d.mu.Unlock()
	var total []byte
	for _, w := range d.writes {
		total = append(total, w...)
	}
	return total
}

func (d *threadSafeStreamSenderDelegate) Clear() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.writes = nil
}

type threadSafeHeadersHandler struct {
	mu                sync.Mutex
	headers           []headerCall
	decodingCompleted bool
	errorDetected     bool
	errorCode         uint64
	errorMessage      string
}

func newThreadSafeHeadersHandler() *threadSafeHeadersHandler {
	return &threadSafeHeadersHandler{
		headers: make([]headerCall, 0),
	}
}

func (h *threadSafeHeadersHandler) OnHeaderDecoded(name, value string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.headers = append(h.headers, headerCall{Name: name, Value: value})
}

func (h *threadSafeHeadersHandler) OnDecodingCompleted() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.decodingCompleted = true
}

func (h *threadSafeHeadersHandler) OnDecodingErrorDetected(errorCode uint64, errorMessage string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.errorDetected = true
	h.errorCode = errorCode
	h.errorMessage = errorMessage
}

func (h *threadSafeHeadersHandler) IsCompleted() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.decodingCompleted
}

func (h *threadSafeHeadersHandler) HasError() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.errorDetected
}

func (h *threadSafeHeadersHandler) HeaderCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.headers)
}

// -----------------------------------------------------------------------------
// Category 1: Stream Blocking, Unblocking, and Observer Dispatch
// -----------------------------------------------------------------------------

// TestM5StreamDrainAdversarial_OutOfOrderBlockingAndThresholdUnblocking
// Simulates 6 streams arriving in mixed order, waiting on different RIC thresholds,
// unblocking when their specific threshold is reached by dynamic table insertions,
// and correctly draining buffered data without loss.
func TestM5StreamDrainAdversarial_OutOfOrderBlockingAndThresholdUnblocking(t *testing.T) {
	sender := newMockStreamSenderDelegate()
	decoder := NewDecoder(2048, 10, nil)
	decoder.SetStreamSenderDelegate(sender)

	// Pre-condition: dynamic table capacity set to 2048
	decoder.EncoderStreamReceiver().Decode(encodeEncoderCapacity(2048))

	type streamPlan struct {
		id       uint64
		ric      uint64
		base     uint64
		wire     []byte
		handler  *mockHeadersHandler
		decoder  *ProgressiveDecoder
		expected []headerCall
	}

	plans := []*streamPlan{
		{
			id:   10,
			ric:  1,
			base: 1,
			// Prefix: RIC=1 (encoded 2), Base=1 (DeltaBase=0, S=0) -> 0200
			// Rel index 0 (Abs 0) -> opcode 0x80
			wire:    decodeHexOrPanic("020080"),
			handler: newMockHeadersHandler(),
			expected: []headerCall{
				{Name: "key-0", Value: "val-0"},
			},
		},
		{
			id:   20,
			ric:  4,
			base: 4,
			// Prefix: RIC=4 (encoded 5), Base=4 (DeltaBase=0, S=0) -> 0500
			// Rel index 0 (Abs = 4-0-1 = 3) -> 0x80
			wire:    decodeHexOrPanic("050080"),
			handler: newMockHeadersHandler(),
			expected: []headerCall{
				{Name: "key-3", Value: "val-3"},
			},
		},
		{
			id:   30,
			ric:  2,
			base: 2,
			// Prefix: RIC=2 (encoded 3), Base=2 -> 0300
			// Rel index 1 (Abs = 2-1-1 = 0) -> 0x81
			// Rel index 0 (Abs = 2-0-1 = 1) -> 0x80
			wire:    decodeHexOrPanic("03008180"),
			handler: newMockHeadersHandler(),
			expected: []headerCall{
				{Name: "key-0", Value: "val-0"},
				{Name: "key-1", Value: "val-1"},
			},
		},
		{
			id:   40,
			ric:  6,
			base: 6,
			// Prefix: RIC=6 (encoded 7), Base=6 -> 0700
			// Rel index 0 (Abs = 6-0-1 = 5) -> 0x80
			wire:    decodeHexOrPanic("070080"),
			handler: newMockHeadersHandler(),
			expected: []headerCall{
				{Name: "key-5", Value: "val-5"},
			},
		},
		{
			id:   50,
			ric:  0,
			base: 0,
			// Prefix: RIC=0, Base=0 -> 0000
			// Static index 17 (:method: GET) -> 0xd1
			wire:    decodeHexOrPanic("0000d1"),
			handler: newMockHeadersHandler(),
			expected: []headerCall{
				{Name: ":method", Value: "GET"},
			},
		},
		{
			id:   60,
			ric:  4,
			base: 4,
			// Prefix: RIC=4 (encoded 5), Base=4 -> 0500
			// Rel index 1 (Abs = 4-1-1 = 2) -> 0x81
			// Rel index 0 (Abs = 4-0-1 = 3) -> 0x80
			// References entries 2 and 3, so RICSoFar = 3 + 1 = 4 matches RIC=4.
			wire:    decodeHexOrPanic("05008180"),
			handler: newMockHeadersHandler(),
			expected: []headerCall{
				{Name: "key-2", Value: "val-2"},
				{Name: "key-3", Value: "val-3"},
			},
		},
	}

	// 1. Dispatch streams out-of-order: 50 (RIC 0), 40 (RIC 6), 20 (RIC 4), 60 (RIC 4), 10 (RIC 1), 30 (RIC 2)
	dispatchOrder := []int{4, 3, 1, 5, 0, 2}
	for _, idx := range dispatchOrder {
		p := plans[idx]
		p.decoder = decoder.CreateProgressiveDecoder(p.id, p.handler)
		p.decoder.Decode(p.wire)
		p.decoder.EndHeaderBlock()
	}

	// Stream 50 (RIC 0) should be completed immediately without blocking
	assert.True(t, plans[4].handler.decodingCompleted, "Stream 50 must complete immediately")
	assert.False(t, plans[4].handler.errorDetected)
	require.Equal(t, 1, len(plans[4].handler.headers))
	assert.Equal(t, ":method", plans[4].handler.headers[0].Name)

	// All other 5 streams must be blocked waiting on dynamic insertions
	for _, idx := range []int{0, 1, 2, 3, 5} {
		assert.False(t, plans[idx].handler.decodingCompleted, fmt.Sprintf("Stream %d must be blocked", plans[idx].id))
		assert.False(t, plans[idx].handler.errorDetected)
	}

	insertEntry := func(k, v string) {
		decoder.EncoderStreamReceiver().Decode(encodeEncoderInsertWithoutNameRef(k, v))
	}

	// 2. Insert Entry 0: ("key-0", "val-0") -> InsertedCount becomes 1
	// Threshold 1 reached! Stream 10 unblocks; Streams 20, 30, 40, 60 remain blocked.
	insertEntry("key-0", "val-0")
	require.Equal(t, uint64(1), decoder.HeaderTable().InsertedEntryCount())

	assert.True(t, plans[0].handler.decodingCompleted, "Stream 10 must unblock at InsertCount=1")
	assert.Equal(t, plans[0].expected, plans[0].handler.headers)
	assert.False(t, plans[2].handler.decodingCompleted, "Stream 30 must remain blocked (RIC=2)")
	assert.False(t, plans[1].handler.decodingCompleted, "Stream 20 must remain blocked (RIC=4)")
	assert.False(t, plans[5].handler.decodingCompleted, "Stream 60 must remain blocked (RIC=4)")
	assert.False(t, plans[3].handler.decodingCompleted, "Stream 40 must remain blocked (RIC=6)")

	// 3. Insert Entry 1: ("key-1", "val-1") -> InsertedCount becomes 2
	// Threshold 2 reached! Stream 30 unblocks; Streams 20, 40, 60 remain blocked.
	insertEntry("key-1", "val-1")
	require.Equal(t, uint64(2), decoder.HeaderTable().InsertedEntryCount())

	assert.True(t, plans[2].handler.decodingCompleted, "Stream 30 must unblock at InsertCount=2")
	assert.Equal(t, plans[2].expected, plans[2].handler.headers)
	assert.False(t, plans[1].handler.decodingCompleted, "Stream 20 must remain blocked")
	assert.False(t, plans[5].handler.decodingCompleted, "Stream 60 must remain blocked")
	assert.False(t, plans[3].handler.decodingCompleted, "Stream 40 must remain blocked")

	// 4. Insert Entry 2: ("key-2", "val-2") -> InsertedCount becomes 3
	// Threshold 3 does not unblock remaining streams (they need 4 and 6).
	insertEntry("key-2", "val-2")
	require.Equal(t, uint64(3), decoder.HeaderTable().InsertedEntryCount())
	assert.False(t, plans[1].handler.decodingCompleted)
	assert.False(t, plans[5].handler.decodingCompleted)
	assert.False(t, plans[3].handler.decodingCompleted)

	// 5. Insert Entry 3: ("key-3", "val-3") -> InsertedCount becomes 4
	// Threshold 4 reached! Both Stream 20 AND Stream 60 must unblock and drain!
	insertEntry("key-3", "val-3")
	require.Equal(t, uint64(4), decoder.HeaderTable().InsertedEntryCount())

	assert.True(t, plans[1].handler.decodingCompleted, "Stream 20 must unblock at InsertCount=4")
	assert.Equal(t, plans[1].expected, plans[1].handler.headers)
	assert.True(t, plans[5].handler.decodingCompleted, "Stream 60 must unblock at InsertCount=4")
	assert.Equal(t, plans[5].expected, plans[5].handler.headers)
	assert.False(t, plans[3].handler.decodingCompleted, "Stream 40 must remain blocked (RIC=6)")

	// 6. Insert Entry 4: ("key-4", "val-4") -> InsertedCount becomes 5
	insertEntry("key-4", "val-4")
	require.Equal(t, uint64(5), decoder.HeaderTable().InsertedEntryCount())
	assert.False(t, plans[3].handler.decodingCompleted)

	// 7. Insert Entry 5: ("key-5", "val-5") -> InsertedCount becomes 6
	// Threshold 6 reached! Stream 40 unblocks!
	insertEntry("key-5", "val-5")
	require.Equal(t, uint64(6), decoder.HeaderTable().InsertedEntryCount())

	assert.True(t, plans[3].handler.decodingCompleted, "Stream 40 must unblock at InsertCount=6")
	assert.Equal(t, plans[3].expected, plans[3].handler.headers)

	// All 6 streams successfully drained without any error
	for _, p := range plans {
		assert.False(
			t,
			p.handler.errorDetected,
			fmt.Sprintf("stream %d should have no error: %s", p.id, p.handler.errorMessage),
		)
		assert.True(t, p.handler.decodingCompleted, fmt.Sprintf("stream %d should be completed", p.id))
	}
}

// TestM5StreamDrainAdversarial_DeepBuffering_LosslessDrainOnThreshold
// Feeds 20 fragmented chunks containing 15 different field line instructions while blocked,
// calls EndHeaderBlock while still blocked, and verifies that upon reaching threshold all
// buffered data is drained and decoded completely and losslessly.
func TestM5StreamDrainAdversarial_DeepBuffering_LosslessDrainOnThreshold(t *testing.T) {
	sender := newMockStreamSenderDelegate()
	decoder := NewDecoder(4096, 5, nil)
	decoder.SetStreamSenderDelegate(sender)
	decoder.EncoderStreamReceiver().Decode(encodeEncoderCapacity(4096))

	var block bytes.Buffer
	// Prefix: RIC=3 (encoded 4), Base=3 (DeltaBase=0, S=0) -> 0400
	block.Write(decodeHexOrPanic("0400"))

	expectedHeaders := make([]headerCall, 0)

	// 1. Static :method GET (0xd1)
	block.WriteByte(0xd1)
	expectedHeaders = append(expectedHeaders, headerCall{Name: ":method", Value: "GET"})

	// 2. Dynamic Rel 2 -> Abs 3-2-1 = 0 ("d-key-0", "d-val-0") -> 0x82
	block.WriteByte(0x82)
	expectedHeaders = append(expectedHeaders, headerCall{Name: "d-key-0", Value: "d-val-0"})

	// 3. Static :scheme https (0xd7)
	block.WriteByte(0xd7)
	expectedHeaders = append(expectedHeaders, headerCall{Name: ":scheme", Value: "https"})

	// 4. Dynamic Rel 1 -> Abs 3-1-1 = 1 ("d-key-1", "d-val-1") -> 0x81
	block.WriteByte(0x81)
	expectedHeaders = append(expectedHeaders, headerCall{Name: "d-key-1", Value: "d-val-1"})

	// 5. Dynamic Rel 0 -> Abs 3-0-1 = 2 ("d-key-2", "d-val-2") -> 0x80
	block.WriteByte(0x80)
	expectedHeaders = append(expectedHeaders, headerCall{Name: "d-key-2", Value: "d-val-2"})

	// 6. Literal with static name ref (:path, static index 1 -> 0x51)
	block.WriteByte(0x51)
	block.WriteByte(byte(len("/deep/buffering/adversarial")))
	block.WriteString("/deep/buffering/adversarial")
	expectedHeaders = append(expectedHeaders, headerCall{Name: ":path", Value: "/deep/buffering/adversarial"})

	// 7. Plain literal (no ref): RFC 9204 §4.5.6: 001 H(0) NNN(3-bit prefix for name length)
	plainField := encodeLiteralFieldWithoutRefAdversarial("x-custom-header", "plain-value-12345")
	block.Write(plainField)
	expectedHeaders = append(expectedHeaders, headerCall{Name: "x-custom-header", Value: "plain-value-12345"})

	// 8-15. 8 static indexed headers: indices 20..27
	// 20: :method: POST (0xd4)
	// 21: :method: PUT (0xd5)
	// 22: :scheme: http (0xd6)
	// 23: :scheme: https (0xd7)
	// 24: :status: 103 (0xd8)
	// 25: :status: 200 (0xd9)
	// 26: :status: 304 (0xda)
	// 27: :status: 404 (0xdb)
	staticIndices := []byte{0xd4, 0xd5, 0xd6, 0xd7, 0xd8, 0xd9, 0xda, 0xdb}
	staticExpected := []headerCall{
		{Name: ":method", Value: "POST"},
		{Name: ":method", Value: "PUT"},
		{Name: ":scheme", Value: "http"},
		{Name: ":scheme", Value: "https"},
		{Name: ":status", Value: "103"},
		{Name: ":status", Value: "200"},
		{Name: ":status", Value: "304"},
		{Name: ":status", Value: "404"},
	}
	for i, op := range staticIndices {
		block.WriteByte(op)
		expectedHeaders = append(expectedHeaders, staticExpected[i])
	}

	fullPayload := block.Bytes()
	require.Equal(t, 15, len(expectedHeaders))
	require.True(t, len(fullPayload) > 50)

	handler := newMockHeadersHandler()
	progDec := decoder.CreateProgressiveDecoder(1, handler)

	// Slice fullPayload into 20 fragmented chunks
	chunkSize := len(fullPayload) / 20
	for i := 0; i < len(fullPayload); i += chunkSize {
		end := min(i+chunkSize, len(fullPayload))
		progDec.Decode(fullPayload[i:end])
	}
	// Call EndHeaderBlock while still blocked on RIC=3
	progDec.EndHeaderBlock()

	// Handler must be completely unnotified because the stream is still waiting on dynamic inserts
	assert.False(t, handler.decodingCompleted)
	assert.False(t, handler.errorDetected)
	assert.Equal(t, 0, len(handler.headers))

	// Ingest dynamic table entries on encoder stream: 0, 1, 2
	insertEntry := func(k, v string) {
		decoder.EncoderStreamReceiver().Decode(encodeEncoderInsertWithoutNameRef(k, v))
	}
	insertEntry("d-key-0", "d-val-0") // count = 1
	assert.False(t, handler.decodingCompleted)

	insertEntry("d-key-1", "d-val-1") // count = 2
	assert.False(t, handler.decodingCompleted)

	// Insert entry 2 -> count = 3 == RIC threshold!
	insertEntry("d-key-2", "d-val-2")

	// Stream must now automatically unblock, drain all 20 chunks through instructionDecoder,
	// and finish decoding!
	assert.False(t, handler.errorDetected, fmt.Sprintf("unexpected error during drain: %s", handler.errorMessage))
	assert.True(t, handler.decodingCompleted)
	require.Equal(t, len(expectedHeaders), len(handler.headers))
	for i, exp := range expectedHeaders {
		assert.Equal(t, exp.Name, handler.headers[i].Name, fmt.Sprintf("header %d name mismatch", i))
		assert.Equal(t, exp.Value, handler.headers[i].Value, fmt.Sprintf("header %d value mismatch", i))
	}
}

// TestM5StreamDrainAdversarial_InterleavedChunkFeedingWhileBlocked
// Tests interleaved byte fragments fed across multiple concurrently blocked streams,
// ensuring byte boundaries inside varints and string literals are correctly preserved
// in each stream's buffer.
func TestM5StreamDrainAdversarial_InterleavedChunkFeedingWhileBlocked(t *testing.T) {
	sender := newMockStreamSenderDelegate()
	decoder := NewDecoder(2048, 5, nil)
	decoder.SetStreamSenderDelegate(sender)
	decoder.EncoderStreamReceiver().Decode(encodeEncoderCapacity(2048))

	// Stream A (id 1): RIC=1, Base=1, literal name ref (rel 0) with a multi-byte string
	var wireA bytes.Buffer
	wireA.Write(decodeHexOrPanic("0200400a"))
	wireA.WriteString("abcdefghij")
	bytesA := wireA.Bytes()

	// Stream B (id 2): RIC=2, Base=2, static indexed 17 (:method: GET), dynamic rel 0 (entry 1)
	bytesB := decodeHexOrPanic("0300d180")

	hA := newMockHeadersHandler()
	hB := newMockHeadersHandler()
	decA := decoder.CreateProgressiveDecoder(1, hA)
	decB := decoder.CreateProgressiveDecoder(2, hB)

	// Feed prefix of A and B
	decA.Decode(bytesA[:2]) // blocks on RIC=1
	decB.Decode(bytesB[:2]) // blocks on RIC=2

	// Interleave feeding 1-byte slices into A and B
	offsetA := 2
	offsetB := 2
	for offsetA < len(bytesA) || offsetB < len(bytesB) {
		if offsetA < len(bytesA) {
			decA.Decode(bytesA[offsetA : offsetA+1])
			offsetA++
		}
		if offsetB < len(bytesB) {
			decB.Decode(bytesB[offsetB : offsetB+1])
			offsetB++
		}
	}
	decA.EndHeaderBlock()
	decB.EndHeaderBlock()

	assert.False(t, hA.decodingCompleted)
	assert.False(t, hB.decodingCompleted)

	// Insert entry 0 ("streamA-key", "streamA-val")
	decoder.EncoderStreamReceiver().Decode(encodeEncoderInsertWithoutNameRef("streamA-key", "streamA-val"))
	assert.True(t, hA.decodingCompleted, "Stream A must complete on insert 1")
	assert.False(t, hB.decodingCompleted, "Stream B must remain blocked")
	require.Equal(t, 1, len(hA.headers))
	assert.Equal(t, "streamA-key", hA.headers[0].Name)
	assert.Equal(t, "abcdefghij", hA.headers[0].Value)

	// Insert entry 1 ("streamB-key", "streamB-val")
	decoder.EncoderStreamReceiver().Decode(encodeEncoderInsertWithoutNameRef("streamB-key", "streamB-val"))
	assert.True(t, hB.decodingCompleted, "Stream B must complete on insert 2")
	require.Equal(t, 2, len(hB.headers))
	assert.Equal(t, ":method", hB.headers[0].Name)
	assert.Equal(t, "GET", hB.headers[0].Value)
	assert.Equal(t, "streamB-key", hB.headers[1].Name)
	assert.Equal(t, "streamB-val", hB.headers[1].Value)
}

// -----------------------------------------------------------------------------
// Category 2: Decoding Completed and Section Acknowledgments
// -----------------------------------------------------------------------------

// TestM5StreamDrainAdversarial_SectionAck_RICGreaterThanZero
// Verifies OnDecodingCompleted generates SendHeaderAcknowledgement when RIC > 0,
// updates knownReceivedCount, and triggers SendInsertCountIncrement appropriately.
func TestM5StreamDrainAdversarial_SectionAck_RICGreaterThanZero(t *testing.T) {
	sender := newMockStreamSenderDelegate()
	decoder := NewDecoder(2048, 5, nil)
	decoder.SetStreamSenderDelegate(sender)
	decoder.EncoderStreamReceiver().Decode(encodeEncoderCapacity(2048))

	// Ingest 5 dynamic table entries on encoder stream
	for i := 0; i < 5; i++ {
		decoder.EncoderStreamReceiver().
			Decode(encodeEncoderInsertWithoutNameRef(fmt.Sprintf("k-%d", i), fmt.Sprintf("v-%d", i)))
	}
	require.Equal(t, uint64(5), decoder.HeaderTable().InsertedEntryCount())
	require.Equal(t, uint64(0), decoder.KnownReceivedCount())

	// Decode header block referencing dynamic entry 2 (RIC=3) on Stream 7
	// RIC=3 (encoded 4), Base=3 -> prefix 0400. Rel index 0 (Abs 2) -> 0x80
	h := newMockHeadersHandler()
	dec := decoder.CreateProgressiveDecoder(7, h)
	dec.Decode(decodeHexOrPanic("040080"))
	dec.EndHeaderBlock()

	assert.True(t, h.decodingCompleted)
	assert.False(t, h.errorDetected)

	decoder.FlushDecoderStream()

	// Feedback emitted on decoder stream:
	// 1. Header Acknowledgement for Stream 7: opcode 0x80 | 7 = 0x87
	// 2. knownReceivedCount became 3. InsertedEntryCount is 5.
	//    Delta = 5 - 3 = 2.
	//    SendInsertCountIncrement(2): opcode 0x00 | 2 = 0x02
	// Expected written wire bytes: [0x87, 0x02]
	var written []byte
	for _, w := range sender.writes {
		written = append(written, w...)
	}
	assert.Equal(t, decodeHexOrPanic("8702"), written)
	assert.Equal(t, uint64(5), decoder.KnownReceivedCount())
}

// TestM5StreamDrainAdversarial_SectionAck_RICZero_NoHeaderAck
// Verifies OnDecodingCompleted does NOT generate SendHeaderAcknowledgement when RIC == 0,
// but triggers SendInsertCountIncrement for all unacknowledged dynamic table entries.
func TestM5StreamDrainAdversarial_SectionAck_RICZero_NoHeaderAck(t *testing.T) {
	sender := newMockStreamSenderDelegate()
	decoder := NewDecoder(2048, 5, nil)
	decoder.SetStreamSenderDelegate(sender)
	decoder.EncoderStreamReceiver().Decode(encodeEncoderCapacity(2048))

	// Ingest 3 dynamic table entries on encoder stream
	for i := 0; i < 3; i++ {
		decoder.EncoderStreamReceiver().
			Decode(encodeEncoderInsertWithoutNameRef(fmt.Sprintf("k-%d", i), fmt.Sprintf("v-%d", i)))
	}
	require.Equal(t, uint64(3), decoder.HeaderTable().InsertedEntryCount())
	require.Equal(t, uint64(0), decoder.KnownReceivedCount())

	// Stream 12 decodes static-only headers (RIC=0)
	h := newMockHeadersHandler()
	dec := decoder.CreateProgressiveDecoder(12, h)
	dec.Decode(decodeHexOrPanic("0000d1")) // RIC=0, Base=0, :method: GET
	dec.EndHeaderBlock()

	assert.True(t, h.decodingCompleted)

	decoder.FlushDecoderStream()

	// Verification:
	// Header Acknowledgement MUST NOT be sent when RIC == 0 (no stream 12 ack)
	// But Insert Count Increment MUST be sent for the 3 unacknowledged dynamic entries:
	// opcode 0x00 | 3 = 0x03
	var written []byte
	for _, w := range sender.writes {
		written = append(written, w...)
	}
	assert.Equal(t, decodeHexOrPanic("03"), written)
	assert.Equal(t, uint64(3), decoder.KnownReceivedCount())
}

// TestM5StreamDrainAdversarial_SectionAck_OutOfOrderCompletions_MonotonicRIC
// Verifies knownReceivedCount monotonically advances when streams complete out of order.
func TestM5StreamDrainAdversarial_SectionAck_OutOfOrderCompletions_MonotonicRIC(t *testing.T) {
	sender := newMockStreamSenderDelegate()
	decoder := NewDecoder(2048, 5, nil)
	decoder.SetStreamSenderDelegate(sender)
	decoder.EncoderStreamReceiver().Decode(encodeEncoderCapacity(2048))

	for i := 0; i < 10; i++ {
		decoder.EncoderStreamReceiver().
			Decode(encodeEncoderInsertWithoutNameRef(fmt.Sprintf("k-%d", i), fmt.Sprintf("v-%d", i)))
	}
	require.Equal(t, uint64(10), decoder.HeaderTable().InsertedEntryCount())

	// Stream 1 completes with RIC = 8 (encoded 9, Base = 8)
	h1 := newMockHeadersHandler()
	d1 := decoder.CreateProgressiveDecoder(1, h1)
	d1.Decode(decodeHexOrPanic("090080")) // RIC=8, Abs 7
	d1.EndHeaderBlock()
	assert.True(t, h1.decodingCompleted)

	// knownReceivedCount is now 10 (8 from ack + 2 from increment)
	assert.Equal(t, uint64(10), decoder.KnownReceivedCount())

	decoder.FlushDecoderStream()
	// Clear sender
	sender.writes = nil

	// Stream 2 completes with lower RIC = 4 (encoded 5, Base = 4)
	h2 := newMockHeadersHandler()
	d2 := decoder.CreateProgressiveDecoder(2, h2)
	d2.Decode(decodeHexOrPanic("050080")) // RIC=4, Abs 3
	d2.EndHeaderBlock()
	assert.True(t, h2.decodingCompleted)

	decoder.FlushDecoderStream()

	// Header Acknowledgement for Stream 2: 0x80 | 2 = 0x82
	// knownReceivedCount remains 10 and NO increment is sent
	var written []byte
	for _, w := range sender.writes {
		written = append(written, w...)
	}
	assert.Equal(t, decodeHexOrPanic("82"), written)
	assert.Equal(t, uint64(10), decoder.KnownReceivedCount())
}

// TestM5StreamDrainAdversarial_SectionAck_LargeStreamIDAndIncrementVarints
// Verifies multi-byte varint generation on decoder stream when streamID > 127 and increment > 63.
func TestM5StreamDrainAdversarial_SectionAck_LargeStreamIDAndIncrementVarints(t *testing.T) {
	sender := newMockStreamSenderDelegate()
	decoder := NewDecoder(65536, 5, nil)
	decoder.SetStreamSenderDelegate(sender)
	decoder.EncoderStreamReceiver().Decode(encodeEncoderCapacity(65536))

	// Ingest 300 entries on encoder stream
	for i := 0; i < 300; i++ {
		decoder.EncoderStreamReceiver().Decode(encodeEncoderInsertWithoutNameRef(fmt.Sprintf("k%d", i), "v"))
	}
	require.Equal(t, uint64(300), decoder.HeaderTable().InsertedEntryCount())

	expectedHeaderAck := encodeVarintAdversarial(0x80, 7, 500)
	expectedIncrement := encodeVarintAdversarial(0x00, 6, 200)
	expectedWire := append(expectedHeaderAck, expectedIncrement...)

	h := newMockHeadersHandler()
	d := decoder.CreateProgressiveDecoder(500, h)
	prefix := encodeHeaderPrefixAdversarial(
		EncodeRequiredInsertCount(100, decoder.HeaderTable().MaxEntries()),
		false,
		0,
	)
	field := encodeVarintAdversarial(0x80, 6, 0)
	d.Decode(append(prefix, field...))
	d.EndHeaderBlock()

	assert.True(t, h.decodingCompleted)

	decoder.FlushDecoderStream()

	var written []byte
	for _, w := range sender.writes {
		written = append(written, w...)
	}
	assert.Equal(t, expectedWire, written)
}

// -----------------------------------------------------------------------------
// Category 3: Stream Reset and Cancellation
// -----------------------------------------------------------------------------

// TestM5StreamDrainAdversarial_StreamReset_CancellationEmissions
// Verifies OnStreamReset sends Stream Cancellation when max capacity > 0 and ignores when 0.
func TestM5StreamDrainAdversarial_StreamReset_CancellationEmissions(t *testing.T) {
	t.Run("MaxCapacityPositive_EmitsCancellation", func(t *testing.T) {
		sender := newMockStreamSenderDelegate()
		decoder := NewDecoder(1024, 5, nil)
		decoder.SetStreamSenderDelegate(sender)

		// Reset small stream ID: 5 -> opcode 0x40 | 5 = 0x45
		decoder.OnStreamReset(5)
		decoder.FlushDecoderStream()

		var written []byte
		for _, w := range sender.writes {
			written = append(written, w...)
		}
		assert.Equal(t, decodeHexOrPanic("45"), written)

		// Reset large stream ID: 250 -> 6-bit prefix 0x40 | 63 = 0x7f, 250-63=187
		sender.writes = nil
		decoder.OnStreamReset(250)
		decoder.FlushDecoderStream()

		expected := encodeVarintAdversarial(0x40, 6, 250)
		written = nil
		for _, w := range sender.writes {
			written = append(written, w...)
		}
		assert.Equal(t, expected, written)
	})

	t.Run("MaxCapacityZero_NoCancellation", func(t *testing.T) {
		sender := newMockStreamSenderDelegate()
		decoder := NewDecoder(0, 5, nil)
		decoder.SetStreamSenderDelegate(sender)

		decoder.OnStreamReset(5)
		decoder.FlushDecoderStream()
		assert.Equal(t, 0, len(sender.writes))
	})
}

// TestM5StreamDrainAdversarial_ProgressiveDecoderClose_UnregistersObserver
// Verifies that closing a blocked progressive decoder unregisters its observer,
// preventing dangling callbacks on dynamic table insertions.
func TestM5StreamDrainAdversarial_ProgressiveDecoderClose_UnregistersObserver(t *testing.T) {
	sender := newMockStreamSenderDelegate()
	decoder := NewDecoder(1024, 5, nil)
	decoder.SetStreamSenderDelegate(sender)
	decoder.EncoderStreamReceiver().Decode(encodeEncoderCapacity(1024))

	h := newMockHeadersHandler()
	p := decoder.CreateProgressiveDecoder(1, h)

	// Block stream on RIC=5
	p.Decode(decodeHexOrPanic("060080")) // RIC=5, Base=5
	assert.False(t, h.errorDetected)
	assert.False(t, h.decodingCompleted)

	// Close progressive decoder before threshold is reached
	p.Close()

	// Ingest 5 dynamic entries
	for i := 0; i < 5; i++ {
		decoder.EncoderStreamReceiver().Decode(encodeEncoderInsertWithoutNameRef(fmt.Sprintf("k-%d", i), "v"))
	}

	// Because decoder was closed, it should NOT have been unblocked or completed
	assert.False(t, h.decodingCompleted, "Closed decoder must not be triggered by dynamic table inserts")
}

// TestM5StreamDrainAdversarial_StreamReset_BlockedStreamStateCleanup
// RFC 9204 §2.1.2 & §2.2.2.2: A stream can also be unblocked if the stream is reset.
// When a blocked stream is reset, its blocked stream quota must be restored so subsequent
// streams are not erroneously rejected for exceeding maximumBlockedStreams.
func TestM5StreamDrainAdversarial_StreamReset_BlockedStreamStateCleanup(t *testing.T) {
	sender := newMockStreamSenderDelegate()
	// Allow exactly 1 blocked stream
	decoder := NewDecoder(1024, 1, nil)
	decoder.SetStreamSenderDelegate(sender)
	decoder.EncoderStreamReceiver().Decode(encodeEncoderCapacity(1024))

	// Stream 1 arrives and blocks on RIC=1 (empty dynamic table)
	h1 := newMockHeadersHandler()
	p1 := decoder.CreateProgressiveDecoder(1, h1)
	p1.Decode(decodeHexOrPanic("020080")) // RIC=1, Base=1
	assert.False(t, h1.errorDetected, "Stream 1 must be allowed to block within limit 1")
	assert.False(t, h1.decodingCompleted)

	// Stream 1 is reset by the connection
	decoder.OnStreamReset(1)
	p1.Close()
	decoder.FlushDecoderStream()

	// Stream 2 arrives and requires RIC=1. Since Stream 1 was reset and closed,
	// Stream 2 should now be permitted to block (active blocked streams count should be 1 <= 1).
	h2 := newMockHeadersHandler()
	p2 := decoder.CreateProgressiveDecoder(2, h2)
	p2.Decode(decodeHexOrPanic("020080"))

	assert.False(
		t,
		h2.errorDetected,
		fmt.Sprintf(
			"Stream 2 must not fail with blocked streams limit after Stream 1 reset; error: %s",
			h2.errorMessage,
		),
	)
	assert.False(t, h2.decodingCompleted)

	// Ingest dynamic table entry to verify Stream 2 unblocks successfully
	decoder.EncoderStreamReceiver().Decode(encodeEncoderInsertWithoutNameRef("k2", "v2"))
	p2.EndHeaderBlock()
	assert.True(t, h2.decodingCompleted)
}

// -----------------------------------------------------------------------------
// Category 4: Race Conditions and Concurrent Stream Decoding (`go test -race`)
// -----------------------------------------------------------------------------

// TestM5StreamDrainAdversarial_Concurrent_SharedDecoderPipeline
// Stress tests 32 concurrent goroutines operating against a single Decoder
// instance protected by a synchronization mutex (simulating connection strand / event loop),
// interleaving progressive decoders, chunk decoding, dynamic table insertions, and resets.
func TestM5StreamDrainAdversarial_Concurrent_SharedDecoderPipeline(t *testing.T) {
	sender := newThreadSafeStreamSenderDelegate()
	var mu sync.Mutex
	decoder := NewDecoder(8192, 50, nil)
	decoder.SetStreamSenderDelegate(sender)

	// Set dynamic table capacity
	mu.Lock()
	decoder.EncoderStreamReceiver().Decode(encodeEncoderCapacity(8192))
	mu.Unlock()

	const numWorkers = 20
	const streamsPerWorker = 10
	var wg sync.WaitGroup

	// Background inserter goroutine inserting dynamic entries periodically
	stopCh := make(chan struct{})
	var inserterWg sync.WaitGroup
	inserterWg.Add(2)
	for ins := 0; ins < 2; ins++ {
		go func(insID int) {
			defer inserterWg.Done()
			cnt := 0
			for {
				select {
				case <-stopCh:
					return
				default:
					mu.Lock()
					wire := encodeEncoderInsertWithoutNameRef(
						fmt.Sprintf("dyn-%d-%d", insID, cnt),
						fmt.Sprintf("val-%d", cnt),
					)
					decoder.EncoderStreamReceiver().Decode(wire)
					mu.Unlock()
					cnt++
				}
			}
		}(ins)
	}

	// 20 decoder workers
	wg.Add(numWorkers)
	for w := 0; w < numWorkers; w++ {
		go func(workerID int) {
			defer wg.Done()
			rng := rand.New(rand.NewSource(int64(workerID * 777)))

			for s := 0; s < streamsPerWorker; s++ {
				streamID := uint64(workerID*1000 + s + 1)
				handler := newThreadSafeHeadersHandler()

				mu.Lock()
				progDec := decoder.CreateProgressiveDecoder(streamID, handler)
				mu.Unlock()

				// Randomly either decode static or dynamic
				if rng.Intn(2) == 0 {
					// Static header block
					wire := decodeHexOrPanic("0000d1")
					mu.Lock()
					progDec.Decode(wire)
					progDec.EndHeaderBlock()
					mu.Unlock()

					assert.True(t, handler.IsCompleted())
					assert.False(t, handler.HasError())
				} else {
					// Stream reset simulation
					mu.Lock()
					decoder.OnStreamReset(streamID)
					progDec.Close()
					mu.Unlock()
				}
			}
		}(w)
	}

	wg.Wait()
	close(stopCh)
	inserterWg.Wait()

	mu.Lock()
	decoder.FlushDecoderStream()
	mu.Unlock()

	assert.True(t, len(sender.WrittenData()) > 0)
}

// TestM5StreamDrainAdversarial_Concurrent_MultiplexedInterleavedStreams
// Simulates a single QUIC connection multiplexing 50 concurrent streams chunk-by-chunk
// in pseudo-random interleaved order on the same Decoder.
func TestM5StreamDrainAdversarial_Concurrent_MultiplexedInterleavedStreams(t *testing.T) {
	sender := newMockStreamSenderDelegate()
	decoder := NewDecoder(4096, 50, nil)
	decoder.SetStreamSenderDelegate(sender)
	decoder.EncoderStreamReceiver().Decode(encodeEncoderCapacity(4096))

	// Pre-populate dynamic table with 5 entries
	for i := 0; i < 5; i++ {
		decoder.EncoderStreamReceiver().
			Decode(encodeEncoderInsertWithoutNameRef(fmt.Sprintf("common-k-%d", i), fmt.Sprintf("common-v-%d", i)))
	}
	require.Equal(t, uint64(5), decoder.HeaderTable().InsertedEntryCount())

	const streamCount = 50
	type multiplexedStream struct {
		id       uint64
		decoder  *ProgressiveDecoder
		handler  *mockHeadersHandler
		wire     []byte
		offset   int
		complete bool
	}

	streams := make([]*multiplexedStream, streamCount)
	for i := 0; i < streamCount; i++ {
		h := newMockHeadersHandler()
		sid := uint64(i + 1)
		d := decoder.CreateProgressiveDecoder(sid, h)

		// Wire: RIC=5 (encoded 6), Base=5 -> 0600
		// Field 1: Static index 17 (:method: GET) -> 0xd1
		// Field 2: Dynamic rel index 0 (Abs 4) -> 0x80
		// Field 3: Plain literal
		var block bytes.Buffer
		block.Write(decodeHexOrPanic("0600d180"))
		block.Write(encodeLiteralFieldWithoutRefAdversarial(fmt.Sprintf("stream-%d", sid), "val"))

		streams[i] = &multiplexedStream{
			id:      sid,
			decoder: d,
			handler: h,
			wire:    block.Bytes(),
		}
	}

	// Interleave feeding 1-to-3 bytes across all 50 streams until all finished
	rng := rand.New(rand.NewSource(42))
	allFinished := false
	for !allFinished {
		allFinished = true
		// Shuffle stream order each round
		perm := rng.Perm(streamCount)
		for _, idx := range perm {
			s := streams[idx]
			if s.offset < len(s.wire) {
				allFinished = false
				chunkSize := rng.Intn(3) + 1
				end := min(s.offset+chunkSize, len(s.wire))
				s.decoder.Decode(s.wire[s.offset:end])
				s.offset = end
				if s.offset == len(s.wire) {
					s.decoder.EndHeaderBlock()
					s.complete = true
				}
			}
		}
	}

	// Verify all 50 streams completed deterministically with zero error
	for _, s := range streams {
		assert.False(t, s.handler.errorDetected, fmt.Sprintf("stream %d had error: %s", s.id, s.handler.errorMessage))
		assert.True(t, s.handler.decodingCompleted, fmt.Sprintf("stream %d should be completed", s.id))
		require.Equal(t, 3, len(s.handler.headers), fmt.Sprintf("stream %d should have 3 headers", s.id))
		assert.Equal(t, ":method", s.handler.headers[0].Name)
		assert.Equal(t, "GET", s.handler.headers[0].Value)
		assert.Equal(t, "common-k-4", s.handler.headers[1].Name)
		assert.Equal(t, "common-v-4", s.handler.headers[1].Value)
		assert.Equal(t, fmt.Sprintf("stream-%d", s.id), s.handler.headers[2].Name)
		assert.Equal(t, "val", s.handler.headers[2].Value)
	}
}

// TestM5StreamDrainAdversarial_Concurrent_IndependentDecoderInstances
// Verifies 16 parallel goroutines running completely independent Decoder instances
// under go test -race to ensure package globals are thread-safe.
func TestM5StreamDrainAdversarial_Concurrent_IndependentDecoderInstances(t *testing.T) {
	const goroutines = 16
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for g := 0; g < goroutines; g++ {
		go func(gid int) {
			defer wg.Done()
			sender := newMockStreamSenderDelegate()
			dec := NewDecoder(2048, 10, nil)
			dec.SetStreamSenderDelegate(sender)
			dec.EncoderStreamReceiver().Decode(encodeEncoderCapacity(2048))

			rng := rand.New(rand.NewSource(int64(gid * 999)))

			for step := 0; step < 20; step++ {
				// Insert entry
				dec.EncoderStreamReceiver().Decode(encodeEncoderInsertWithoutNameRef(
					fmt.Sprintf("g%d-k%d", gid, step), "v",
				))

				sid := uint64(step + 1)
				h := newMockHeadersHandler()
				p := dec.CreateProgressiveDecoder(sid, h)

				if rng.Intn(2) == 0 {
					p.Decode(decodeHexOrPanic("0000d1")) // static :method GET
					p.EndHeaderBlock()
					assert.True(t, h.decodingCompleted)
				} else {
					dec.OnStreamReset(sid)
					p.Close()
				}
			}
			dec.FlushDecoderStream()
		}(g)
	}

	wg.Wait()
}
