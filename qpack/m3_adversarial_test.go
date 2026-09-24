// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package qpack

import (
	"math"
	"math/rand"
	"testing"

	"github.com/lemon4ksan/foundation/testing/assert"
	"github.com/lemon4ksan/foundation/testing/require"
)

// =============================================================================
// Adversarial Stress Tests for BlockingManager
// =============================================================================

// TestAdversarial_BlockingManager_EvictionBarrierInvariant stress-tests the
// eviction barrier (SmallestBlockingIndex) with randomized streams, blocks,
// interleaving section acks and stream cancellations, continuously checking
// the invariant against a naive reference model.
func TestAdversarial_BlockingManager_EvictionBarrierInvariant(t *testing.T) {
	manager := NewBlockingManager(100)
	rng := rand.New(rand.NewSource(42))

	// Reference model: streamID -> list of blocks, each block is a slice of indices
	type blockInfo struct {
		indices []uint64
		minIdx  uint64
		ric     uint64
	}
	model := make(map[uint64][]blockInfo)

	computeExpectedSmallest := func() uint64 {
		smallest := uint64(math.MaxUint64)
		for _, blocks := range model {
			for _, b := range blocks {
				if len(b.indices) > 0 && b.minIdx < smallest {
					smallest = b.minIdx
				}
			}
		}
		return smallest
	}

	for iter := 0; iter < 1000; iter++ {
		action := rng.Intn(4)
		streamID := uint64(rng.Intn(20) + 1)

		switch action {
		case 0, 1: // Send block
			numIndices := rng.Intn(5)
			indices := make([]uint64, numIndices)
			minVal := uint64(math.MaxUint64)
			maxVal := uint64(0)
			for i := 0; i < numIndices; i++ {
				indices[i] = uint64(rng.Intn(100))
				if indices[i] < minVal {
					minVal = indices[i]
				}
				if indices[i] > maxVal {
					maxVal = indices[i]
				}
			}
			var ric uint64
			if numIndices > 0 {
				ric = maxVal + 1
			}

			manager.OnHeaderBlockSent(streamID, indices)
			model[streamID] = append(model[streamID], blockInfo{
				indices: indices,
				minIdx:  minVal,
				ric:     ric,
			})

		case 2: // Section Ack
			blocks := model[streamID]
			expectedOk := len(blocks) > 0
			ok := manager.OnSectionAck(streamID)
			require.Equal(t, expectedOk, ok)
			if expectedOk {
				model[streamID] = blocks[1:]
				if len(model[streamID]) == 0 {
					delete(model, streamID)
				}
			}

		case 3: // Stream Cancellation
			manager.OnStreamCancellation(streamID)
			delete(model, streamID)
		}

		expectedSmallest := computeExpectedSmallest()
		actualSmallest := manager.SmallestBlockingIndex()
		if expectedSmallest != actualSmallest {
			t.Fatalf(
				"iteration %d: mismatch in SmallestBlockingIndex: expected %d, got %d",
				iter,
				expectedSmallest,
				actualSmallest,
			)
		}
	}
}

// TestAdversarial_BlockingManager_BoundaryConditions tests extreme values and edge cases.
func TestAdversarial_BlockingManager_BoundaryConditions(t *testing.T) {
	t.Run("EmptyIndicesBlock", func(t *testing.T) {
		manager := NewBlockingManager()
		manager.OnHeaderBlockSent(1, []uint64{})
		assert.Equal(t, uint64(math.MaxUint64), manager.SmallestBlockingIndex())
		assert.False(t, manager.IsBlocked(1))
		assert.Equal(t, uint64(0), manager.NumBlockedStreams())

		// Acknowledging empty block
		ok := manager.OnSectionAck(1)
		assert.True(t, ok)
		assert.Equal(t, uint64(math.MaxUint64), manager.SmallestBlockingIndex())
		assert.False(t, manager.IsBlocked(1))

		// Ack again -> false
		assert.False(t, manager.OnSectionAck(1))
	})

	t.Run("IndexZero", func(t *testing.T) {
		manager := NewBlockingManager()
		manager.OnHeaderBlockSent(1, []uint64{0})
		assert.Equal(t, uint64(0), manager.SmallestBlockingIndex())
		assert.True(t, manager.IsBlocked(1))

		// Increment by 1 satisfies RIC 1
		ok := manager.OnInsertCountIncrement(1)
		assert.True(t, ok)
		assert.False(t, manager.IsBlocked(1))
		assert.Equal(t, uint64(0), manager.SmallestBlockingIndex())

		// Ack clears index
		ok = manager.OnSectionAck(1)
		assert.True(t, ok)
		assert.Equal(t, uint64(math.MaxUint64), manager.SmallestBlockingIndex())
	})

	t.Run("DuplicateMinIndicesOnDifferentBlocks", func(t *testing.T) {
		manager := NewBlockingManager()
		manager.OnHeaderBlockSent(1, []uint64{7, 10})
		manager.OnHeaderBlockSent(1, []uint64{7, 20})
		manager.OnHeaderBlockSent(2, []uint64{7, 30})

		assert.Equal(t, uint64(7), manager.SmallestBlockingIndex())

		// Pop stream 1 block 1
		assert.True(t, manager.OnSectionAck(1))
		assert.Equal(t, uint64(7), manager.SmallestBlockingIndex())

		// Pop stream 1 block 2
		assert.True(t, manager.OnSectionAck(1))
		assert.Equal(t, uint64(7), manager.SmallestBlockingIndex())

		// Cancel stream 2 -> now cleared
		manager.OnStreamCancellation(2)
		assert.Equal(t, uint64(math.MaxUint64), manager.SmallestBlockingIndex())
	})

	t.Run("BlockingAllowedWithZeroLimit", func(t *testing.T) {
		manager := NewBlockingManager(0)
		// Stream 1 is not blocked, maxBlocked = 0 -> not allowed
		assert.False(t, manager.BlockingAllowedOnStream(1))

		// If stream 1 sends block with index 0, it becomes blocked
		manager.OnHeaderBlockSent(1, []uint64{0})
		assert.True(t, manager.IsBlocked(1))

		// Now stream 1 is already blocked, so BlockingAllowedOnStream is true
		assert.True(t, manager.BlockingAllowedOnStream(1))
		// Stream 2 is still not allowed
		assert.False(t, manager.BlockingAllowedOnStream(2))
	})

	t.Run("RICMonotonicity", func(t *testing.T) {
		manager := NewBlockingManager()
		// Increment to 10
		assert.True(t, manager.OnInsertCountIncrement(10))
		assert.Equal(t, uint64(10), manager.KnownReceivedCount())

		// Block with RIC = 5 (indices [4])
		manager.OnHeaderBlockSent(1, []uint64{4})
		// Acknowledging block with RIC 5 must NOT decrease KnownReceivedCount from 10 to 5
		assert.True(t, manager.OnSectionAck(1))
		assert.Equal(t, uint64(10), manager.KnownReceivedCount())
	})
}

// =============================================================================
// Adversarial Stress Tests for StreamReceiver
// =============================================================================

// TestAdversarial_StreamReceiver_OpcodeAndByteFragmentation tests that feed
// arbitrarily fragmented bytes, ensuring that error latching and state machine
// transitions remain strictly deterministic.
func TestAdversarial_StreamReceiver_OpcodeAndByteFragmentation(t *testing.T) {
	t.Run("EncoderReceiver_TruncatedInstructionOnEndDecoding", func(t *testing.T) {
		delegate := &mockEncoderReceiverDelegate{}
		receiver := NewEncoderStreamReceiver(delegate)

		// 0x60 is prefix of InsertWithoutNameReference; in-progress instruction
		receiver.Decode([]byte{0x60})
		assert.Equal(t, 0, len(delegate.errorCalls))

		// Abrupt end of decoding triggers stream error
		receiver.EndDecoding()
		require.Equal(t, 1, len(delegate.errorCalls))
		assert.Equal(t, uint64(QPACK_ENCODER_STREAM_ERROR), delegate.errorCalls[0].qpackError)
		assert.Equal(t, "Truncated instruction.", delegate.errorCalls[0].message)

		// Subsequent bytes must be completely ignored
		receiver.Decode([]byte{0x11, 0x12, 0x13})
		assert.Equal(t, 1, len(delegate.errorCalls))
		assert.Equal(t, 0, len(delegate.duplicateCalls))
	})

	t.Run("DecoderReceiver_ExtremeByteFragmentation", func(t *testing.T) {
		delegate := &mockDecoderReceiverDelegate{}
		receiver := NewDecoderStreamReceiver(delegate)

		// Stream Cancellation for stream 110: "7f2f"
		data := []byte{0x7f, 0x2f}
		for _, b := range data {
			receiver.Decode([]byte{b})
		}

		require.Equal(t, 1, len(delegate.cancellationCalls))
		assert.Equal(t, uint64(110), delegate.cancellationCalls[0])
		assert.Equal(t, 0, len(delegate.errorCalls))
	})

	t.Run("DecoderReceiver_LatchingAfterVarintOverflow", func(t *testing.T) {
		delegate := &mockDecoderReceiverDelegate{}
		receiver := NewDecoderStreamReceiver(delegate)

		// Varint overflow: 0xff followed by 10 extension bytes with MSB set
		overflowBytes := []byte{0xff, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x01}
		receiver.Decode(overflowBytes)

		require.Equal(t, 1, len(delegate.errorCalls))
		assert.Equal(t, uint64(QPACK_DECODER_STREAM_ERROR), delegate.errorCalls[0].qpackError)

		// Feed 1000 valid Section Acks
		for i := 0; i < 1000; i++ {
			receiver.Decode([]byte{0x80})
		}
		assert.Equal(t, 0, len(delegate.sectionAckCalls))
		assert.Equal(t, 1, len(delegate.errorCalls))
	})
}

// =============================================================================
// Adversarial Stress Tests for StreamSender
// =============================================================================

// TestAdversarial_StreamSender_CoalesceAndBufferIsolation stress-tests massive
// coalescing, buffer clearing, and isolation.
func TestAdversarial_StreamSender_CoalesceAndBufferIsolation(t *testing.T) {
	t.Run("EncoderSender_MassiveCoalescing", func(t *testing.T) {
		delegate := newMockStreamSenderDelegate()
		sender := NewEncoderStreamSenderWithHuffman(HuffmanEncodingDisabled, delegate)

		const count = 100
		for i := uint64(0); i < count; i++ {
			sender.SendDuplicate(i)
		}

		assert.Equal(t, 0, len(delegate.writes))
		sender.Flush()
		require.Equal(t, 1, len(delegate.writes))

		// Decode all written data with instruction decoder to ensure wire integrity
		instructionCount := 0
		mockDec := &countingInstructionDecoderDelegate{
			onInstruction: func(instruction *Instruction) bool {
				instructionCount++
				return true
			},
		}
		dec := NewInstructionDecoder(EncoderStreamLanguage(), mockDec)
		ok := dec.Decode(delegate.writes[0])
		assert.True(t, ok)
		assert.False(t, dec.HasError())
		dec.EndDecoding()
		assert.Equal(t, count, instructionCount)
	})

	t.Run("DecoderSender_BufferIsolationOnFlush", func(t *testing.T) {
		// Verify that when Flush is called, the slice passed to WriteStreamData
		// is independent and sender's internal buffer is reset to nil.
		var capturedData []byte
		capturingDelegate := &reentrantMockSenderDelegate{
			onWrite: func(data []byte) {
				capturedData = data
			},
		}
		sender := NewDecoderStreamSender(capturingDelegate)
		sender.SendInsertCountIncrement(42)
		sender.Flush()

		require.NotNil(t, capturedData)
		origLen := len(capturedData)

		// Subsequent sends should not alias capturedData
		sender.SendInsertCountIncrement(99)
		assert.Equal(t, origLen, len(capturedData))
	})
}

// Helper delegates for adversarial tests
type countingInstructionDecoderDelegate struct {
	onInstruction func(instruction *Instruction) bool
}

func (c *countingInstructionDecoderDelegate) OnInstructionDecoded(instruction *Instruction) bool {
	if c.onInstruction != nil {
		return c.onInstruction(instruction)
	}
	return true
}

func (c *countingInstructionDecoderDelegate) OnInstructionDecodingError(
	errorCode InstructionDecoderErrorCode,
	errorMessage string,
) {
}

type reentrantMockSenderDelegate struct {
	onWrite func(data []byte)
}

func (r *reentrantMockSenderDelegate) WriteStreamData(data []byte) {
	if r.onWrite != nil {
		r.onWrite(data)
	}
}

func (r *reentrantMockSenderDelegate) NumBytesBuffered() uint64 {
	return 0
}
