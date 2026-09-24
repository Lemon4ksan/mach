// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package qpack

import (
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"testing"

	"github.com/lemon4ksan/foundation/testing/assert"
	"github.com/lemon4ksan/foundation/testing/require"
)

// -----------------------------------------------------------------------------
// Top-Level Entrypoint for Milestone 6 Adversarial Challenge
// -----------------------------------------------------------------------------

func TestM6Adversarial(t *testing.T) {
	t.Run("ArbitraryChunkSlicingAndPacketJitter", TestM6Adversarial_ArbitraryChunkSlicingAndPacketJitter)
	t.Run("OutOfOrderEvictionAndCapacityShrink", TestM6Adversarial_OutOfOrderEvictionAndCapacityShrink)
	t.Run("StreamCancellationStormRaces", TestM6Adversarial_StreamCancellationStormRaces)
	t.Run("PseudoRandomFuzzHeaders", TestM6Adversarial_PseudoRandomFuzzHeaders)
}

// -----------------------------------------------------------------------------
// Adversarial Chaos Helpers
// -----------------------------------------------------------------------------

// chaosDeliverInterleaved delivers encoder stream, request stream block, and decoder feedback
// in interleaved random chunks (sizes between minChunk and maxChunk) to stress streaming state machines.
func chaosDeliverInterleaved(
	t *testing.T,
	h *roundtripHarness,
	streamID uint64,
	block []byte,
	rng *rand.Rand,
	minChunk, maxChunk int,
) *roundtripHeadersHandler {
	t.Helper()
	handler := newRoundtripHeadersHandler()

	h.mu.Lock()
	pDec := h.decoder.CreateProgressiveDecoder(streamID, handler)
	h.mu.Unlock()

	blockRemaining := block
	for len(blockRemaining) > 0 || len(h.encStreamBuffer) > 0 {
		// Randomly choose action: 0 = feed request block chunk, 1 = deliver encoder stream chunk
		action := rng.Intn(2)
		if len(blockRemaining) == 0 {
			action = 1
		} else if len(h.encStreamBuffer) == 0 {
			action = 0
		}

		chunkSize := minChunk
		if maxChunk > minChunk {
			chunkSize += rng.Intn(maxChunk - minChunk + 1)
		}

		if action == 0 && len(blockRemaining) > 0 {
			toTake := chunkSize
			if toTake > len(blockRemaining) {
				toTake = len(blockRemaining)
			}
			chunk := blockRemaining[:toTake]
			blockRemaining = blockRemaining[toTake:]

			h.mu.Lock()
			pDec.Decode(chunk)
			h.mu.Unlock()
		} else if action == 1 && len(h.encStreamBuffer) > 0 {
			h.DeliverEncoderStream(chunkSize)
		}
	}

	h.mu.Lock()
	pDec.EndHeaderBlock()
	h.mu.Unlock()

	// Flush any leftover encoder stream instructions and decoder feedback
	h.DeliverEncoderStream(0)
	h.FlushDecoderStream()

	// Deliver decoder stream in random chunks as well
	for len(h.decStreamBuffer) > 0 {
		chunkSize := minChunk
		if maxChunk > minChunk {
			chunkSize += rng.Intn(maxChunk - minChunk + 1)
		}
		h.DeliverDecoderStream(chunkSize)
	}

	return handler
}

// assertHeaderListsEqual validates header field count, names, and values with detailed failure diagnostics.
func assertHeaderListsEqual(t *testing.T, expected, actual []HeaderField, contextMsg string) {
	t.Helper()
	require.Equalf(
		t,
		len(expected),
		len(actual),
		"%s: header count mismatch (expected %d, got %d)",
		contextMsg,
		len(expected),
		len(actual),
	)
	for i := range expected {
		assert.Equalf(t, expected[i].Name, actual[i].Name, "%s: header name mismatch at index %d", contextMsg, i)
		assert.Equalf(t, expected[i].Value, actual[i].Value, "%s: header value mismatch at index %d", contextMsg, i)
	}
}

// -----------------------------------------------------------------------------
// Stress Test 1: Arbitrary Chunk Slicing & Packet Jitter
// -----------------------------------------------------------------------------

func TestM6Adversarial_ArbitraryChunkSlicingAndPacketJitter(t *testing.T) {
	t.Run("SingleByteDelivery_ComprehensiveLifecycle", func(t *testing.T) {
		t.Parallel()
		h := newRoundtripHarness(t, 2048, 10, modeManual)

		streamsData := [][]HeaderField{
			{
				{Name: ":method", Value: "GET"},
				{Name: ":scheme", Value: "https"},
				{Name: ":authority", Value: "chaos.stream.io"},
				{Name: ":path", Value: "/byte-by-byte/0"},
				{Name: "user-agent", Value: "AdversarialTester/2.0"},
				{Name: "x-custom-metric", Value: "metric-alpha-001"},
			},
			{
				{Name: ":method", Value: "POST"},
				{Name: ":scheme", Value: "https"},
				{Name: ":authority", Value: "chaos.stream.io"},
				{Name: ":path", Value: "/byte-by-byte/1"},
				{Name: "content-type", Value: "application/json; charset=utf-8"},
				{Name: "user-agent", Value: "AdversarialTester/2.0"},
				{Name: "x-custom-metric", Value: "metric-beta-002"},
				{Name: "x-payload-signature", Value: "sig_9876543210_abcdef"},
			},
			{
				{Name: ":method", Value: "GET"},
				{Name: ":scheme", Value: "https"},
				{Name: ":authority", Value: "chaos.stream.io"},
				{Name: ":path", Value: "/byte-by-byte/2"},
				{Name: "accept", Value: "text/html,application/xhtml+xml"},
				{Name: "x-custom-metric", Value: "metric-gamma-003"},
			},
		}

		for streamIdx, fields := range streamsData {
			streamID := uint64(streamIdx*2 + 1)
			block := h.Encode(streamID, fields)

			// 1. Deliver encoder stream strictly 1 byte at a time
			for len(h.encStreamBuffer) > 0 {
				delivered := h.DeliverEncoderStream(1)
				require.Equal(t, 1, delivered, "single byte delivery must deliver 1 byte")
			}

			// 2. Deliver request stream header block strictly 1 byte at a time
			handler := newRoundtripHeadersHandler()
			pDec := h.decoder.CreateProgressiveDecoder(streamID, handler)
			for len(block) > 0 {
				pDec.Decode(block[:1])
				block = block[1:]
			}
			pDec.EndHeaderBlock()

			require.Truef(t, handler.Completed(), "stream %d must be completed", streamID)
			require.Falsef(t, handler.HasError(), "stream %d must have no error", streamID)
			assertHeaderListsEqual(t, fields, handler.Headers(), fmt.Sprintf("stream %d", streamID))

			// 3. Deliver decoder feedback strictly 1 byte at a time
			h.FlushDecoderStream()
			for len(h.decStreamBuffer) > 0 {
				delivered := h.DeliverDecoderStream(1)
				require.Equal(t, 1, delivered, "single byte feedback delivery must deliver 1 byte")
			}
		}

		assert.Equal(t, h.encoder.HeaderTable().InsertedEntryCount(), h.decoder.HeaderTable().InsertedEntryCount())
		assert.Equal(t, h.encoder.BlockingManager().KnownReceivedCount(), h.decoder.KnownReceivedCount())
	})

	t.Run("RandomChunkSlicing_InterleavedJitter", func(t *testing.T) {
		t.Parallel()
		h := newRoundtripHarness(t, 4096, 20, modeManual)
		rng := rand.New(rand.NewSource(123456789))

		const numStreams = 25
		for i := 0; i < numStreams; i++ {
			streamID := uint64(100 + i*2)
			fields := []HeaderField{
				{Name: ":method", Value: "GET"},
				{Name: ":scheme", Value: "https"},
				{Name: ":authority", Value: fmt.Sprintf("host-%d.jitter.net", i%5)},
				{Name: ":path", Value: fmt.Sprintf("/resource/jitter/%d?filter=all&page=%d", i, i%10)},
				{Name: "user-agent", Value: "AdversarialJitterRunner/3.0"},
				{
					Name:  fmt.Sprintf("x-jitter-%d", i%7),
					Value: fmt.Sprintf("jitter-token-payload-%d-%d", i, rng.Int63()),
				},
				{Name: "x-ephemeral", Value: fmt.Sprintf("ephemeral-%d", i)},
			}

			block := h.Encode(streamID, fields)

			// Interleave random chunk deliveries: chunks vary from 1 to 9 bytes
			handler := chaosDeliverInterleaved(t, h, streamID, block, rng, 1, 9)

			require.Truef(t, handler.Completed(), "stream %d must complete under interleaved jitter", streamID)
			require.Falsef(
				t,
				handler.HasError(),
				"stream %d must have no decoding error: %s",
				streamID,
				handler.errorMessage,
			)
			assertHeaderListsEqual(t, fields, handler.Headers(), fmt.Sprintf("stream %d", streamID))
		}

		assert.Equal(t, h.encoder.HeaderTable().InsertedEntryCount(), h.decoder.HeaderTable().InsertedEntryCount())
		assert.Equal(t, h.encoder.BlockingManager().KnownReceivedCount(), h.decoder.KnownReceivedCount())
	})

	t.Run("ControlStream_ByteFragmentedOpcodesAndVarints", func(t *testing.T) {
		t.Parallel()
		h := newRoundtripHarness(t, 2048, 10, modeManual)

		// Create a dynamic header with a multi-byte varint length and large value to force
		// instruction decoder state machine across byte boundaries
		largeValue := strings.Repeat("A", 350)
		fields := []HeaderField{
			{Name: "x-large-chunked-header", Value: largeValue},
		}

		block := h.Encode(77, fields)

		// Deliver encoder stream with primed fragmentation: 1 byte, 2 bytes, 1 byte, 3 bytes...
		slicePlan := []int{1, 1, 2, 1, 3, 1, 2, 1, 1, 4, 1}
		planIdx := 0
		for len(h.encStreamBuffer) > 0 {
			take := slicePlan[planIdx%len(slicePlan)]
			planIdx++
			h.DeliverEncoderStream(take)
		}

		// Decode header block with prime fragmentation
		handler := newRoundtripHeadersHandler()
		pDec := h.decoder.CreateProgressiveDecoder(77, handler)
		planIdx = 0
		for len(block) > 0 {
			take := slicePlan[planIdx%len(slicePlan)]
			planIdx++
			if take > len(block) {
				take = len(block)
			}
			pDec.Decode(block[:take])
			block = block[take:]
		}
		pDec.EndHeaderBlock()

		require.True(t, handler.Completed())
		require.False(t, handler.HasError())
		assertHeaderListsEqual(t, fields, handler.Headers(), "stream 77")

		// Deliver decoder feedback with single bytes
		h.FlushDecoderStream()
		for len(h.decStreamBuffer) > 0 {
			h.DeliverDecoderStream(1)
		}

		assert.Equal(t, h.encoder.HeaderTable().InsertedEntryCount(), h.decoder.HeaderTable().InsertedEntryCount())
	})
}

// -----------------------------------------------------------------------------
// Stress Test 2: Out-Of-Order Stream Deliveries, Eviction & Capacity Shrinking
// -----------------------------------------------------------------------------

func TestM6Adversarial_OutOfOrderEvictionAndCapacityShrink(t *testing.T) {
	t.Run("DeepOutOfOrder_DynamicTableEviction", func(t *testing.T) {
		t.Parallel()
		// Small dynamic table (250 bytes) fits ~3 entries (each entry ~70 bytes)
		h := newRoundtripHarness(t, 250, 10, modeManual)

		// Step 1: Prime table with 3 acknowledged streams (entries 0, 1, 2)
		for i := 1; i <= 3; i++ {
			streamID := uint64(i)
			fields := []HeaderField{{Name: fmt.Sprintf("x-prime-%d", i), Value: fmt.Sprintf("prime-val-%d", i)}}
			block := h.Encode(streamID, fields)
			h.DeliverEncoderStream(0)
			handler := h.DecodeBlock(streamID, block)
			require.True(t, handler.Completed())
			h.DeliverDecoderStream(0) // Ack delivered, so entries are now evictable
		}
		require.Equal(t, uint64(0), h.encoder.HeaderTable().DroppedEntryCount())

		// Step 2: Encode 3 subsequent streams (Stream 10, 20, 30) inserting new entries
		// Since earlier entries are acknowledged, the encoder evicts entries 0, 1, 2!
		type streamPayload struct {
			streamID uint64
			fields   []HeaderField
			block    []byte
		}
		numBurst := 3
		burstPayloads := make([]streamPayload, numBurst)
		for i := 0; i < numBurst; i++ {
			streamID := uint64(10 + i*10)
			fields := []HeaderField{{Name: fmt.Sprintf("x-burst-%d", i), Value: fmt.Sprintf("burst-val-%d-payload", i)}}
			block := h.Encode(streamID, fields)
			burstPayloads[i] = streamPayload{streamID: streamID, fields: fields, block: block}
		}

		// Encoder has now evicted older entries
		require.True(
			t,
			h.encoder.HeaderTable().DroppedEntryCount() > 0,
			"encoder must have evicted entries under tight capacity",
		)

		// Step 3: Deliver burst header blocks in REVERSE order (Stream 30, then 20, then 10)
		handlers := make(map[uint64]*roundtripHeadersHandler)
		for i := numBurst - 1; i >= 0; i-- {
			p := burstPayloads[i]
			handler := newRoundtripHeadersHandler()
			pDec := h.decoder.CreateProgressiveDecoder(p.streamID, handler)
			pDec.Decode(p.block)
			pDec.EndHeaderBlock()
			handlers[p.streamID] = handler

			// All streams must be blocked waiting on encoder stream
			assert.Falsef(t, handler.Completed(), "stream %d should be blocked awaiting its RIC", p.streamID)
		}

		// Step 4: Deliver encoder stream in 12-byte slices
		for len(h.encStreamBuffer) > 0 {
			h.DeliverEncoderStream(12)
		}

		// All burst streams must unblock and complete successfully
		for i := 0; i < numBurst; i++ {
			p := burstPayloads[i]
			handler := handlers[p.streamID]
			require.Truef(t, handler.Completed(), "stream %d must have unblocked and completed", p.streamID)
			require.Falsef(
				t,
				handler.HasError(),
				"stream %d must have no decoding error: %s",
				p.streamID,
				handler.errorMessage,
			)
			assertHeaderListsEqual(t, p.fields, handler.Headers(), fmt.Sprintf("stream %d", p.streamID))
		}

		// Step 5: Deliver decoder feedback to encoder
		h.DeliverDecoderStream(0)
		assert.Equal(t, h.encoder.HeaderTable().DroppedEntryCount(), h.decoder.HeaderTable().DroppedEntryCount())
		assert.Equal(t, h.encoder.HeaderTable().InsertedEntryCount(), h.decoder.HeaderTable().InsertedEntryCount())
	})

	t.Run("CapacityShrink_MidFlight_WithEviction", func(t *testing.T) {
		t.Parallel()
		// Start with 1500 bytes capacity
		h := newRoundtripHarness(t, 1500, 10, modeManual)

		// Stream 1: inserts 2 dynamic entries (~160 bytes) and gets acknowledged
		fields1 := []HeaderField{
			{Name: "x-phase1-a", Value: "phase1-value-alpha"},
			{Name: "x-phase1-b", Value: "phase1-value-bravo"},
		}
		block1 := h.Encode(1, fields1)
		h.DeliverEncoderStream(0)
		handler1 := h.DecodeBlock(1, block1)
		require.True(t, handler1.Completed())
		h.DeliverDecoderStream(0)

		// Stream 2: inserts 2 dynamic entries (~160 bytes) and gets acknowledged
		fields2 := []HeaderField{
			{Name: "x-phase2-a", Value: "phase2-value-delta"},
			{Name: "x-phase2-b", Value: "phase2-value-echo"},
		}
		block2 := h.Encode(2, fields2)
		h.DeliverEncoderStream(0)
		handler2 := h.DecodeBlock(2, block2)
		require.True(t, handler2.Completed())
		h.DeliverDecoderStream(0)

		// Shrink dynamic table capacity down to 100 bytes!
		// This causes encoder to evict entries and emit SetDynamicTableCapacity instruction on encoder stream
		h.encoder.SetDynamicTableCapacity(100)
		h.FlushEncoderStream()

		require.Equal(t, uint64(100), h.encoder.HeaderTable().DynamicTableCapacity())
		require.True(t, h.encoder.HeaderTable().DroppedEntryCount() > 0, "capacity shrink must drop entries")

		// Stream 3: encoded under shrunk capacity (small enough to insert in 100-byte table: ~62 bytes)
		fields3 := []HeaderField{
			{Name: "x-phase3-a", Value: "phase3-val"},
		}
		block3 := h.Encode(3, fields3)

		// Deliver Stream 3 header block to decoder BEFORE delivering encoder stream
		handler3 := newRoundtripHeadersHandler()
		pDec3 := h.decoder.CreateProgressiveDecoder(3, handler3)
		pDec3.Decode(block3)
		pDec3.EndHeaderBlock()
		assert.False(t, handler3.Completed(), "stream 3 must block waiting for encoder stream")

		// Deliver encoder stream in arbitrary slices (contains Capacity update and Stream 3 insert)
		for len(h.encStreamBuffer) > 0 {
			h.DeliverEncoderStream(17)
		}

		// Verify Stream 3 completed successfully
		require.True(t, handler3.Completed(), "stream 3 must complete")
		require.False(t, handler3.HasError(), "stream 3 must not error")
		assertHeaderListsEqual(t, fields3, handler3.Headers(), "stream 3")

		// Verify decoder table matches encoder table capacity and drop count
		assert.Equal(t, uint64(100), h.decoder.HeaderTable().DynamicTableCapacity())
		assert.Equal(t, h.encoder.HeaderTable().DroppedEntryCount(), h.decoder.HeaderTable().DroppedEntryCount())
	})

	t.Run("CapacityShrinkToZero_AndRegrow", func(t *testing.T) {
		t.Parallel()
		h := newRoundtripHarness(t, 2048, 10, modeManual)

		// Step 1: Normal dynamic inserts
		fields1 := []HeaderField{{Name: "x-init-token", Value: "token-value-12345"}}
		block1 := h.Encode(10, fields1)
		h.DeliverEncoderStream(0)
		handler1 := h.DecodeBlock(10, block1)
		require.True(t, handler1.Completed())
		assertHeaderListsEqual(t, fields1, handler1.Headers(), "stream 10")

		// Step 2: Shrink to ZERO (disables dynamic table, evicts everything)
		h.encoder.SetDynamicTableCapacity(0)
		h.FlushEncoderStream()
		h.DeliverEncoderStream(0)

		assert.Equal(t, uint64(0), h.encoder.HeaderTable().DynamicTableCapacity())
		assert.Equal(t, uint64(0), h.decoder.HeaderTable().DynamicTableCapacity())

		// Step 3: Encode stream under 0 capacity (must encode purely via literals / static table)
		fields2 := []HeaderField{
			{Name: ":method", Value: "POST"},
			{Name: ":path", Value: "/zero-capacity/submit"},
			{Name: "x-literal-header", Value: "literal-only-value"},
		}
		block2 := h.Encode(20, fields2)
		handler2 := h.DecodeBlock(20, block2)
		require.True(t, handler2.Completed())
		assertHeaderListsEqual(t, fields2, handler2.Headers(), "stream 20")

		// Step 4: Regrow capacity to 2048 (allowed since maxCapacity is 2048)
		h.encoder.SetDynamicTableCapacity(2048)
		h.FlushEncoderStream()
		h.DeliverEncoderStream(0)

		assert.Equal(t, uint64(2048), h.encoder.HeaderTable().DynamicTableCapacity())
		assert.Equal(t, uint64(2048), h.decoder.HeaderTable().DynamicTableCapacity())

		// Step 5: Dynamic inserts resume
		fields3 := []HeaderField{{Name: "x-regrown-token", Value: "token-value-67890"}}
		block3 := h.Encode(30, fields3)
		h.DeliverEncoderStream(0)
		handler3 := h.DecodeBlock(30, block3)
		require.True(t, handler3.Completed())
		assertHeaderListsEqual(t, fields3, handler3.Headers(), "stream 30")
	})
}

// -----------------------------------------------------------------------------
// Stress Test 3: Stream Cancellation Storms Racing Against Feedback & Acks
// -----------------------------------------------------------------------------

func TestM6Adversarial_StreamCancellationStormRaces(t *testing.T) {
	t.Run("ConcurrentCancellationStorm_MultiWorker", func(t *testing.T) {
		t.Parallel()
		h := newRoundtripHarness(t, 8192, 100, modeImmediate)

		const numWorkers = 16
		const streamsPerWorker = 15
		var wg sync.WaitGroup
		wg.Add(numWorkers)

		for w := 0; w < numWorkers; w++ {
			go func(workerID int) {
				defer wg.Done()
				workerRng := rand.New(rand.NewSource(int64(workerID*777 + 999)))

				for s := 0; s < streamsPerWorker; s++ {
					streamID := uint64(workerID*1000 + s*4 + 1)
					fields := []HeaderField{
						{Name: ":method", Value: "GET"},
						{Name: ":path", Value: fmt.Sprintf("/worker/%d/storm/%d", workerID, s)},
						{Name: "x-worker-token", Value: fmt.Sprintf("token-%d-%d", workerID, s)},
						{Name: "x-storm-nonce", Value: fmt.Sprintf("nonce-%d", workerRng.Int63())},
					}

					fate := workerRng.Intn(100)
					if fate < 40 {
						// Fate A: Normal complete roundtrip
						h.Roundtrip(streamID, fields)
					} else if fate < 70 {
						// Fate B: Immediate cancellation before decoding
						h.Encode(streamID, fields)
						h.mu.Lock()
						h.decoder.OnStreamReset(streamID)
						h.decoder.FlushDecoderStream()
						h.mu.Unlock()
					} else {
						// Fate C: Partial progressive decode followed by stream reset and close
						block := h.Encode(streamID, fields)
						handler := newRoundtripHeadersHandler()

						h.mu.Lock()
						pDec := h.decoder.CreateProgressiveDecoder(streamID, handler)
						if len(block) > 2 {
							pDec.Decode(block[:len(block)/2])
						}
						h.decoder.OnStreamReset(streamID)
						pDec.Close()
						h.decoder.FlushDecoderStream()
						h.mu.Unlock()
					}
				}
			}(w)
		}

		wg.Wait()

		// Verify state integrity after storm
		assert.Equal(t, h.encoder.HeaderTable().InsertedEntryCount(), h.decoder.HeaderTable().InsertedEntryCount())
	})

	t.Run("BlockedStreamsCancellation_UnderQuotaPressure", func(t *testing.T) {
		t.Parallel()
		// Maximum blocked streams = 3
		h := newRoundtripHarness(t, 2048, 3, modeManual)

		// Encode 3 streams with dynamic entries (fill quota completely)
		block1 := h.Encode(101, []HeaderField{{Name: "x-block-1", Value: "val-1"}})
		block2 := h.Encode(102, []HeaderField{{Name: "x-block-2", Value: "val-2"}})
		block3 := h.Encode(103, []HeaderField{{Name: "x-block-3", Value: "val-3"}})

		pDec1 := h.decoder.CreateProgressiveDecoder(101, newRoundtripHeadersHandler())
		pDec1.Decode(block1)
		pDec1.EndHeaderBlock()

		pDec2 := h.decoder.CreateProgressiveDecoder(102, newRoundtripHeadersHandler())
		pDec2.Decode(block2)
		pDec2.EndHeaderBlock()

		pDec3 := h.decoder.CreateProgressiveDecoder(103, newRoundtripHeadersHandler())
		pDec3.Decode(block3)
		pDec3.EndHeaderBlock()

		// Currently 3 streams are blocked (quota exhausted)
		require.Equal(t, 3, len(h.decoder.blockedStreams))

		// Cancel stream 101 and 102
		pDec1.Close()
		h.decoder.OnStreamReset(101)

		pDec2.Close()
		h.decoder.OnStreamReset(102)

		require.Equal(t, 1, len(h.decoder.blockedStreams))

		// Deliver decoder feedback to encoder
		h.DeliverDecoderStream(0)
		assert.False(t, h.encoder.BlockingManager().IsBlocked(101))
		assert.False(t, h.encoder.BlockingManager().IsBlocked(102))

		// Now stream 104 and 105 can block without violating the limit of 3
		block4 := h.Encode(104, []HeaderField{{Name: "x-block-4", Value: "val-4"}})
		block5 := h.Encode(105, []HeaderField{{Name: "x-block-5", Value: "val-5"}})

		handler4 := newRoundtripHeadersHandler()
		pDec4 := h.decoder.CreateProgressiveDecoder(104, handler4)
		pDec4.Decode(block4)
		pDec4.EndHeaderBlock()

		handler5 := newRoundtripHeadersHandler()
		pDec5 := h.decoder.CreateProgressiveDecoder(105, handler5)
		pDec5.Decode(block5)
		pDec5.EndHeaderBlock()

		require.Equal(t, 3, len(h.decoder.blockedStreams))

		// Deliver all encoder stream insertions
		h.DeliverEncoderStream(0)

		// Surviving streams unblock and complete successfully
		require.True(t, handler4.Completed())
		require.True(t, handler5.Completed())
		require.False(t, handler4.HasError())
		require.False(t, handler5.HasError())
	})

	t.Run("ResetRacingWithSectionAck_DoubleFeedback", func(t *testing.T) {
		t.Parallel()
		h := newRoundtripHarness(t, 2048, 5, modeManual)

		fields := []HeaderField{{Name: "x-race-ack", Value: "race-ack-value"}}
		block := h.Encode(201, fields)
		h.DeliverEncoderStream(0)

		// 1. Decode stream completely -> generates SectionAck
		handler := h.DecodeBlock(201, block)
		require.True(t, handler.Completed())

		// 2. Client immediately resets stream -> generates StreamCancellation
		h.decoder.OnStreamReset(201)

		// 3. Deliver decoder feedback: SectionAck followed by StreamCancellation
		h.DeliverDecoderStream(0)

		// 4. Verify encoder gracefully handled both without error or state corruption
		assert.False(t, h.encoder.BlockingManager().IsBlocked(201))

		// 5. Subsequent stream succeeds normally
		h.Roundtrip(202, []HeaderField{{Name: "x-after-race", Value: "ok"}})
	})
}

// -----------------------------------------------------------------------------
// Stress Test 4: Pseudo-Random Fuzz Headers (Unicode, Binary, Giant, Duplicates)
// -----------------------------------------------------------------------------

func TestM6Adversarial_PseudoRandomFuzzHeaders(t *testing.T) {
	t.Run("UnicodeAndInternationalization_Matrix", func(t *testing.T) {
		t.Parallel()

		unicodeFields := []HeaderField{
			{Name: ":method", Value: "GET"},
			{Name: ":path", Value: "/i18n/test"},
			{Name: "x-cyrillic", Value: "Привет, мир! Тестирование QPACK таблицы."},
			{Name: "x-cjk-chinese", Value: "你好世界！高性能动态表压缩测试。"},
			{Name: "x-cjk-japanese", Value: "こんにちは世界！動的テーブル圧縮テスト。"},
			{Name: "x-arabic-rtl", Value: "أهلاً وسهلاً بالعالم! اختبار ضغط الترويسة."},
			{Name: "x-hebrew-rtl", Value: "שלום עולם! בדיקת דחיסת כותרות."},
			{Name: "x-devanagari", Value: "नमस्ते दुनिया! हेडर कम्प्रेशन टेस्ट."},
			{Name: "x-accents-latin", Value: "Café, naïve façade, crème brûlée & pâté."},
			{Name: "x-emoji-astral-4byte", Value: "🚀🔥🎉💻🦀𠜎𝄞𝒳😀⚡🎯🛡️"},
		}

		combinations := []struct {
			name    string
			huffman HuffmanEncoding
			cookie  CookieCrumbling
		}{
			{"HuffmanOn_CookieOn", HuffmanEncodingEnabled, CookieCrumblingEnabled},
			{"HuffmanOff_CookieOn", HuffmanEncodingDisabled, CookieCrumblingEnabled},
			{"HuffmanOn_CookieOff", HuffmanEncodingEnabled, CookieCrumblingDisabled},
			{"HuffmanOff_CookieOff", HuffmanEncodingDisabled, CookieCrumblingDisabled},
		}

		for _, tc := range combinations {
			tc := tc
			t.Run(tc.name, func(t *testing.T) {
				h := newRoundtripHarnessWithSettings(t, 4096, 10, modeImmediate, tc.huffman, tc.cookie)
				decoded := h.Roundtrip(1, unicodeFields)
				assertHeaderListsEqual(t, unicodeFields, decoded, tc.name)
			})
		}
	})

	t.Run("BinaryValuesAndRawBytes", func(t *testing.T) {
		t.Parallel()
		h := newRoundtripHarness(t, 4096, 10, modeImmediate)

		// Arbitrary binary bytes from 0x01 to 0x1f and 0x80 to 0xff
		rawLowBytes := make([]byte, 31)
		for i := range rawLowBytes {
			rawLowBytes[i] = byte(i + 1) // 0x01 to 0x1f
		}

		rawHighBytes := make([]byte, 128)
		for i := range rawHighBytes {
			rawHighBytes[i] = byte(0x80 + i) // 0x80 to 0xff
		}

		binaryFields := []HeaderField{
			{Name: "x-binary-low", Value: string(rawLowBytes)},
			{Name: "x-binary-high", Value: string(rawHighBytes)},
			{Name: "x-binary-mixed", Value: string(append(rawLowBytes, rawHighBytes...))},
		}

		decoded := h.Roundtrip(1, binaryFields)
		assertHeaderListsEqual(t, binaryFields, decoded, "binary fields")

		// Also verify null-splitting behavior: in Chromium, non-cookie headers with \0
		// are split into multiple fields with the same name.
		nullSeparated := []HeaderField{
			{Name: "x-multi-null", Value: "val1\x00val2\x00val3"},
		}
		expectedSplit := []HeaderField{
			{Name: "x-multi-null", Value: "val1"},
			{Name: "x-multi-null", Value: "val2"},
			{Name: "x-multi-null", Value: "val3"},
		}
		decodedSplit := h.RoundtripWithExpected(2, nullSeparated, expectedSplit)
		assertHeaderListsEqual(t, expectedSplit, decodedSplit, "null-separated split fields")
	})

	t.Run("GiantHeaders_64KB_Stress", func(t *testing.T) {
		t.Parallel()

		// Test A: Giant 64KB header value exceeding table capacity -> falls back to literal
		t.Run("LiteralFallback_ExceedsCapacity", func(t *testing.T) {
			h := newRoundtripHarness(t, 2048, 10, modeManual)
			giantVal := strings.Repeat("0123456789ABCDEF", 4096) // 65,536 bytes (64 KB)
			fields := []HeaderField{
				{Name: ":method", Value: "POST"},
				{Name: ":path", Value: "/giant-upload"},
				{Name: "x-giant-payload-64k", Value: giantVal},
			}

			block := h.Encode(1, fields)
			handler := chaosDeliverInterleaved(t, h, 1, block, rand.New(rand.NewSource(42)), 256, 4096)
			require.True(t, handler.Completed())
			require.False(t, handler.HasError())
			assertHeaderListsEqual(t, fields, handler.Headers(), "giant literal header")
		})

		// Test B: Large header fits in dynamic table
		t.Run("DynamicInsert_FitsInTable", func(t *testing.T) {
			h := newRoundtripHarness(t, 131072, 10, modeManual)  // 128 KB table
			giantVal := strings.Repeat("ZXCVBNMASDFGHJKL", 2048) // 32,768 bytes (32 KB)
			fields := []HeaderField{
				{Name: "x-giant-dynamic-32k", Value: giantVal},
			}

			block := h.Encode(1, fields)
			handler := chaosDeliverInterleaved(t, h, 1, block, rand.New(rand.NewSource(99)), 1024, 8192)
			require.True(t, handler.Completed())
			require.False(t, handler.HasError())
			assertHeaderListsEqual(t, fields, handler.Headers(), "giant dynamic header")

			// Second stream: dynamic table HIT on 32KB entry! Wire size must be tiny (< 50 bytes)
			block2, sentBytes2 := h.EncodeWithByteCount(2, fields)
			assert.Truef(t, sentBytes2 == 0, "stream 2 should have 0 encoder stream bytes due to dynamic hit")
			assert.Truef(
				t,
				len(block2) < 50,
				"stream 2 header block (%d bytes) should be tiny due to indexed match",
				len(block2),
			)

			handler2 := chaosDeliverInterleaved(t, h, 2, block2, rand.New(rand.NewSource(101)), 16, 64)
			require.True(t, handler2.Completed())
			assertHeaderListsEqual(t, fields, handler2.Headers(), "giant dynamic hit")
		})
	})

	t.Run("DuplicateNamesAndCookieMatrix", func(t *testing.T) {
		t.Parallel()
		h := newRoundtripHarness(t, 4096, 10, modeImmediate)

		var duplicateFields []HeaderField
		duplicateFields = append(duplicateFields, HeaderField{Name: ":method", Value: "GET"})
		duplicateFields = append(duplicateFields, HeaderField{Name: ":path", Value: "/cookies"})

		// 25 set-cookie headers with differing values
		for i := 0; i < 25; i++ {
			duplicateFields = append(duplicateFields, HeaderField{
				Name:  "set-cookie",
				Value: fmt.Sprintf("session_cookie_%02d=token_%d; Path=/; Secure; HttpOnly; SameSite=Lax", i, i*12345),
			})
		}

		// 10 identical custom headers to stress dynamic table deduplication
		for i := 0; i < 10; i++ {
			duplicateFields = append(duplicateFields, HeaderField{
				Name:  "x-identical-dedup",
				Value: "same-exact-payload-value-dedup",
			})
		}

		decoded := h.Roundtrip(1, duplicateFields)
		assertHeaderListsEqual(t, duplicateFields, decoded, "duplicate header fields")
	})

	t.Run("PseudoRandomFuzz_Roundtrip", func(t *testing.T) {
		t.Parallel()
		h := newRoundtripHarness(t, 8192, 20, modeImmediate)
		rng := rand.New(rand.NewSource(20260919))

		standardNames := []string{
			":method", ":scheme", ":authority", ":path",
			"user-agent", "accept", "accept-encoding", "accept-language",
			"cache-control", "authorization", "content-type", "content-length",
			"x-custom-alpha", "x-custom-beta", "x-custom-gamma",
		}

		const numFuzzRequests = 50
		for reqIdx := 0; reqIdx < numFuzzRequests; reqIdx++ {
			streamID := uint64(1000 + reqIdx*2)
			numFields := rng.Intn(18) + 2

			var fields []HeaderField
			for f := 0; f < numFields; f++ {
				var name string
				if rng.Intn(3) == 0 {
					// Custom random name
					name = fmt.Sprintf("x-fuzz-%d-%d", reqIdx, rng.Intn(100))
				} else {
					// Standard name
					name = standardNames[rng.Intn(len(standardNames))]
				}

				// Random value generator
				valType := rng.Intn(5)
				var val string
				switch valType {
				case 0:
					val = "" // Empty value
				case 1:
					val = fmt.Sprintf("short-%d", rng.Int63())
				case 2:
					val = strings.Repeat("A", rng.Intn(300)+10)
				case 3:
					val = fmt.Sprintf("Unicode_🚀_Тест_%d_%d", reqIdx, f)
				case 4:
					b := make([]byte, rng.Intn(50)+5)
					for i := range b {
						b[i] = byte(rng.Intn(255) + 1) // Non-zero binary byte
					}
					val = string(b)
				}

				fields = append(fields, HeaderField{Name: name, Value: val})
			}

			decoded := h.Roundtrip(streamID, fields)
			assertHeaderListsEqual(t, fields, decoded, fmt.Sprintf("fuzz request %d", reqIdx))
		}

		assert.Equal(t, h.encoder.HeaderTable().InsertedEntryCount(), h.decoder.HeaderTable().InsertedEntryCount())
	})
}
