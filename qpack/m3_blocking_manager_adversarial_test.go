// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package qpack

import (
	"fmt"
	"math"
	"math/rand"
	"sync"
	"testing"
)

// blockingManagerOracle is an independent reference model for BlockingManager.
type blockingManagerOracle struct {
	streamBlocks       map[uint64][]oracleBlock
	knownReceivedCount uint64
	minIndexMultiSet   map[uint64]int
	maxBlockedStreams  uint64
}

type oracleBlock struct {
	ric      uint64
	minIndex uint64
	hasMin   bool
}

func newOracle(maxBlocked uint64) *blockingManagerOracle {
	return &blockingManagerOracle{
		streamBlocks:       make(map[uint64][]oracleBlock),
		knownReceivedCount: 0,
		minIndexMultiSet:   make(map[uint64]int),
		maxBlockedStreams:  maxBlocked,
	}
}

func (o *blockingManagerOracle) onHeaderBlockSent(streamID uint64, indices []uint64) {
	if len(indices) == 0 {
		b := oracleBlock{ric: 0, minIndex: 0, hasMin: false}
		o.streamBlocks[streamID] = append(o.streamBlocks[streamID], b)
		return
	}
	minIdx := indices[0]
	maxIdx := indices[0]
	for _, idx := range indices[1:] {
		if idx < minIdx {
			minIdx = idx
		}
		if idx > maxIdx {
			maxIdx = idx
		}
	}
	b := oracleBlock{ric: maxIdx + 1, minIndex: minIdx, hasMin: true}
	o.streamBlocks[streamID] = append(o.streamBlocks[streamID], b)
	o.minIndexMultiSet[minIdx]++
}

func (o *blockingManagerOracle) onHeaderBlockReceived(streamID, ric uint64) {
	b := oracleBlock{ric: ric, minIndex: 0, hasMin: false}
	o.streamBlocks[streamID] = append(o.streamBlocks[streamID], b)
}

func (o *blockingManagerOracle) onSectionAck(streamID uint64) bool {
	blocks := o.streamBlocks[streamID]
	if len(blocks) == 0 {
		return false
	}
	popped := blocks[0]
	if len(blocks) == 1 {
		delete(o.streamBlocks, streamID)
	} else {
		o.streamBlocks[streamID] = blocks[1:]
	}
	if popped.hasMin {
		o.minIndexMultiSet[popped.minIndex]--
		if o.minIndexMultiSet[popped.minIndex] == 0 {
			delete(o.minIndexMultiSet, popped.minIndex)
		}
	}
	if popped.ric > o.knownReceivedCount {
		o.knownReceivedCount = popped.ric
	}
	return true
}

func (o *blockingManagerOracle) onStreamCancellation(streamID uint64) {
	blocks, ok := o.streamBlocks[streamID]
	if !ok {
		return
	}
	for _, b := range blocks {
		if b.hasMin {
			o.minIndexMultiSet[b.minIndex]--
			if o.minIndexMultiSet[b.minIndex] == 0 {
				delete(o.minIndexMultiSet, b.minIndex)
			}
		}
	}
	delete(o.streamBlocks, streamID)
}

func (o *blockingManagerOracle) onInsertCountIncrement(delta uint64) bool {
	if math.MaxUint64-o.knownReceivedCount < delta {
		return false
	}
	o.knownReceivedCount += delta
	return true
}

func (o *blockingManagerOracle) smallestBlockingIndex() uint64 {
	if len(o.minIndexMultiSet) == 0 {
		return math.MaxUint64
	}
	minVal := uint64(math.MaxUint64)
	for idx := range o.minIndexMultiSet {
		if idx < minVal {
			minVal = idx
		}
	}
	return minVal
}

func (o *blockingManagerOracle) isBlocked(streamID uint64) bool {
	blocks, ok := o.streamBlocks[streamID]
	if !ok || len(blocks) == 0 {
		return false
	}
	for _, b := range blocks {
		if b.ric > o.knownReceivedCount {
			return true
		}
	}
	return false
}

func (o *blockingManagerOracle) numBlockedStreams() uint64 {
	count := uint64(0)
	for s := range o.streamBlocks {
		if o.isBlocked(s) {
			count++
		}
	}
	return count
}

func (o *blockingManagerOracle) blockingAllowed(streamID uint64) bool {
	if o.isBlocked(streamID) {
		return true
	}
	return o.numBlockedStreams() < o.maxBlockedStreams
}

// ----------------------------------------------------------------------------
// Test 1: Massive stream concurrency (thousands of streams, interleaved blocks)
// ----------------------------------------------------------------------------
func TestM3BlockingManagerAdversarial_MassiveStreamConcurrency(t *testing.T) {
	const numStreams = 5000
	const numOperations = 50000

	maxBlocked := uint64(200)
	mgr := NewBlockingManager(maxBlocked)
	oracle := newOracle(maxBlocked)

	rng := rand.New(rand.NewSource(99991))

	var activeStreams []uint64
	streamCreated := make(map[uint64]bool)

	for op := 0; op < numOperations; op++ {
		action := rng.Intn(100)

		switch {
		case action < 45: // 45%: OnHeaderBlockSent on a stream
			var streamID uint64
			if len(activeStreams) < numStreams && (len(activeStreams) == 0 || rng.Intn(3) == 0) {
				streamID = uint64(len(activeStreams) * 4) // HTTP/3 client-initiated bidirectional stream ID pattern
				activeStreams = append(activeStreams, streamID)
				streamCreated[streamID] = true
			} else {
				streamID = activeStreams[rng.Intn(len(activeStreams))]
			}

			// Generate 0 to 10 random indices
			numIdx := rng.Intn(11)
			indices := make([]uint64, numIdx)
			baseIdx := uint64(rng.Intn(5000))
			for i := 0; i < numIdx; i++ {
				indices[i] = baseIdx + uint64(rng.Intn(50))
			}

			mgr.OnHeaderBlockSent(streamID, indices)
			oracle.onHeaderBlockSent(streamID, indices)

		case action < 70: // 25%: OnSectionAck on an existing stream
			if len(activeStreams) > 0 {
				idx := rng.Intn(len(activeStreams))
				streamID := activeStreams[idx]
				resMgr := mgr.OnSectionAck(streamID)
				resOracle := oracle.onSectionAck(streamID)
				if resMgr != resOracle {
					t.Fatalf("op %d: OnSectionAck(%d) mismatch: mgr=%v, oracle=%v", op, streamID, resMgr, resOracle)
				}
			}

		case action < 85: // 15%: OnStreamCancellation
			if len(activeStreams) > 0 {
				idx := rng.Intn(len(activeStreams))
				streamID := activeStreams[idx]
				mgr.OnStreamCancellation(streamID)
				oracle.onStreamCancellation(streamID)
			}

		case action < 95: // 10%: OnInsertCountIncrement
			delta := uint64(rng.Intn(20) + 1)
			resMgr := mgr.OnInsertCountIncrement(delta)
			resOracle := oracle.onInsertCountIncrement(delta)
			if resMgr != resOracle {
				t.Fatalf("op %d: OnInsertCountIncrement(%d) mismatch: mgr=%v, oracle=%v", op, delta, resMgr, resOracle)
			}

		default: // 5%: OnHeaderBlockReceived (decoder path)
			if len(activeStreams) > 0 {
				streamID := activeStreams[rng.Intn(len(activeStreams))]
				ric := uint64(rng.Intn(6000))
				blockedMgr := mgr.OnHeaderBlockReceived(streamID, ric)
				oracle.onHeaderBlockReceived(streamID, ric)
				blockedOracle := oracle.isBlocked(streamID)
				if blockedMgr != blockedOracle {
					t.Fatalf(
						"op %d: OnHeaderBlockReceived(%d, %d) blocked mismatch: mgr=%v, oracle=%v",
						op,
						streamID,
						ric,
						blockedMgr,
						blockedOracle,
					)
				}
			}
		}

		// Invariant checks every 500 operations
		if op%500 == 0 || op == numOperations-1 {
			if mgr.SmallestBlockingIndex() != oracle.smallestBlockingIndex() {
				t.Fatalf("op %d: SmallestBlockingIndex mismatch: got %d, expected %d",
					op, mgr.SmallestBlockingIndex(), oracle.smallestBlockingIndex())
			}
			if mgr.KnownReceivedCount() != oracle.knownReceivedCount {
				t.Fatalf("op %d: KnownReceivedCount mismatch: got %d, expected %d",
					op, mgr.KnownReceivedCount(), oracle.knownReceivedCount)
			}
			if mgr.NumBlockedStreams() != oracle.numBlockedStreams() {
				t.Fatalf("op %d: NumBlockedStreams mismatch: got %d, expected %d",
					op, mgr.NumBlockedStreams(), oracle.numBlockedStreams())
			}

			// Spot-check 20 active streams
			for i := 0; i < 20 && i < len(activeStreams); i++ {
				s := activeStreams[rng.Intn(len(activeStreams))]
				if mgr.IsBlocked(s) != oracle.isBlocked(s) {
					t.Fatalf(
						"op %d: IsBlocked(%d) mismatch: got %v, expected %v",
						op,
						s,
						mgr.IsBlocked(s),
						oracle.isBlocked(s),
					)
				}
				if mgr.BlockingAllowedOnStream(s) != oracle.blockingAllowed(s) {
					t.Fatalf("op %d: BlockingAllowedOnStream(%d) mismatch: got %v, expected %v",
						op, s, mgr.BlockingAllowedOnStream(s), oracle.blockingAllowed(s))
				}
			}
		}
	}
}

// ----------------------------------------------------------------------------
// Test 2: Pathological block patterns
// ----------------------------------------------------------------------------
func TestM3BlockingManagerAdversarial_PathologicalBlockPatterns(t *testing.T) {
	t.Run("MultipleBlocksOnSameStreamFIFOOrdering", func(t *testing.T) {
		mgr := NewBlockingManager()
		streamID := uint64(42)

		// 100 blocks on a single stream with zig-zag indices
		const numBlocks = 100
		minExpected := make([]uint64, numBlocks)
		ricExpected := make([]uint64, numBlocks)

		for i := 0; i < numBlocks; i++ {
			var indices []uint64
			if i%3 == 0 {
				indices = []uint64{uint64(100 + i), uint64(200 + i)}
			} else if i%3 == 1 {
				indices = []uint64{uint64(50 + i), uint64(300 + i)}
			} else {
				indices = []uint64{uint64(10 + i), uint64(500 + i)}
			}
			minExpected[i] = indices[0]
			ricExpected[i] = indices[1] + 1
			mgr.OnHeaderBlockSent(streamID, indices)
		}

		// Pop blocks one by one and assert SmallestBlockingIndex updates correctly
		for i := 0; i < numBlocks; i++ {
			// Find min among remaining blocks
			expectedMin := uint64(math.MaxUint64)
			for j := i; j < numBlocks; j++ {
				if minExpected[j] < expectedMin {
					expectedMin = minExpected[j]
				}
			}

			if mgr.SmallestBlockingIndex() != expectedMin {
				t.Fatalf(
					"step %d: SmallestBlockingIndex = %d, expected %d",
					i,
					mgr.SmallestBlockingIndex(),
					expectedMin,
				)
			}

			if !mgr.OnSectionAck(streamID) {
				t.Fatalf("step %d: OnSectionAck failed prematurely", i)
			}
		}

		// After all popped, must be MaxUint64
		if mgr.SmallestBlockingIndex() != math.MaxUint64 {
			t.Fatalf(
				"after popping all blocks, SmallestBlockingIndex = %d, expected MaxUint64",
				mgr.SmallestBlockingIndex(),
			)
		}
		if mgr.OnSectionAck(streamID) {
			t.Fatalf("extra OnSectionAck should have returned false")
		}
	})

	t.Run("EmptyBlocksEdgeCases", func(t *testing.T) {
		mgr := NewBlockingManager()
		streamID := uint64(99)

		// 1. Send empty block (nil and empty slice)
		mgr.OnHeaderBlockSent(streamID, nil)
		mgr.OnHeaderBlockSent(streamID, []uint64{})

		if mgr.SmallestBlockingIndex() != math.MaxUint64 {
			t.Fatalf("empty blocks should not affect SmallestBlockingIndex, got %d", mgr.SmallestBlockingIndex())
		}
		if mgr.IsBlocked(streamID) {
			t.Fatalf("empty blocks should not block stream")
		}
		if mgr.NumBlockedStreams() != 0 {
			t.Fatalf("NumBlockedStreams should be 0, got %d", mgr.NumBlockedStreams())
		}

		// 2. Interleave empty block with non-empty block
		mgr.OnHeaderBlockSent(streamID, []uint64{40, 50}) // min 40, RIC 51
		if mgr.SmallestBlockingIndex() != 40 {
			t.Fatalf("SmallestBlockingIndex = %d, expected 40", mgr.SmallestBlockingIndex())
		}
		if !mgr.IsBlocked(streamID) {
			t.Fatalf("stream should be blocked by block 3")
		}

		mgr.OnHeaderBlockSent(streamID, []uint64{}) // block 4 is empty
		if mgr.SmallestBlockingIndex() != 40 {
			t.Fatalf("empty block should not overwrite SmallestBlockingIndex")
		}

		// Pop block 1 (empty)
		if !mgr.OnSectionAck(streamID) {
			t.Fatalf("popping empty block 1 failed")
		}
		if mgr.SmallestBlockingIndex() != 40 {
			t.Fatalf("SmallestBlockingIndex should remain 40 after popping empty block")
		}
		if !mgr.IsBlocked(streamID) {
			t.Fatalf("stream should remain blocked while block 3 is pending")
		}

		// Pop block 2 (empty)
		if !mgr.OnSectionAck(streamID) {
			t.Fatalf("popping empty block 2 failed")
		}

		// Pop block 3 (non-empty: min 40, RIC 51)
		if !mgr.OnSectionAck(streamID) {
			t.Fatalf("popping block 3 failed")
		}
		if mgr.KnownReceivedCount() != 51 {
			t.Fatalf("KnownReceivedCount = %d, expected 51", mgr.KnownReceivedCount())
		}
		if mgr.SmallestBlockingIndex() != math.MaxUint64 {
			t.Fatalf("SmallestBlockingIndex should be MaxUint64 after block 3 popped")
		}
		if mgr.IsBlocked(streamID) {
			t.Fatalf("stream should be unblocked now")
		}

		// Pop block 4 (empty)
		if !mgr.OnSectionAck(streamID) {
			t.Fatalf("popping block 4 failed")
		}
		if mgr.OnSectionAck(streamID) {
			t.Fatalf("popping non-existent block 5 should fail")
		}
	})

	t.Run("SingleIndexBlocksAndHighMultiplicity", func(t *testing.T) {
		mgr := NewBlockingManager()
		const count = 1000

		// 1000 streams all reference identical single index 7
		for s := uint64(1); s <= count; s++ {
			mgr.OnHeaderBlockSent(s, []uint64{7})
			if mgr.SmallestBlockingIndex() != 7 {
				t.Fatalf("stream %d: expected SmallestBlockingIndex 7, got %d", s, mgr.SmallestBlockingIndex())
			}
		}

		// Stream 2000 sends index 15
		mgr.OnHeaderBlockSent(2000, []uint64{15})

		// Acknowledge first 999 streams with index 7
		for s := uint64(1); s < count; s++ {
			if !mgr.OnSectionAck(s) {
				t.Fatalf("stream %d: ack failed", s)
			}
			// Smallest must remain 7 because stream 1000 still references index 7
			if mgr.SmallestBlockingIndex() != 7 {
				t.Fatalf("stream %d: SmallestBlockingIndex prematurely changed to %d", s, mgr.SmallestBlockingIndex())
			}
		}

		// Acknowledge stream 1000 (last stream referencing index 7)
		if !mgr.OnSectionAck(count) {
			t.Fatalf("stream %d: ack failed", count)
		}
		// Now SmallestBlockingIndex must transition to 15!
		if mgr.SmallestBlockingIndex() != 15 {
			t.Fatalf("after last index 7 acked, SmallestBlockingIndex = %d, expected 15", mgr.SmallestBlockingIndex())
		}

		// Cancel stream 2000
		mgr.OnStreamCancellation(2000)
		if mgr.SmallestBlockingIndex() != math.MaxUint64 {
			t.Fatalf(
				"after cancelling stream 2000, SmallestBlockingIndex = %d, expected MaxUint64",
				mgr.SmallestBlockingIndex(),
			)
		}
	})

	t.Run("SingleBlockWithDuplicateIndices", func(t *testing.T) {
		mgr := NewBlockingManager()
		// Block contains multiple duplicates of the same index
		mgr.OnHeaderBlockSent(1, []uint64{5, 5, 5, 5, 5})
		if mgr.SmallestBlockingIndex() != 5 {
			t.Fatalf("expected SmallestBlockingIndex 5, got %d", mgr.SmallestBlockingIndex())
		}

		// Single ack should completely remove index 5 (only 1 block was sent)
		if !mgr.OnSectionAck(1) {
			t.Fatalf("ack failed")
		}
		if mgr.SmallestBlockingIndex() != math.MaxUint64 {
			t.Fatalf("expected MaxUint64 after single ack, got %d", mgr.SmallestBlockingIndex())
		}
	})
}

// ----------------------------------------------------------------------------
// Test 3: Out-of-order stream cancellations and duplicate section acknowledgments
// ----------------------------------------------------------------------------
func TestM3BlockingManagerAdversarial_OutOfOrderCancellationsAndDuplicateAcks(t *testing.T) {
	mgr := NewBlockingManager(10)

	// Setup 5 streams
	mgr.OnHeaderBlockSent(1, []uint64{10, 20}) // min 10, RIC 21
	mgr.OnHeaderBlockSent(2, []uint64{5, 15})  // min 5,  RIC 16
	mgr.OnHeaderBlockSent(3, []uint64{12, 18}) // min 12, RIC 19
	mgr.OnHeaderBlockSent(4, []uint64{3, 8})   // min 3,  RIC 9
	mgr.OnHeaderBlockSent(5, []uint64{25, 30}) // min 25, RIC 31

	if mgr.SmallestBlockingIndex() != 3 {
		t.Fatalf("expected SmallestBlockingIndex 3, got %d", mgr.SmallestBlockingIndex())
	}
	if mgr.NumBlockedStreams() != 5 {
		t.Fatalf("expected 5 blocked streams, got %d", mgr.NumBlockedStreams())
	}

	// 1. Cancel Stream 4 (which held the smallest index 3)
	mgr.OnStreamCancellation(4)
	if mgr.IsBlocked(4) {
		t.Fatalf("stream 4 should not be blocked after cancellation")
	}
	if mgr.NumBlockedStreams() != 4 {
		t.Fatalf("expected 4 blocked streams after cancelling stream 4, got %d", mgr.NumBlockedStreams())
	}
	// Smallest should advance to 5 (stream 2)
	if mgr.SmallestBlockingIndex() != 5 {
		t.Fatalf("expected SmallestBlockingIndex 5 after stream 4 cancellation, got %d", mgr.SmallestBlockingIndex())
	}
	// Stream cancellation must NOT advance KnownReceivedCount
	if mgr.KnownReceivedCount() != 0 {
		t.Fatalf("cancellation should not advance KnownReceivedCount, got %d", mgr.KnownReceivedCount())
	}

	// 2. Duplicate Ack on cancelled stream 4 must fail
	if mgr.OnSectionAck(4) {
		t.Fatalf("OnSectionAck on cancelled stream 4 should return false")
	}
	if mgr.OnHeaderAcknowledgement(4) {
		t.Fatalf("OnHeaderAcknowledgement on cancelled stream 4 should return false")
	}

	// 3. Repeated cancellation on already cancelled stream 4 should be safe no-op
	for i := 0; i < 10; i++ {
		mgr.OnStreamCancellation(4)
	}
	if mgr.SmallestBlockingIndex() != 5 {
		t.Fatalf("SmallestBlockingIndex corrupted after repeated cancellations, got %d", mgr.SmallestBlockingIndex())
	}

	// 4. Cancellation on never-seen stream 999 should be safe no-op
	mgr.OnStreamCancellation(999)
	if mgr.SmallestBlockingIndex() != 5 {
		t.Fatalf(
			"SmallestBlockingIndex corrupted after cancellation of unseen stream, got %d",
			mgr.SmallestBlockingIndex(),
		)
	}

	// 5. Duplicate acks on active stream 2
	// First ack succeeds and advances knownReceivedCount to 16
	if !mgr.OnSectionAck(2) {
		t.Fatalf("first ack on stream 2 failed")
	}
	if mgr.KnownReceivedCount() != 16 {
		t.Fatalf("KnownReceivedCount = %d, expected 16", mgr.KnownReceivedCount())
	}
	// Stream 2 is now completely drained. Subsequent 50 acks must fail cleanly
	for i := 0; i < 50; i++ {
		if mgr.OnSectionAck(2) {
			t.Fatalf("iteration %d: duplicate ack on drained stream 2 should return false", i)
		}
	}

	// Invariants check after 50 duplicate acks
	if mgr.KnownReceivedCount() != 16 {
		t.Fatalf("KnownReceivedCount should remain 16, got %d", mgr.KnownReceivedCount())
	}
	// Remaining active streams: 1 (min 10, RIC 21), 3 (min 12, RIC 19), 5 (min 25, RIC 31)
	// Smallest index must now be 10!
	if mgr.SmallestBlockingIndex() != 10 {
		t.Fatalf("expected SmallestBlockingIndex 10, got %d", mgr.SmallestBlockingIndex())
	}
	// Stream 3 had RIC 19 > 16, so stream 3 is still blocked
	// Stream 1 had RIC 21 > 16, so stream 1 is still blocked
	// Stream 5 had RIC 31 > 16, so stream 5 is still blocked
	if mgr.NumBlockedStreams() != 3 {
		t.Fatalf("expected 3 blocked streams, got %d", mgr.NumBlockedStreams())
	}

	// Clean up stream 1, 3, 5
	mgr.OnStreamCancellation(1)
	mgr.OnStreamCancellation(3)
	mgr.OnStreamCancellation(5)
	if mgr.SmallestBlockingIndex() != math.MaxUint64 {
		t.Fatalf("expected MaxUint64 after all cancellations, got %d", mgr.SmallestBlockingIndex())
	}
	if mgr.NumBlockedStreams() != 0 {
		t.Fatalf("expected 0 blocked streams, got %d", mgr.NumBlockedStreams())
	}
}

// ----------------------------------------------------------------------------
// Test 4: Eviction barrier regression (SmallestBlockingIndex strictly protects entries)
// ----------------------------------------------------------------------------
func TestM3BlockingManagerAdversarial_EvictionBarrierRegression(t *testing.T) {
	// Create dynamic table and blocking manager
	table := NewEncoderHeaderTable()
	const tableCapacity = 10000
	table.SetMaximumDynamicTableCapacity(tableCapacity)
	table.SetDynamicTableCapacity(tableCapacity)

	mgr := NewBlockingManager()

	// Insert 80 entries into dynamic table
	const totalEntries = 80
	for i := 0; i < totalEntries; i++ {
		name := fmt.Sprintf("custom-key-%03d", i)
		val := fmt.Sprintf("custom-val-%03d", i)
		idx := table.InsertEntry(name, val)
		if idx != uint64(i) {
			t.Fatalf("inserted index mismatch: got %d, expected %d", idx, i)
		}
	}

	if table.InsertedEntryCount() != totalEntries {
		t.Fatalf("InsertedEntryCount = %d, expected %d", table.InsertedEntryCount(), totalEntries)
	}
	if table.DroppedEntryCount() != 0 {
		t.Fatalf("DroppedEntryCount = %d, expected 0", table.DroppedEntryCount())
	}

	// 10 streams send blocks referencing dynamic entries
	type streamPlan struct {
		streamID uint64
		indices  []uint64
	}

	plans := []streamPlan{
		{streamID: 1, indices: []uint64{15, 25, 40}}, // min 15
		{streamID: 2, indices: []uint64{8, 12, 30}},  // min 8
		{streamID: 3, indices: []uint64{35, 50, 70}}, // min 35
		{streamID: 4, indices: []uint64{5, 10, 20}},  // min 5
		{streamID: 5, indices: []uint64{22, 45, 60}}, // min 22
		{streamID: 6, indices: []uint64{3, 18, 55}},  // min 3
		{streamID: 7, indices: []uint64{50, 65, 75}}, // min 50
		{streamID: 8, indices: []uint64{1, 4, 16}},   // min 1
	}

	for _, p := range plans {
		mgr.OnHeaderBlockSent(p.streamID, p.indices)
	}

	// SmallestBlockingIndex must be 1 (stream 8)
	if mgr.SmallestBlockingIndex() != 1 {
		t.Fatalf("SmallestBlockingIndex = %d, expected 1", mgr.SmallestBlockingIndex())
	}

	// Evict dynamic table down to index 1 (can only drop index 0)
	maxInsertSize := table.MaxInsertSizeWithoutEvictingGivenEntry(mgr.SmallestBlockingIndex())
	if maxInsertSize == 0 {
		t.Fatalf("expected positive insert budget without evicting index 1")
	}

	// Actively referenced entries map: index -> count
	activeRefs := make(map[uint64]int)
	for _, p := range plans {
		for _, idx := range p.indices {
			activeRefs[idx]++
		}
	}

	// Assert invariant: for all active refs, idx >= SmallestBlockingIndex()
	assertEvictionBarrierInvariant := func(stage string) {
		smallest := mgr.SmallestBlockingIndex()
		if len(activeRefs) == 0 {
			if smallest != math.MaxUint64 {
				t.Fatalf("[%s] expected MaxUint64 when no active refs, got %d", stage, smallest)
			}
			return
		}

		minActive := uint64(math.MaxUint64)
		for idx := range activeRefs {
			if idx < minActive {
				minActive = idx
			}
			if idx < smallest {
				t.Fatalf("[%s] ACTIVE ENTRY %d IS BELOW EVICITON BARRIER %d!", stage, idx, smallest)
			}
		}

		if smallest != minActive {
			t.Fatalf("[%s] SmallestBlockingIndex %d does not match minActive %d", stage, smallest, minActive)
		}

		// Evict table up to barrier: dropped count must never exceed barrier
		table.EvictDownToCapacity(table.DynamicTableCapacity() - table.DynamicTableSize() + 10)
		if table.DroppedEntryCount() > smallest {
			t.Fatalf("[%s] table dropped %d entries, exceeding barrier %d!", stage, table.DroppedEntryCount(), smallest)
		}

		// Verify every active entry is STILL accessible and intact
		for idx := range activeRefs {
			name := fmt.Sprintf("custom-key-%03d", idx)
			val := fmt.Sprintf("custom-val-%03d", idx)
			res := table.FindHeaderField(name, val)
			if res.Match != MatchTypeNameAndValue || res.IsStatic || res.Index != idx {
				t.Fatalf("[%s] Active entry %d was prematurely evicted or corrupted! Match=%v, IsStatic=%v, Index=%d",
					stage, idx, res.Match, res.IsStatic, res.Index)
			}
		}
	}

	assertEvictionBarrierInvariant("Initial state")

	// Step-by-step acknowledge or cancel streams out of order
	// 1. Ack stream 8 (min 1). Smallest should advance to 3 (stream 6)
	mgr.OnSectionAck(8)
	for _, idx := range plans[7].indices {
		activeRefs[idx]--
		if activeRefs[idx] == 0 {
			delete(activeRefs, idx)
		}
	}
	assertEvictionBarrierInvariant("After acking stream 8")

	// 2. Cancel stream 6 (min 3). Smallest should advance to 5 (stream 4)
	mgr.OnStreamCancellation(6)
	for _, idx := range plans[5].indices {
		activeRefs[idx]--
		if activeRefs[idx] == 0 {
			delete(activeRefs, idx)
		}
	}
	assertEvictionBarrierInvariant("After cancelling stream 6")

	// 3. Ack stream 4 (min 5). Smallest should advance to 8 (stream 2)
	mgr.OnSectionAck(4)
	for _, idx := range plans[3].indices {
		activeRefs[idx]--
		if activeRefs[idx] == 0 {
			delete(activeRefs, idx)
		}
	}
	assertEvictionBarrierInvariant("After acking stream 4")

	// 4. Cancel stream 2 (min 8). Smallest should advance to 15 (stream 1)
	mgr.OnStreamCancellation(2)
	for _, idx := range plans[1].indices {
		activeRefs[idx]--
		if activeRefs[idx] == 0 {
			delete(activeRefs, idx)
		}
	}
	assertEvictionBarrierInvariant("After cancelling stream 2")

	// 5. Ack stream 1 (min 15). Smallest should advance to 22 (stream 5)
	mgr.OnSectionAck(1)
	for _, idx := range plans[0].indices {
		activeRefs[idx]--
		if activeRefs[idx] == 0 {
			delete(activeRefs, idx)
		}
	}
	assertEvictionBarrierInvariant("After acking stream 1")

	// 6. Ack remaining streams 5, 3, 7
	for _, sid := range []uint64{5, 3, 7} {
		mgr.OnSectionAck(sid)
	}
	activeRefs = make(map[uint64]int)
	assertEvictionBarrierInvariant("After acking all streams")

	// Now barrier is MaxUint64. Table can be completely evicted without restriction.
	table.EvictDownToCapacity(0)
	if table.DynamicTableSize() != 0 {
		t.Fatalf("table size = %d, expected 0", table.DynamicTableSize())
	}
	if table.DroppedEntryCount() != totalEntries {
		t.Fatalf("DroppedEntryCount = %d, expected %d", table.DroppedEntryCount(), totalEntries)
	}
}

// ----------------------------------------------------------------------------
// Test 5: Overflow and boundary conditions in RIC and known received counts
// ----------------------------------------------------------------------------
func TestM3BlockingManagerAdversarial_OverflowAndBoundaryConditions(t *testing.T) {
	t.Run("InsertCountIncrementBoundaries", func(t *testing.T) {
		mgr := NewBlockingManager()

		// Advance to MaxUint64 - 2
		if !mgr.OnInsertCountIncrement(math.MaxUint64 - 2) {
			t.Fatalf("increment failed")
		}
		if mgr.KnownReceivedCount() != math.MaxUint64-2 {
			t.Fatalf("KnownReceivedCount = %d, expected MaxUint64 - 2", mgr.KnownReceivedCount())
		}

		// Increment by 2 exactly reaches MaxUint64
		if !mgr.OnInsertCountIncrement(2) {
			t.Fatalf("increment by 2 failed")
		}
		if mgr.KnownReceivedCount() != math.MaxUint64 {
			t.Fatalf("KnownReceivedCount = %d, expected MaxUint64", mgr.KnownReceivedCount())
		}

		// Incrementing by 0 at MaxUint64 should succeed
		if !mgr.OnInsertCountIncrement(0) {
			t.Fatalf("increment by 0 at MaxUint64 should succeed")
		}
		if mgr.KnownReceivedCount() != math.MaxUint64 {
			t.Fatalf("KnownReceivedCount should remain MaxUint64")
		}

		// Incrementing by 1 beyond MaxUint64 must fail with overflow
		if mgr.OnInsertCountIncrement(1) {
			t.Fatalf("increment beyond MaxUint64 should return false")
		}
		if mgr.KnownReceivedCount() != math.MaxUint64 {
			t.Fatalf("KnownReceivedCount corrupted after overflow attempt, got %d", mgr.KnownReceivedCount())
		}
	})

	t.Run("MaxUint64RequiredInsertCount", func(t *testing.T) {
		mgr := NewBlockingManager(5)

		// Decoder receives header block with RIC = MaxUint64
		blocked := mgr.OnHeaderBlockReceived(1, math.MaxUint64)
		if !blocked {
			t.Fatalf("stream 1 with RIC = MaxUint64 must be blocked")
		}
		if !mgr.IsBlocked(1) {
			t.Fatalf("IsBlocked(1) should be true")
		}
		if mgr.NumBlockedStreams() != 1 {
			t.Fatalf("NumBlockedStreams = %d, expected 1", mgr.NumBlockedStreams())
		}

		// Section ack pops the block and advances knownReceivedCount to MaxUint64
		if !mgr.OnSectionAck(1) {
			t.Fatalf("OnSectionAck(1) failed")
		}
		if mgr.KnownReceivedCount() != math.MaxUint64 {
			t.Fatalf("KnownReceivedCount = %d, expected MaxUint64", mgr.KnownReceivedCount())
		}
		if mgr.IsBlocked(1) {
			t.Fatalf("stream 1 should now be unblocked")
		}
		if mgr.NumBlockedStreams() != 0 {
			t.Fatalf("NumBlockedStreams = %d, expected 0", mgr.NumBlockedStreams())
		}

		// Subsequent block with RIC = MaxUint64 on stream 2 should NOT block
		blocked = mgr.OnHeaderBlockReceived(2, math.MaxUint64)
		if blocked {
			t.Fatalf("stream 2 should not block because knownReceivedCount is already MaxUint64")
		}
	})

	t.Run("MaxBlockedStreamsZeroBoundary", func(t *testing.T) {
		// SETTINGS_QPACK_BLOCKED_STREAMS default is 0
		mgr := NewBlockingManager(0)
		if mgr.MaxBlockedStreams() != 0 {
			t.Fatalf("expected MaxBlockedStreams 0, got %d", mgr.MaxBlockedStreams())
		}

		// Initially unblocked stream: blocking is NOT allowed
		if mgr.BlockingAllowedOnStream(1) {
			t.Fatalf("when MaxBlockedStreams == 0, unblocked stream must not be allowed to block")
		}

		// If stream 1 sends a blocking block anyway
		mgr.OnHeaderBlockSent(1, []uint64{0})
		if !mgr.IsBlocked(1) {
			t.Fatalf("stream 1 should be blocked")
		}
		// Now stream 1 is ALREADY blocked: sending additional blocks on stream 1 is allowed
		if !mgr.BlockingAllowedOnStream(1) {
			t.Fatalf("already blocked stream 1 must be allowed to send blocks")
		}
		// But stream 2 is NOT blocked: still not allowed
		if mgr.BlockingAllowedOnStream(2) {
			t.Fatalf("unblocked stream 2 must not be allowed to block")
		}
	})

	t.Run("IndicesSliceBoundaryCalculations", func(t *testing.T) {
		// Index = math.MaxUint64 - 1
		hd := NewHeaderData([]uint64{math.MaxUint64 - 1})
		if hd.RequiredInsertCount() != math.MaxUint64 {
			t.Fatalf("expected RIC MaxUint64, got %d", hd.RequiredInsertCount())
		}
		if min, ok := hd.MinIndex(); !ok || min != math.MaxUint64-1 {
			t.Fatalf("expected minIndex MaxUint64 - 1, got %d", min)
		}

		// Large unsorted indices with 0
		hd2 := NewHeaderData([]uint64{100, 50, 0, 75, 200})
		if min, ok := hd2.MinIndex(); !ok || min != 0 {
			t.Fatalf("expected minIndex 0, got %d", min)
		}
		if hd2.RequiredInsertCount() != 201 {
			t.Fatalf("expected RIC 201, got %d", hd2.RequiredInsertCount())
		}
	})
}

// ----------------------------------------------------------------------------
// Test 6: Concurrency isolation and thread-safety under external mutex
// ----------------------------------------------------------------------------
func TestM3BlockingManagerAdversarial_GoroutineIsolationAndConcurrentStress(t *testing.T) {
	// 1. Independent instance isolation: multiple goroutines each with their own manager
	const numRoutines = 30
	var wg sync.WaitGroup
	wg.Add(numRoutines)

	for g := 0; g < numRoutines; g++ {
		go func(routineID int) {
			defer wg.Done()
			m := NewBlockingManager(10)
			for i := 0; i < 500; i++ {
				sid := uint64(i * 2)
				m.OnHeaderBlockSent(sid, []uint64{uint64(i)})
				if m.SmallestBlockingIndex() != uint64(i) {
					t.Errorf(
						"routine %d: expected SmallestBlockingIndex %d, got %d",
						routineID,
						i,
						m.SmallestBlockingIndex(),
					)
					return
				}
				m.OnSectionAck(sid)
			}
		}(g)
	}
	wg.Wait()

	// 2. High-throughput concurrent access to a single manager protected by a mutex
	type safeBlockingManager struct {
		mu  sync.Mutex
		mgr *BlockingManager
	}

	sm := &safeBlockingManager{mgr: NewBlockingManager(50)}
	const concurrentWorkers = 50
	const opsPerWorker = 500

	wg.Add(concurrentWorkers)
	for w := 0; w < concurrentWorkers; w++ {
		go func(workerID int) {
			defer wg.Done()
			baseStream := uint64(workerID * 1000)

			for i := 0; i < opsPerWorker; i++ {
				sid := baseStream + uint64(i)
				sm.mu.Lock()
				sm.mgr.OnHeaderBlockSent(sid, []uint64{uint64(workerID * 10), uint64(workerID*10 + 5)})
				sm.mgr.BlockingAllowedOnStream(sid)
				sm.mgr.SmallestBlockingIndex()
				sm.mgr.IsBlocked(sid)
				sm.mu.Unlock()

				if i%2 == 0 {
					sm.mu.Lock()
					sm.mgr.OnSectionAck(sid)
					sm.mu.Unlock()
				} else {
					sm.mu.Lock()
					sm.mgr.OnStreamCancellation(sid)
					sm.mu.Unlock()
				}
			}
		}(w)
	}
	wg.Wait()

	// Verify all streams were cleaned up
	if sm.mgr.SmallestBlockingIndex() != math.MaxUint64 {
		t.Fatalf(
			"expected SmallestBlockingIndex MaxUint64 after all workers finished, got %d",
			sm.mgr.SmallestBlockingIndex(),
		)
	}
	if sm.mgr.NumBlockedStreams() != 0 {
		t.Fatalf("expected 0 blocked streams, got %d", sm.mgr.NumBlockedStreams())
	}
}
