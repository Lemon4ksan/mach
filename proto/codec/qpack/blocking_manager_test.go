// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package qpack

import (
	"math"
	"testing"
)

// Test 1: Empty
func TestBlockingManager_Empty(t *testing.T) {
	manager := NewBlockingManager()
	if manager.SmallestBlockingIndex() != math.MaxUint64 {
		t.Errorf("expected SmallestBlockingIndex == MaxUint64, got %d", manager.SmallestBlockingIndex())
	}
	if manager.smallest_blocking_index() != math.MaxUint64 {
		t.Errorf("expected smallest_blocking_index == MaxUint64, got %d", manager.smallest_blocking_index())
	}
	if manager.KnownReceivedCount() != 0 {
		t.Errorf("expected KnownReceivedCount == 0, got %d", manager.KnownReceivedCount())
	}
	if manager.known_received_count() != 0 {
		t.Errorf("expected known_received_count == 0, got %d", manager.known_received_count())
	}
	if manager.IsBlocked(0) {
		t.Errorf("stream 0 should not be blocked")
	}
	if manager.is_blocked(0) {
		t.Errorf("stream 0 should not be blocked")
	}
	if manager.IsBlocked(1) {
		t.Errorf("stream 1 should not be blocked")
	}
	if manager.NumBlockedStreams() != 0 {
		t.Errorf("expected NumBlockedStreams == 0, got %d", manager.NumBlockedStreams())
	}
	if manager.num_blocked_streams() != 0 {
		t.Errorf("expected num_blocked_streams == 0, got %d", manager.num_blocked_streams())
	}
	if !manager.BlockingAllowedOnStream(0, 1) {
		t.Errorf("expected blocking allowed on stream 0")
	}
	if !manager.blocking_allowed_on_stream(0, 1) {
		t.Errorf("expected blocking_allowed_on_stream on stream 0")
	}
}

// Test 2: Blocked
func TestBlockingManager_Blocked(t *testing.T) {
	manager := NewBlockingManager()
	// Stream 1 sends block with index 0 (RIC = 1)
	manager.OnHeaderBlockSent(1, []uint64{0})
	if !manager.IsBlocked(1) {
		t.Errorf("stream 1 should be blocked")
	}
	if manager.NumBlockedStreams() != 1 {
		t.Errorf("expected 1 blocked stream, got %d", manager.NumBlockedStreams())
	}
	if manager.SmallestBlockingIndex() != 0 {
		t.Errorf("expected SmallestBlockingIndex == 0, got %d", manager.SmallestBlockingIndex())
	}
	if manager.KnownReceivedCount() != 0 {
		t.Errorf("expected KnownReceivedCount == 0, got %d", manager.KnownReceivedCount())
	}
}

// Test 3: NotBlockedByInsertCountIncrement
func TestBlockingManager_NotBlockedByInsertCountIncrement(t *testing.T) {
	manager := NewBlockingManager()
	if !manager.OnInsertCountIncrement(2) {
		t.Fatalf("OnInsertCountIncrement failed")
	}
	if manager.KnownReceivedCount() != 2 {
		t.Errorf("expected KnownReceivedCount == 2, got %d", manager.KnownReceivedCount())
	}

	// Indices 0 and 1: required insert count is max(0, 1) + 1 = 2 <= KnownReceivedCount (2)
	manager.OnHeaderBlockSent(1, []uint64{0, 1})
	if manager.IsBlocked(1) {
		t.Errorf("stream 1 should not be blocked")
	}
	if manager.NumBlockedStreams() != 0 {
		t.Errorf("expected 0 blocked streams, got %d", manager.NumBlockedStreams())
	}
	if manager.SmallestBlockingIndex() != 0 {
		t.Errorf("expected SmallestBlockingIndex == 0, got %d", manager.SmallestBlockingIndex())
	}
}

// Test 4: UnblockedByInsertCountIncrement
func TestBlockingManager_UnblockedByInsertCountIncrement(t *testing.T) {
	manager := NewBlockingManager()
	manager.OnHeaderBlockSent(1, []uint64{0})
	if !manager.IsBlocked(1) {
		t.Errorf("stream 1 should be blocked")
	}
	if manager.NumBlockedStreams() != 1 {
		t.Errorf("expected 1 blocked stream, got %d", manager.NumBlockedStreams())
	}

	if !manager.OnInsertCountIncrement(1) {
		t.Fatalf("OnInsertCountIncrement failed")
	}
	if manager.IsBlocked(1) {
		t.Errorf("stream 1 should be unblocked")
	}
	if manager.NumBlockedStreams() != 0 {
		t.Errorf("expected 0 blocked streams, got %d", manager.NumBlockedStreams())
	}
	if manager.SmallestBlockingIndex() != 0 {
		t.Errorf("expected SmallestBlockingIndex == 0, got %d", manager.SmallestBlockingIndex())
	}
	if manager.KnownReceivedCount() != 1 {
		t.Errorf("expected KnownReceivedCount == 1, got %d", manager.KnownReceivedCount())
	}
}

// Test 5: NotBlockedByHeaderAcknowledgement
func TestBlockingManager_NotBlockedByHeaderAcknowledgement(t *testing.T) {
	manager := NewBlockingManager()
	manager.OnHeaderBlockSent(1, []uint64{0})
	if !manager.IsBlocked(1) {
		t.Errorf("stream 1 should be blocked")
	}

	// Acknowledge stream 1
	if !manager.OnHeaderAcknowledgement(1) {
		t.Fatalf("OnHeaderAcknowledgement failed")
	}
	if manager.KnownReceivedCount() != 1 {
		t.Errorf("expected KnownReceivedCount == 1, got %d", manager.KnownReceivedCount())
	}
	if manager.IsBlocked(1) {
		t.Errorf("stream 1 should not be blocked")
	}

	// Stream 2 sends block referencing index 0 (RIC=1 <= KnownReceivedCount=1)
	manager.OnHeaderBlockSent(2, []uint64{0})
	if manager.IsBlocked(2) {
		t.Errorf("stream 2 should not be blocked")
	}
	if manager.NumBlockedStreams() != 0 {
		t.Errorf("expected 0 blocked streams, got %d", manager.NumBlockedStreams())
	}
}

// Test 6: UnblockedByHeaderAcknowledgement
func TestBlockingManager_UnblockedByHeaderAcknowledgement(t *testing.T) {
	manager := NewBlockingManager()
	// Stream 1 sends block with index 1 (RIC = 2)
	manager.OnHeaderBlockSent(1, []uint64{1})
	// Stream 2 sends block with index 0 (RIC = 1)
	manager.OnHeaderBlockSent(2, []uint64{0})

	if !manager.IsBlocked(1) || !manager.IsBlocked(2) {
		t.Errorf("both stream 1 and stream 2 should be blocked")
	}
	if manager.NumBlockedStreams() != 2 {
		t.Errorf("expected 2 blocked streams, got %d", manager.NumBlockedStreams())
	}

	// Acknowledge stream 1 -> known received count becomes 2
	if !manager.OnHeaderAcknowledgement(1) {
		t.Fatalf("OnHeaderAcknowledgement failed")
	}
	if manager.KnownReceivedCount() != 2 {
		t.Errorf("expected KnownReceivedCount == 2, got %d", manager.KnownReceivedCount())
	}
	if manager.IsBlocked(1) {
		t.Errorf("stream 1 should not be blocked")
	}
	if manager.IsBlocked(2) {
		t.Errorf("stream 2 should be unblocked by stream 1's ack")
	}
	if manager.NumBlockedStreams() != 0 {
		t.Errorf("expected 0 blocked streams, got %d", manager.NumBlockedStreams())
	}
}

// Test 7: KnownReceivedCount
func TestBlockingManager_KnownReceivedCount(t *testing.T) {
	manager := NewBlockingManager()
	if manager.KnownReceivedCount() != 0 {
		t.Errorf("expected 0, got %d", manager.KnownReceivedCount())
	}

	if !manager.OnInsertCountIncrement(3) {
		t.Fatalf("OnInsertCountIncrement failed")
	}
	if manager.KnownReceivedCount() != 3 {
		t.Errorf("expected 3, got %d", manager.KnownReceivedCount())
	}

	// Send block with index 1 (RIC = 2). Ack it -> known count should remain 3.
	manager.OnHeaderBlockSent(1, []uint64{1})
	if !manager.OnHeaderAcknowledgement(1) {
		t.Fatalf("OnHeaderAcknowledgement failed")
	}
	if manager.KnownReceivedCount() != 3 {
		t.Errorf("expected 3, got %d", manager.KnownReceivedCount())
	}

	// Send block with index 4 (RIC = 5). Ack it -> known count becomes 5.
	manager.OnHeaderBlockSent(2, []uint64{4})
	if !manager.OnHeaderAcknowledgement(2) {
		t.Fatalf("OnHeaderAcknowledgement failed")
	}
	if manager.KnownReceivedCount() != 5 {
		t.Errorf("expected 5, got %d", manager.KnownReceivedCount())
	}

	// Increment by 2 -> known count becomes 7.
	if !manager.OnInsertCountIncrement(2) {
		t.Fatalf("OnInsertCountIncrement failed")
	}
	if manager.KnownReceivedCount() != 7 {
		t.Errorf("expected 7, got %d", manager.KnownReceivedCount())
	}
}

// Test 8: SmallestBlockingIndex
func TestBlockingManager_SmallestBlockingIndex(t *testing.T) {
	manager := NewBlockingManager()
	if manager.SmallestBlockingIndex() != math.MaxUint64 {
		t.Errorf("expected MaxUint64, got %d", manager.SmallestBlockingIndex())
	}

	// Stream 1: indices [2, 5] -> min is 2
	manager.OnHeaderBlockSent(1, []uint64{2, 5})
	if manager.SmallestBlockingIndex() != 2 {
		t.Errorf("expected SmallestBlockingIndex == 2, got %d", manager.SmallestBlockingIndex())
	}

	// Stream 2: indices [4, 7] -> min is 4
	manager.OnHeaderBlockSent(2, []uint64{4, 7})
	if manager.SmallestBlockingIndex() != 2 {
		t.Errorf("expected SmallestBlockingIndex == 2, got %d", manager.SmallestBlockingIndex())
	}

	// Stream 3: indices [1, 8] -> min is 1
	manager.OnHeaderBlockSent(3, []uint64{1, 8})
	if manager.SmallestBlockingIndex() != 1 {
		t.Errorf("expected SmallestBlockingIndex == 1, got %d", manager.SmallestBlockingIndex())
	}

	// Ack stream 3 -> remaining min indices are 2 and 4
	if !manager.OnHeaderAcknowledgement(3) {
		t.Fatalf("ack stream 3 failed")
	}
	if manager.SmallestBlockingIndex() != 2 {
		t.Errorf("expected SmallestBlockingIndex == 2, got %d", manager.SmallestBlockingIndex())
	}

	// Ack stream 1 -> remaining min index is 4
	if !manager.OnHeaderAcknowledgement(1) {
		t.Fatalf("ack stream 1 failed")
	}
	if manager.SmallestBlockingIndex() != 4 {
		t.Errorf("expected SmallestBlockingIndex == 4, got %d", manager.SmallestBlockingIndex())
	}

	// Ack stream 2 -> no remaining blocks
	if !manager.OnHeaderAcknowledgement(2) {
		t.Fatalf("ack stream 2 failed")
	}
	if manager.SmallestBlockingIndex() != math.MaxUint64 {
		t.Errorf("expected SmallestBlockingIndex == MaxUint64, got %d", manager.SmallestBlockingIndex())
	}
}

// Test 9: HeaderAcknowledgementsOnSingleStream
func TestBlockingManager_HeaderAcknowledgementsOnSingleStream(t *testing.T) {
	manager := NewBlockingManager()
	// Stream 1 sends block A with index 2
	manager.OnHeaderBlockSent(1, []uint64{2})
	// Stream 1 sends block B with index 1
	manager.OnHeaderBlockSent(1, []uint64{1})
	// Stream 1 sends block C with index 5
	manager.OnHeaderBlockSent(1, []uint64{5})

	if manager.SmallestBlockingIndex() != 1 {
		t.Errorf("expected SmallestBlockingIndex == 1, got %d", manager.SmallestBlockingIndex())
	}

	// First ack: pops block A (index 2). Remaining are 1 and 5.
	if !manager.OnHeaderAcknowledgement(1) {
		t.Fatalf("first ack failed")
	}
	if manager.SmallestBlockingIndex() != 1 {
		t.Errorf("expected SmallestBlockingIndex == 1, got %d", manager.SmallestBlockingIndex())
	}

	// Second ack: pops block B (index 1). Remaining is 5.
	if !manager.OnHeaderAcknowledgement(1) {
		t.Fatalf("second ack failed")
	}
	if manager.SmallestBlockingIndex() != 5 {
		t.Errorf("expected SmallestBlockingIndex == 5, got %d", manager.SmallestBlockingIndex())
	}

	// Third ack: pops block C (index 5). No remaining blocks.
	if !manager.OnHeaderAcknowledgement(1) {
		t.Fatalf("third ack failed")
	}
	if manager.SmallestBlockingIndex() != math.MaxUint64 {
		t.Errorf("expected SmallestBlockingIndex == MaxUint64, got %d", manager.SmallestBlockingIndex())
	}

	// Fourth ack: should fail
	if manager.OnHeaderAcknowledgement(1) {
		t.Errorf("fourth ack should have failed")
	}
}

// Test 10: CancelStream
func TestBlockingManager_CancelStream(t *testing.T) {
	manager := NewBlockingManager()
	manager.OnHeaderBlockSent(1, []uint64{2, 5})
	manager.OnHeaderBlockSent(2, []uint64{1, 4})

	if manager.SmallestBlockingIndex() != 1 {
		t.Errorf("expected SmallestBlockingIndex == 1, got %d", manager.SmallestBlockingIndex())
	}
	if manager.NumBlockedStreams() != 2 {
		t.Errorf("expected 2 blocked streams, got %d", manager.NumBlockedStreams())
	}

	manager.OnStreamCancellation(2)
	if manager.IsBlocked(2) {
		t.Errorf("stream 2 should not be blocked after cancellation")
	}
	if manager.NumBlockedStreams() != 1 {
		t.Errorf("expected 1 blocked stream, got %d", manager.NumBlockedStreams())
	}
	if manager.SmallestBlockingIndex() != 2 {
		t.Errorf("expected SmallestBlockingIndex == 2, got %d", manager.SmallestBlockingIndex())
	}
	if manager.KnownReceivedCount() != 0 {
		t.Errorf("KnownReceivedCount should still be 0, got %d", manager.KnownReceivedCount())
	}

	manager.OnStreamCancellation(1)
	if manager.IsBlocked(1) {
		t.Errorf("stream 1 should not be blocked after cancellation")
	}
	if manager.NumBlockedStreams() != 0 {
		t.Errorf("expected 0 blocked streams, got %d", manager.NumBlockedStreams())
	}
	if manager.SmallestBlockingIndex() != math.MaxUint64 {
		t.Errorf("expected SmallestBlockingIndex == MaxUint64, got %d", manager.SmallestBlockingIndex())
	}
}

// Test 11: BlockingAllowedOnStream
func TestBlockingManager_BlockingAllowedOnStream(t *testing.T) {
	manager := NewBlockingManager()
	maxBlocked := uint64(2)

	if !manager.BlockingAllowedOnStream(1, maxBlocked) {
		t.Errorf("expected blocking allowed on stream 1")
	}

	// Stream 1 blocks
	manager.OnHeaderBlockSent(1, []uint64{0})
	if manager.NumBlockedStreams() != 1 {
		t.Errorf("expected 1 blocked stream, got %d", manager.NumBlockedStreams())
	}
	if !manager.BlockingAllowedOnStream(2, maxBlocked) {
		t.Errorf("expected blocking allowed on stream 2 (1 < 2)")
	}
	if !manager.BlockingAllowedOnStream(1, maxBlocked) {
		t.Errorf("expected blocking allowed on stream 1 (already blocked)")
	}

	// Stream 2 blocks
	manager.OnHeaderBlockSent(2, []uint64{1})
	if manager.NumBlockedStreams() != 2 {
		t.Errorf("expected 2 blocked streams, got %d", manager.NumBlockedStreams())
	}
	// Stream 3 is not blocked, and num_blocked == maxBlocked -> NOT allowed
	if manager.BlockingAllowedOnStream(3, maxBlocked) {
		t.Errorf("stream 3 should not be allowed to block (2 >= 2)")
	}
	// Streams 1 and 2 are ALREADY blocked -> STILL allowed
	if !manager.BlockingAllowedOnStream(1, maxBlocked) {
		t.Errorf("already blocked stream 1 should still be allowed")
	}
	if !manager.BlockingAllowedOnStream(2, maxBlocked) {
		t.Errorf("already blocked stream 2 should still be allowed")
	}

	// Acknowledge stream 1 -> num_blocked becomes 1
	if !manager.OnHeaderAcknowledgement(1) {
		t.Fatalf("ack stream 1 failed")
	}
	if manager.NumBlockedStreams() != 1 {
		t.Errorf("expected 1 blocked stream, got %d", manager.NumBlockedStreams())
	}
	if !manager.BlockingAllowedOnStream(3, maxBlocked) {
		t.Errorf("stream 3 should now be allowed to block (1 < 2)")
	}
}

// Test 12: InsertCountIncrementOverflow
func TestBlockingManager_InsertCountIncrementOverflow(t *testing.T) {
	manager := NewBlockingManager()
	if !manager.OnInsertCountIncrement(math.MaxUint64) {
		t.Fatalf("increment to MaxUint64 failed")
	}
	if manager.KnownReceivedCount() != math.MaxUint64 {
		t.Errorf("expected MaxUint64, got %d", manager.KnownReceivedCount())
	}

	// Incrementing further should fail (overflow)
	if manager.OnInsertCountIncrement(1) {
		t.Errorf("expected overflow error on incrementing beyond MaxUint64")
	}
	if manager.KnownReceivedCount() != math.MaxUint64 {
		t.Errorf("expected KnownReceivedCount to remain MaxUint64, got %d", manager.KnownReceivedCount())
	}
}

// Test 13: IndexSet
func TestBlockingManager_IndexSet(t *testing.T) {
	set := NewIndexSet()
	if !set.Empty() {
		t.Errorf("expected empty set")
	}
	if set.Size() != 0 {
		t.Errorf("expected size 0, got %d", set.Size())
	}
	if set.RequiredInsertCount() != 0 {
		t.Errorf("expected RIC 0, got %d", set.RequiredInsertCount())
	}
	if _, ok := set.MinIndex(); ok {
		t.Errorf("expected no min index on empty set")
	}
	if _, ok := set.MaxIndex(); ok {
		t.Errorf("expected no max index on empty set")
	}

	set.Insert(5)
	if set.Empty() {
		t.Errorf("set should not be empty")
	}
	if set.Size() != 1 {
		t.Errorf("expected size 1, got %d", set.Size())
	}
	if min, ok := set.MinIndex(); !ok || min != 5 {
		t.Errorf("expected min 5, got %d", min)
	}
	if max, ok := set.MaxIndex(); !ok || max != 5 {
		t.Errorf("expected max 5, got %d", max)
	}
	if set.RequiredInsertCount() != 6 {
		t.Errorf("expected RIC 6, got %d", set.RequiredInsertCount())
	}

	set.Insert(2)
	if min, ok := set.MinIndex(); !ok || min != 2 {
		t.Errorf("expected min 2, got %d", min)
	}
	if max, ok := set.MaxIndex(); !ok || max != 5 {
		t.Errorf("expected max 5, got %d", max)
	}
	if set.RequiredInsertCount() != 6 {
		t.Errorf("expected RIC 6, got %d", set.RequiredInsertCount())
	}

	set.Insert(10)
	if min, ok := set.MinIndex(); !ok || min != 2 {
		t.Errorf("expected min 2, got %d", min)
	}
	if max, ok := set.MaxIndex(); !ok || max != 10 {
		t.Errorf("expected max 10, got %d", max)
	}
	if set.RequiredInsertCount() != 11 {
		t.Errorf("expected RIC 11, got %d", set.RequiredInsertCount())
	}

	indices := set.Indices()
	if len(indices) != 3 || indices[0] != 5 || indices[1] != 2 || indices[2] != 10 {
		t.Errorf("unexpected indices slice: %v", indices)
	}
}

// Test 14: OnHeaderBlockReceived and OnSectionAck
func TestBlockingManager_HeaderBlockReceivedAndSectionAck(t *testing.T) {
	manager := NewBlockingManager(2)
	manager.SetMaxBlockedStreams(2)
	if manager.MaxBlockedStreams() != 2 {
		t.Errorf("expected MaxBlockedStreams == 2, got %d", manager.MaxBlockedStreams())
	}

	// Stream 1 received with RIC 3 (known is 0 -> blocked)
	isBlocked := manager.OnHeaderBlockReceived(1, 3)
	if !isBlocked {
		t.Errorf("stream 1 should be blocked")
	}
	if !manager.IsBlocked(1) {
		t.Errorf("IsBlocked(1) should be true")
	}

	// Stream 2 received with RIC 0 (known is 0 -> not blocked)
	isBlocked = manager.OnHeaderBlockReceived(2, 0)
	if isBlocked {
		t.Errorf("stream 2 should not be blocked with RIC 0")
	}
	if manager.IsBlocked(2) {
		t.Errorf("IsBlocked(2) should be false")
	}

	// Section ack on stream 1 advances knownReceivedCount to 3 and unblocks stream 1
	if !manager.OnSectionAck(1) {
		t.Fatalf("OnSectionAck(1) failed")
	}
	if manager.KnownReceivedCount() != 3 {
		t.Errorf("expected KnownReceivedCount == 3, got %d", manager.KnownReceivedCount())
	}
	if manager.IsBlocked(1) {
		t.Errorf("stream 1 should be unblocked")
	}

	// Section ack on stream 2
	if !manager.OnSectionAck(2) {
		t.Fatalf("OnSectionAck(2) failed")
	}

	// Invalid ack on stream 3
	if manager.OnSectionAck(3) {
		t.Errorf("OnSectionAck on unrecorded stream should return false")
	}
}
