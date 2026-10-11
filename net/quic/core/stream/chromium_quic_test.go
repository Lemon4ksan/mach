// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package stream

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
)

// TestChromium_StreamReassembly_OverlappingDivergentData verifies RFC 9000 §2.2:
// Receipt of overlapping stream frames with divergent payload data MUST be treated
// as a PROTOCOL_VIOLATION connection error (ported from Chromium QuicStreamSequencerBufferTest).
func TestChromium_StreamReassembly_OverlappingDivergentData(t *testing.T) {
	t.Run("PartialOverlap_DivergentEnd", func(t *testing.T) {
		rb := NewReassemblyBuffer()
		if err := rb.Push(0, []byte("abcdefghij"), false); err != nil {
			t.Fatalf("unexpected push error: %v", err)
		}

		// Overlap at offset 5..15: bytes 5..9 should be "fghij", but send "fgXij" (offset 7 divergent)
		divergentData := []byte("fgXijklmno")
		err := rb.Push(5, divergentData, false)
		if err == nil {
			t.Fatalf("expected protocol violation for divergent overlapping data, got nil")
		}

		var te *TransportError
		if !errors.As(err, &te) || te.ErrorCode != ProtocolViolation {
			t.Fatalf("expected ProtocolViolation error, got: %v", err)
		}
	})

	t.Run("PartialOverlap_DivergentBeginning", func(t *testing.T) {
		rb := NewReassemblyBuffer()
		if err := rb.Push(10, []byte("0123456789"), false); err != nil {
			t.Fatalf("unexpected push error: %v", err)
		}

		// Overlap at offset 5..15: bytes 10..14 should be "01234", but send "01Z34"
		divergentData := []byte("abcde01Z34")
		err := rb.Push(5, divergentData, false)
		if err == nil {
			t.Fatalf("expected protocol violation for divergent overlapping data, got nil")
		}

		var te *TransportError
		if !errors.As(err, &te) || te.ErrorCode != ProtocolViolation {
			t.Fatalf("expected ProtocolViolation error, got: %v", err)
		}
	})

	t.Run("ContainedDuplicate_ExactMatch", func(t *testing.T) {
		rb := NewReassemblyBuffer()
		if err := rb.Push(0, []byte("0123456789abcdef"), false); err != nil {
			t.Fatalf("unexpected push error: %v", err)
		}

		// Subsegment identical duplicate at 4..10
		if err := rb.Push(4, []byte("456789"), false); err != nil {
			t.Fatalf("identical contained duplicate should succeed, got: %v", err)
		}

		buf := make([]byte, 32)
		n, err := rb.Read(buf)
		if err != nil && err != io.EOF {
			t.Fatalf("unexpected read error: %v", err)
		}
		if string(buf[:n]) != "0123456789abcdef" {
			t.Fatalf("unexpected content: %q", string(buf[:n]))
		}
	})

	t.Run("SupersetChunk_ExactMatchOverlap", func(t *testing.T) {
		rb := NewReassemblyBuffer()
		// Push middle chunk first
		if err := rb.Push(5, []byte("56789"), false); err != nil {
			t.Fatalf("unexpected push error: %v", err)
		}

		// Push superset spanning 0..15 where 5..9 matches exactly
		if err := rb.Push(0, []byte("0123456789abcde"), true); err != nil {
			t.Fatalf("matching superset chunk should succeed, got: %v", err)
		}

		buf := make([]byte, 32)
		n, err := rb.Read(buf)
		if err != io.EOF {
			t.Fatalf("expected io.EOF, got %v", err)
		}
		if string(buf[:n]) != "0123456789abcde" {
			t.Fatalf("unexpected content: %q", string(buf[:n]))
		}
	})
}

// TestChromium_StreamReassembly_GapsAndDuplicates tests adversarial out-of-order,
// duplicate, and gapped stream frame arrivals (ported from Chromium QuicStreamSequencerBufferTest).
func TestChromium_StreamReassembly_GapsAndDuplicates(t *testing.T) {
	rb := NewReassemblyBuffer()

	// Target message: 64 bytes
	target := make([]byte, 64)
	for i := range target {
		target[i] = byte('A' + (i % 26))
	}

	// Chunks with gaps and duplicates
	// Order: [32..48], [48..64 FIN], [32..48 duplicate], [16..32], [0..16]
	if err := rb.Push(32, target[32:48], false); err != nil {
		t.Fatalf("push [32..48] failed: %v", err)
	}
	if err := rb.Push(48, target[48:64], true); err != nil {
		t.Fatalf("push [48..64 FIN] failed: %v", err)
	}
	// Redundant duplicate push
	if err := rb.Push(32, target[32:48], false); err != nil {
		t.Fatalf("duplicate push [32..48] failed: %v", err)
	}
	// Missing middle gap
	if err := rb.Push(16, target[16:32], false); err != nil {
		t.Fatalf("push [16..32] failed: %v", err)
	}

	// At this point, offset 0..16 is missing; read should return 0 bytes
	temp := make([]byte, 64)
	n, err := rb.Read(temp)
	if n != 0 || err != nil {
		t.Fatalf("expected 0 bytes before offset 0 arrives, got n=%d err=%v", n, err)
	}

	// Deliver first segment [0..16]
	if err := rb.Push(0, target[0:16], false); err != nil {
		t.Fatalf("push [0..16] failed: %v", err)
	}

	// Read entire stream
	n, err = rb.Read(temp)
	if err != io.EOF {
		t.Fatalf("expected io.EOF, got %v", err)
	}
	if n != 64 || !bytes.Equal(temp[:n], target) {
		t.Fatalf("reassembled content mismatch: got %d bytes, expected 64", n)
	}
}

// TestChromium_FlowControl_CreditExhaustionAndAutoTuning verifies credit exhaustion,
// BLOCKED frame signaling, window update emission, and stall recovery (ported from Chromium QuicFlowControllerTest).
func TestChromium_FlowControl_CreditExhaustionAndAutoTuning(t *testing.T) {
	connFC := NewConnectionFlowController(1000, 4000, 1000, nil, nil)
	streamFC := NewStreamFlowController(0, connFC, 500, 2000, 500, nil, nil)

	// 1. Consume up to stream limit (500 bytes)
	streamFC.AddBytesSent(500)
	_, _, credit := streamFC.SendCreditAvailable()
	if credit != 0 {
		t.Fatalf("expected 0 credit at send limit, got %d", credit)
	}

	blockedOffset, ok := streamFC.GetStreamDataBlockedFrame()
	if !ok || blockedOffset != 500 {
		t.Fatalf("expected STREAM_DATA_BLOCKED frame at 500, got ok=%v, offset=%d", ok, blockedOffset)
	}

	// Subsequent call should not emit duplicate blocked frame until offset increases
	_, ok = streamFC.GetStreamDataBlockedFrame()
	if ok {
		t.Fatalf("duplicate STREAM_DATA_BLOCKED frame emitted without state change")
	}

	// 2. Unblock stream via MAX_STREAM_DATA (window update to 1000)
	streamFC.UpdateSendWindow(1000)
	_, _, credit = streamFC.SendCreditAvailable()
	if credit != 500 {
		t.Fatalf("expected 500 credit after UpdateSendWindow(1000), got %d", credit)
	}

	// 3. Receive side auto-tuning and window update emission
	streamFC.AddBytesRead(300) // Consumed > 50% of 500 byte window
	streamFC.CheckAndQueueWindowUpdate()

	update, hasUpdate := streamFC.GetWindowUpdate()
	if !hasUpdate || update < 800 {
		t.Fatalf("expected queued MAX_STREAM_DATA window update, got hasUpdate=%v update=%d", hasUpdate, update)
	}

	// After consuming update, GetWindowUpdate should return false
	_, hasUpdate = streamFC.GetWindowUpdate()
	if hasUpdate {
		t.Fatalf("expected window update to be consumed")
	}
}

// TestChromium_ResetStream_StopSending_BufferedEgressRace verifies that when a stream
// is cancelled or stopped while egress data is buffered in user space, the RESET_STREAM
// Final Size matches sentOffset (exact bytes on the wire) and unsent credits are refunded
// (RFC 9000 §4.5, ported from Chromium QuicChromiumClientStreamTest).
func TestChromium_ResetStream_StopSending_BufferedEgressRace(t *testing.T) {
	connFC := NewConnectionFlowController(10000, 40000, 10000, nil, nil)
	streamFC := NewStreamFlowController(4, connFC, 10000, 40000, 10000, nil, nil)

	s := NewSendStream(context.Background(), 4, newDummySender(), streamFC)

	// Write 1000 bytes into stream buffer
	data := bytes.Repeat([]byte("X"), 1000)
	n, err := s.Write(data)
	if err != nil || n != 1000 {
		t.Fatalf("write failed: n=%d, err=%v", n, err)
	}

	// Packetize only 300 bytes onto the wire
	frameData, offset, _, ok := s.PopStreamFrame(300)
	if !ok || len(frameData) != 300 || offset != 0 {
		t.Fatalf("expected 300 byte frame at offset 0, got ok=%v len=%d offset=%d", ok, len(frameData), offset)
	}

	// Connection flow controller now accounts for 1000 bytes sent
	if connFC.bytesSent != 1000 {
		t.Fatalf("expected connFC.bytesSent=1000, got %d", connFC.bytesSent)
	}

	// CancelWrite (simulates local reset)
	s.CancelWrite(42)

	// RFC 9000 §4.5: Final Size MUST be 300 (sentOffset), NOT 1000 (writeOffset)
	code, finalSize, hasRst := s.GetResetStreamFrame()
	if !hasRst || finalSize != 300 || code != 42 {
		t.Fatalf("expected RESET_STREAM with FinalSize=300 code=42, got hasRst=%v finalSize=%d code=%d", hasRst, finalSize, code)
	}

	// Unsent 700 bytes MUST be reclaimed by connection flow controller
	if connFC.bytesSent != 300 {
		t.Fatalf("expected connFC.bytesSent refunded to 300, got %d", connFC.bytesSent)
	}

	// No further stream frames should be generated from buffered data
	_, _, _, hasFrame := s.PopStreamFrame(500)
	if hasFrame {
		t.Fatalf("unexpected stream frame emitted after stream reset")
	}
}
