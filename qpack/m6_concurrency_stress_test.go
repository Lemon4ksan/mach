// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package qpack

import (
	"fmt"
	"math"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lemon4ksan/foundation/testing/assert"
)

// -----------------------------------------------------------------------------
// Target 1: Concurrency Stress with 500+ Streams Across 50 Goroutines
// -----------------------------------------------------------------------------

func TestM6Concurrency_500Streams_50Goroutines_Stress(t *testing.T) {
	t.Parallel()

	const (
		numGoroutines    = 50
		streamsPerWorker = 12
		totalStreams     = numGoroutines * streamsPerWorker // 600 streams (> 500 target)
		tableCapacity    = 16384
		maxBlocked       = 50
	)

	h := newRoundtripHarness(t, tableCapacity, maxBlocked, modeImmediate)

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	type workerError struct {
		workerID int
		streamID uint64
		err      error
	}
	errCh := make(chan workerError, totalStreams)

	for w := 0; w < numGoroutines; w++ {
		go func(workerID int) {
			defer wg.Done()

			for s := 0; s < streamsPerWorker; s++ {
				streamID := uint64(workerID*2000 + s*2 + 1)

				// Interleave pseudo-headers, shared corporate headers, and worker-unique headers
				headers := []HeaderField{
					{Name: ":method", Value: "POST"},
					{Name: ":scheme", Value: "https"},
					{Name: ":authority", Value: "cluster.internal.mesh"},
					{Name: ":path", Value: fmt.Sprintf("/v2/workers/%d/tasks/%d", workerID, s)},
					{Name: "user-agent", Value: "Go-QPACK-ConcurrencyStress/4.0 (x86_64)"},
					{Name: "content-type", Value: "application/json"},
					{Name: "accept", Value: "application/json, text/plain, */*"},
					{Name: "accept-encoding", Value: "gzip, deflate, br, zstd"},
					{Name: "x-worker-id", Value: fmt.Sprintf("worker-node-%03d", workerID)},
					{Name: "x-task-seq", Value: fmt.Sprintf("seq-%06d", s)},
					{
						Name:  "x-trace-id",
						Value: fmt.Sprintf("trace-%04d-%04d-%08x", workerID, s, workerID*7919+s*104729),
					},
				}

				decoded := h.Roundtrip(streamID, headers)
				if len(decoded) != len(headers) {
					errCh <- workerError{
						workerID: workerID,
						streamID: streamID,
						err:      fmt.Errorf("length mismatch: got %d, want %d", len(decoded), len(headers)),
					}
					return
				}

				for i := range headers {
					if decoded[i].Name != headers[i].Name || decoded[i].Value != headers[i].Value {
						errCh <- workerError{
							workerID: workerID,
							streamID: streamID,
							err: fmt.Errorf("field %d mismatch: got (%s: %s), want (%s: %s)",
								i, decoded[i].Name, decoded[i].Value, headers[i].Name, headers[i].Value),
						}
						return
					}
				}
			}
		}(w)
	}

	wg.Wait()
	close(errCh)

	for we := range errCh {
		t.Fatalf("concurrency stress failure in worker %d on stream %d: %v", we.workerID, we.streamID, we.err)
	}

	// Dynamic table synchronization invariants
	encTable := h.encoder.HeaderTable()
	decTable := h.decoder.HeaderTable()

	assert.Equal(t, encTable.InsertedEntryCount(), decTable.InsertedEntryCount(), "inserted entry counts must match")
	assert.Equal(t, encTable.DroppedEntryCount(), decTable.DroppedEntryCount(), "dropped entry counts must match")
	assert.Equal(
		t,
		h.encoder.BlockingManager().KnownReceivedCount(),
		h.decoder.KnownReceivedCount(),
		"known received counts must match",
	)

	assert.True(t, encTable.InsertedEntryCount() > 0, "must have dynamic insertions")
	assert.True(t, encTable.DroppedEntryCount() > 0, "table turnover must have caused evictions")
}

// -----------------------------------------------------------------------------
// Target 2: Blocked Stream Quota Saturation and Boundary Enforcement
// -----------------------------------------------------------------------------

func TestM6Concurrency_BlockedStreamQuota_SaturationAndBoundary(t *testing.T) {
	t.Parallel()

	// -------------------------------------------------------------------------
	// Part A: Encoder-Side Boundary Enforcement & Quota Saturation
	// -------------------------------------------------------------------------
	t.Run("EncoderQuotaSaturation", func(t *testing.T) {
		const maxBlocked = 3
		h := newRoundtripHarness(t, 4096, maxBlocked, modeManual)

		maxLimit := h.encoder.MaximumBlockedStreams()
		assert.Equal(t, uint64(maxBlocked), maxLimit)

		bm := h.encoder.BlockingManager()
		assert.Equal(t, uint64(0), bm.NumBlockedStreams())

		// Streams 10, 20, 30 insert dynamic entries and become blocked (since feedback is withheld)
		for s := uint64(1); s <= maxBlocked; s++ {
			streamID := s * 10
			assert.Truef(
				t,
				bm.BlockingAllowedOnStream(streamID, maxLimit),
				"blocking should be allowed for stream %d",
				streamID,
			)
			block := h.Encode(
				streamID,
				[]HeaderField{{Name: fmt.Sprintf("x-quota-%d", s), Value: fmt.Sprintf("val-%d", s)}},
			)
			assert.True(t, len(block) > 0)
			assert.Truef(t, bm.IsBlocked(streamID), "stream %d must be marked blocked", streamID)
		}

		assert.Equal(t, uint64(maxBlocked), bm.NumBlockedStreams(), "blocked streams quota must be fully saturated")

		// Boundary Check 1: A NEW stream (stream 99) must NOT be permitted to block
		assert.False(
			t,
			bm.BlockingAllowedOnStream(99, maxLimit),
			"blocking must NOT be allowed when quota is saturated",
		)

		// Encode stream 99 with a novel header: encoder must refuse to insert or reference unacknowledged entry
		block99 := h.Encode(99, []HeaderField{{Name: "x-refused-dynamic", Value: "must-be-literal"}})
		assert.True(t, len(block99) > 0)
		assert.False(t, bm.IsBlocked(99), "stream 99 must NOT be blocked (fell back to literal)")
		assert.Equal(t, uint64(maxBlocked), bm.NumBlockedStreams(), "blocked streams count must stay at maxBlocked")

		// Boundary Check 2: An ALREADY BLOCKED stream (e.g. stream 20) CAN reference dynamic entries
		// because its blocking status does not increase the number of blocked streams
		assert.True(
			t,
			bm.BlockingAllowedOnStream(20, maxLimit),
			"already blocked stream must be allowed to block further",
		)
		block20_second := h.Encode(20, []HeaderField{{Name: "x-quota-2", Value: "val-2"}})
		assert.True(t, len(block20_second) > 0)
		assert.Equal(
			t,
			uint64(maxBlocked),
			bm.NumBlockedStreams(),
			"blocked streams count must remain exactly maxBlocked",
		)

		// Cancellation of stream 10 releases 1 blocked stream quota without advancing knownReceivedCount
		h.encoder.OnStreamCancellation(10)
		assert.Equal(
			t,
			uint64(maxBlocked-1),
			bm.NumBlockedStreams(),
			"blocked stream count must decrease after cancellation",
		)
		assert.True(t, bm.BlockingAllowedOnStream(100, maxLimit), "new stream must now be permitted to block")

		// Encode stream 100: fills quota back to maxBlocked
		block100 := h.Encode(100, []HeaderField{{Name: "x-quota-100", Value: "val-100"}})
		assert.True(t, len(block100) > 0)
		assert.True(t, bm.IsBlocked(100))
		assert.Equal(t, uint64(maxBlocked), bm.NumBlockedStreams())
	})

	// -------------------------------------------------------------------------
	// Part B: Decoder-Side Boundary Enforcement Against Adversarial Peer
	// -------------------------------------------------------------------------
	t.Run("DecoderStrictBoundaryEnforcement", func(t *testing.T) {
		const decoderMaxBlocked = 2
		decSender := &virtualControlStreamSender{}
		decoder := NewDecoder(4096, decoderMaxBlocked, nil)
		decoder.SetDynamicTableCapacity(4096)
		decoder.SetStreamSenderDelegate(decSender)

		// Create an unrestricted helper encoder (maxBlocked=10) to craft blocks requiring RIC > 0
		encSender := &virtualControlStreamSender{}
		encHelper := NewEncoder(nil, HuffmanEncodingEnabled, CookieCrumblingEnabled)
		encHelper.SetMaximumDynamicTableCapacity(4096)
		encHelper.SetDynamicTableCapacity(4096)
		encHelper.SetMaximumBlockedStreams(10)
		encHelper.SetStreamSenderDelegate(encSender)

		// Generate 3 header blocks with increasing RIC (RIC=1, RIC=2, RIC=3)
		blockA := encHelper.EncodeHeaderList(100, []HeaderField{{Name: "x-adv-1", Value: "val1"}}, nil)
		blockB := encHelper.EncodeHeaderList(200, []HeaderField{{Name: "x-adv-2", Value: "val2"}}, nil)
		blockC := encHelper.EncodeHeaderList(300, []HeaderField{{Name: "x-adv-3", Value: "val3"}}, nil)

		// Do NOT deliver encoder stream instructions yet (decoder InsertedEntryCount is 0)

		// Deliver Block A (stream 100): requires RIC 1 > 0 -> blocks (count = 1)
		handlerA := newRoundtripHeadersHandler()
		pDecA := decoder.CreateProgressiveDecoder(100, handlerA)
		pDecA.Decode(blockA)
		pDecA.EndHeaderBlock()
		assert.False(t, handlerA.Completed())
		assert.False(t, handlerA.HasError())
		assert.Equal(t, 1, len(decoder.blockedStreams))

		// Deliver Block B (stream 200): requires RIC 2 > 0 -> blocks (count = 2 == decoderMaxBlocked)
		handlerB := newRoundtripHeadersHandler()
		pDecB := decoder.CreateProgressiveDecoder(200, handlerB)
		pDecB.Decode(blockB)
		pDecB.EndHeaderBlock()
		assert.False(t, handlerB.Completed())
		assert.False(t, handlerB.HasError())
		assert.Equal(t, 2, len(decoder.blockedStreams))

		// Deliver Block C (stream 300): requires RIC 3 > 0 -> EXCEEDS decoderMaxBlocked!
		handlerC := newRoundtripHeadersHandler()
		pDecC := decoder.CreateProgressiveDecoder(300, handlerC)
		pDecC.Decode(blockC)
		pDecC.EndHeaderBlock()

		// Adversarial rejection assertion: decoder MUST detect quota violation
		assert.True(t, handlerC.HasError(), "decoder must reject stream exceeding maxBlockedStreams")
		assert.Equal(t, uint64(QPACK_DECOMPRESSION_FAILED), handlerC.errorCode)
		assert.Contains(t, handlerC.errorMessage, "Limit on number of blocked streams exceeded.")

		// Abort violating stream 300
		decoder.OnStreamReset(300)
		pDecC.Close()
		assert.Equal(
			t,
			2,
			len(decoder.blockedStreams),
			"blocked streams count must be back to 2 after resetting failed stream",
		)

		// Deliver first dynamic entry to unblock stream 100 (RIC=1)
		decoder.InsertWithoutNameReference("x-adv-1", "val1")
		assert.True(t, handlerA.Completed(), "stream 100 must unblock when RIC 1 is satisfied")
		assert.Equal(t, 1, len(decoder.blockedStreams), "blocked stream count must decrement to 1")

		// Now a new stream (stream 400) requiring RIC=2 can block safely (1 + 1 <= 2)
		blockD := encHelper.EncodeHeaderList(400, []HeaderField{{Name: "x-adv-2", Value: "val2"}}, nil)
		handlerD := newRoundtripHeadersHandler()
		pDecD := decoder.CreateProgressiveDecoder(400, handlerD)
		pDecD.Decode(blockD)
		pDecD.EndHeaderBlock()
		assert.False(t, handlerD.HasError(), "stream 400 must NOT error now that quota has room")
		assert.Equal(t, 2, len(decoder.blockedStreams))

		// Deliver second dynamic entry to unblock stream 200 and stream 400 (RIC=2)
		decoder.InsertWithoutNameReference("x-adv-2", "val2")
		assert.True(t, handlerB.Completed())
		assert.True(t, handlerD.Completed())
		assert.Equal(t, 0, len(decoder.blockedStreams))
	})
}

func roundtripStreamAtomic(h *roundtripHarness, streamID uint64, fields []HeaderField) ([]HeaderField, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	block := h.encoder.EncodeHeaderList(streamID, fields, nil)
	handler := newRoundtripHeadersHandler()
	progDec := h.decoder.CreateProgressiveDecoder(streamID, handler)
	progDec.Decode(block)
	progDec.EndHeaderBlock()
	h.decoder.FlushDecoderStream()

	if !handler.Completed() {
		return nil, fmt.Errorf("stream %d decoding not completed", streamID)
	}
	if handler.HasError() {
		return nil, fmt.Errorf("stream %d decoding error %d: %s", streamID, handler.errorCode, handler.errorMessage)
	}
	return handler.Headers(), nil
}

func TestM6Concurrency_DynamicTableCapacityToggles_InFlight(t *testing.T) {
	t.Parallel()

	const (
		maxCapacity   = 65536
		numWorkers    = 30
		streamsPerW   = 15
		totalExpected = numWorkers * streamsPerW
	)

	h := newRoundtripHarness(t, maxCapacity, 50, modeImmediate)

	var running atomic.Bool
	running.Store(true)

	// Goroutine 1: Rapid capacity toggler cycling 65536 <-> 0 <-> intermediate capacities
	var togglerWg sync.WaitGroup
	togglerWg.Add(1)
	go func() {
		defer togglerWg.Done()
		capacities := []uint64{65536, 0, 32768, 512, 0, 16384, 0, 65536, 1024, 0, 65536}
		idx := 0
		for running.Load() {
			capVal := capacities[idx%len(capacities)]
			idx++

			h.mu.Lock()
			h.encoder.SetDynamicTableCapacity(capVal)
			h.encoder.EncoderStreamSender().Flush()
			h.mu.Unlock()

			time.Sleep(1 * time.Millisecond)
		}
	}()

	// 30 concurrent workers continuously sending requests while capacity oscillates
	var workersWg sync.WaitGroup
	workersWg.Add(numWorkers)

	type failureRecord struct {
		workerID int
		streamID uint64
		err      error
	}
	failCh := make(chan failureRecord, totalExpected)

	for w := 0; w < numWorkers; w++ {
		go func(workerID int) {
			defer workersWg.Done()

			for s := 0; s < streamsPerW; s++ {
				streamID := uint64(workerID*5000 + s*2 + 1)
				headers := []HeaderField{
					{Name: ":method", Value: "GET"},
					{Name: ":scheme", Value: "https"},
					{Name: ":authority", Value: "dynamic-toggle.mesh.local"},
					{Name: ":path", Value: fmt.Sprintf("/toggle/worker/%d/req/%d", workerID, s)},
					{Name: "x-toggle-header", Value: fmt.Sprintf("val-toggle-%d-%d", workerID, s)},
					{Name: "x-toggle-padding", Value: "padding-string-for-varying-size-eviction-1234567890"},
				}

				decoded, err := roundtripStreamAtomic(h, streamID, headers)
				if err != nil {
					failCh <- failureRecord{
						workerID: workerID,
						streamID: streamID,
						err:      err,
					}
					return
				}
				for i := range headers {
					if decoded[i].Name != headers[i].Name || decoded[i].Value != headers[i].Value {
						failCh <- failureRecord{
							workerID: workerID,
							streamID: streamID,
							err: fmt.Errorf("header mismatch: got %s: %s, want %s: %s",
								decoded[i].Name, decoded[i].Value, headers[i].Name, headers[i].Value),
						}
						return
					}
				}
			}
		}(w)
	}

	workersWg.Wait()
	running.Store(false)
	togglerWg.Wait()
	close(failCh)

	for f := range failCh {
		t.Fatalf("dynamic table toggle failure in worker %d on stream %d: %v", f.workerID, f.streamID, f.err)
	}

	// Final stabilization: set capacity back to 65536 and verify sync
	h.mu.Lock()
	h.encoder.SetDynamicTableCapacity(maxCapacity)
	h.encoder.EncoderStreamSender().Flush()
	h.mu.Unlock()

	h.Roundtrip(999999, []HeaderField{{Name: "x-final-probe", Value: "probe-value"}})

	assert.Equal(t, h.encoder.HeaderTable().DynamicTableCapacity(), h.decoder.HeaderTable().DynamicTableCapacity())
	assert.Equal(t, h.encoder.HeaderTable().InsertedEntryCount(), h.decoder.HeaderTable().InsertedEntryCount())
	assert.Equal(t, h.encoder.HeaderTable().DroppedEntryCount(), h.decoder.HeaderTable().DroppedEntryCount())
	assert.Equal(t, h.encoder.BlockingManager().KnownReceivedCount(), h.decoder.KnownReceivedCount())
}

// -----------------------------------------------------------------------------
// Target 4: Memory Leak & Reference Retention Checks After Completion/Reset
// -----------------------------------------------------------------------------

func TestM6Concurrency_MemoryRetention_CompletionAndCancellation(t *testing.T) {
	t.Parallel()

	// -------------------------------------------------------------------------
	// Part A: Normal Stream Completion Reference Cleansing
	// -------------------------------------------------------------------------
	t.Run("NormalStreamCompletionRetention", func(t *testing.T) {
		h := newRoundtripHarness(t, 8192, 50, modeImmediate)

		for i := 0; i < 100; i++ {
			streamID := uint64(i*2 + 1)
			h.Roundtrip(streamID, []HeaderField{
				{Name: ":method", Value: "GET"},
				{Name: ":path", Value: fmt.Sprintf("/res/item/%d", i)},
				{Name: fmt.Sprintf("x-cust-%d", i), Value: fmt.Sprintf("val-%d", i)},
			})
		}

		bm := h.encoder.BlockingManager()
		assert.Equal(t, 0, len(bm.streamMap), "streamMap must have 0 retained entries after completions")
		assert.Equal(t, 0, len(bm.blockedStreams), "blockedStreams must be 0 after completions")
		assert.Equal(t, 0, len(bm.minIndexRefCounts), "minIndexRefCounts must be 0 after completions")
		assert.Equal(t, uint64(math.MaxUint64), bm.SmallestBlockingIndex(), "smallestBlockingIndex must be MaxUint64")

		assert.Equal(t, 0, len(h.decoder.blockedStreams), "decoder blockedStreams must be 0 after completions")
		assert.Equal(t, 0, len(h.decoder.headerTable.observers), "decoder observers must be 0 after completions")
	})

	// -------------------------------------------------------------------------
	// Part B: Stream Cancellation & Reset Reference Cleansing
	// -------------------------------------------------------------------------
	t.Run("StreamCancellationRetention", func(t *testing.T) {
		const numResetStreams = 40
		h := newRoundtripHarness(t, 8192, 50, modeManual)

		pDecs := make([]*ProgressiveDecoder, numResetStreams)

		for i := 0; i < numResetStreams; i++ {
			streamID := uint64(1000 + i*2)
			block := h.Encode(streamID, []HeaderField{
				{Name: fmt.Sprintf("x-cancel-hdr-%d", i), Value: fmt.Sprintf("cancel-val-%d", i)},
			})

			handler := newRoundtripHeadersHandler()
			pDec := h.decoder.CreateProgressiveDecoder(streamID, handler)
			pDec.Decode(block)
			pDec.EndHeaderBlock()
			pDecs[i] = pDec
		}

		// Verify state prior to cancellation: 40 streams registered as in-flight / blocked
		bm := h.encoder.BlockingManager()
		assert.Equal(t, numResetStreams, len(bm.streamMap), "all 40 streams must be in streamMap before reset")
		assert.Equal(
			t,
			numResetStreams,
			len(h.decoder.blockedStreams),
			"all 40 streams must be in decoder blockedStreams",
		)
		assert.Equal(t, numResetStreams, len(h.decoder.headerTable.observers), "all 40 observers must be registered")

		// Abort all 40 streams
		for i := 0; i < numResetStreams; i++ {
			streamID := uint64(1000 + i*2)
			h.decoder.OnStreamReset(streamID)
			pDecs[i].Close()
		}

		// Deliver decoder stream Stream Cancellation instructions to encoder
		h.DeliverDecoderStream(0)

		// Assert total reference liberation on both encoder and decoder
		assert.Equal(t, 0, len(bm.streamMap), "streamMap must be completely drained after stream cancellations")
		assert.Equal(t, 0, len(bm.blockedStreams), "blockedStreams must be 0 after stream cancellations")
		assert.Equal(t, 0, len(bm.minIndexRefCounts), "minIndexRefCounts must be empty after cancellations")
		assert.Equal(
			t,
			uint64(math.MaxUint64),
			bm.SmallestBlockingIndex(),
			"smallestBlockingIndex must reset to MaxUint64",
		)

		assert.Equal(t, 0, len(h.decoder.blockedStreams), "decoder blockedStreams must be empty after resets")
		assert.Equal(t, 0, len(h.decoder.headerTable.observers), "decoder observers must be empty after resets")
	})

	// -------------------------------------------------------------------------
	// Part C: Ring Buffer Pointer Zeroing Inspection on Eviction
	// -------------------------------------------------------------------------
	t.Run("RingBufferEvictionPointerZeroing", func(t *testing.T) {
		table := NewDecoderHeaderTable()
		table.SetMaximumDynamicTableCapacity(200)
		table.SetDynamicTableCapacity(200)

		// Insert 100 entries into 200-byte table. Each entry is ~50 bytes (name+val+32)
		// Table can hold at most ~3-4 entries concurrently. 96+ entries will be evicted.
		for i := 0; i < 100; i++ {
			table.InsertEntry(fmt.Sprintf("key-%03d", i), fmt.Sprintf("value-%03d", i))
		}

		assert.True(t, table.DroppedEntryCount() > 90, "over 90 entries must be dropped")

		rb := &table.dynamicEntries
		assert.True(t, rb.count <= 4, "active count must not exceed capacity limit")

		// Inspect underlying ring buffer slice directly: all inactive slots must be strictly nil!
		activeIndices := make(map[int]bool)
		for i := 0; i < rb.count; i++ {
			idx := (rb.head + i) % len(rb.entries)
			activeIndices[idx] = true
		}

		for i, entry := range rb.entries {
			if !activeIndices[i] {
				assert.Nilf(t, entry, "inactive ring buffer slot at index %d must be nil (no memory retention)", i)
			} else {
				assert.NotNilf(t, entry, "active ring buffer slot at index %d must hold entry", i)
			}
		}

		// Force garbage collection to ensure no dangling pointer crashes
		runtime.GC()
	})
}
