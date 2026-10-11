// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package stream

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/lemon4ksan/mach/net/quic/core/wire"
)

// TestReproduce_UniStreamResetDeadlock demonstrates the self-deadlock when HandleResetStreamFrame
// is called on an incoming unidirectional stream.
func TestReproduce_UniStreamResetDeadlock(t *testing.T) {
	serverConn := NewConn(ConnConfig{Perspective: PerspectiveServer})
	err := serverConn.HandleStreamFrame(2, 0, []byte("hello"), false)
	if err != nil {
		t.Fatalf("HandleStreamFrame failed: %v", err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = serverConn.HandleResetStreamFrame(2, 42, 5)
	}()

	select {
	case <-done:
		t.Log("HandleResetStreamFrame completed without deadlocking")
	case <-time.After(2 * time.Second):
		t.Fatal("DEADLOCK: HandleResetStreamFrame on unidirectional stream locked Conn.mu recursively and deadlocked!")
	}
}

// TestReproduce_ClosedStreamZombieRecreation reproduces the bug where a completed, closed stream
// is resurrected as a zombie new stream upon receiving a retransmitted or delayed STREAM frame.
// RFC 9000 §2.1 & §3.2 mandate that stream IDs are never reused within a connection.
func TestReproduce_ClosedStreamZombieRecreation(t *testing.T) {
	serverConn := NewConn(ConnConfig{Perspective: PerspectiveServer})

	// Peer sends uni stream 2 with FIN
	err := serverConn.HandleStreamFrame(2, 0, []byte("test-payload"), true)
	if err != nil {
		t.Fatalf("HandleStreamFrame failed: %v", err)
	}

	// Server accepts and reads uni stream 2 to EOF
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	sUni, err := serverConn.AcceptUniStream(ctx)
	if err != nil {
		t.Fatalf("AcceptUniStream failed: %v", err)
	}
	buf := make([]byte, 32)
	n, rErr := sUni.Read(buf)
	if rErr != io.EOF {
		t.Fatalf("expected io.EOF on read, got %v", rErr)
	}
	if string(buf[:n]) != "test-payload" {
		t.Fatalf("unexpected content %q", string(buf[:n]))
	}

	// Stream 2 is now closed and completed.
	// A retransmitted STREAM frame for stream 2 arrives from the peer.
	err = serverConn.HandleStreamFrame(2, 0, []byte("test-payload"), true)
	if err != nil {
		t.Fatalf("HandleStreamFrame for retransmit failed: %v", err)
	}

	// The closed stream MUST NOT be re-queued to AcceptUniStream as a zombie stream!
	select {
	case zombieStr := <-serverConn.acceptUniCh:
		t.Fatalf("RFC 9000 violation: closed stream %d was re-created as a zombie new stream!", zombieStr.StreamID())
	default:
		// Passed: not re-queued
	}
}

// TestReproduce_ClosedLocallyInitiatedStreamRetransmitError reproduces the bug where receiving
// a retransmission on a closed locally-initiated stream triggers a fatal StreamStateError.
// In QUIC, delayed or reordered packets for closed streams MUST be safely ignored.
func TestReproduce_ClosedLocallyInitiatedStreamRetransmitError(t *testing.T) {
	clientConn := NewConn(ConnConfig{Perspective: PerspectiveClient})

	// Client opens stream 0
	str, err := clientConn.OpenStream()
	if err != nil {
		t.Fatalf("OpenStream failed: %v", err)
	}

	// Server sends response with FIN
	err = clientConn.HandleStreamFrame(str.StreamID(), 0, []byte("response"), true)
	if err != nil {
		t.Fatalf("HandleStreamFrame failed: %v", err)
	}

	// Client closes write, pops FIN frame, and receives ACK
	_ = str.Close()
	_, _, fin, ok := str.SendStream().PopStreamFrame(100)
	if !ok || !fin {
		t.Fatalf("expected FIN popped")
	}
	str.SendStream().HandleAck(true)

	buf := make([]byte, 32)
	_, _ = str.Read(buf)
	_, err = str.Read(buf)
	if err != io.EOF {
		t.Fatalf("expected io.EOF, got %v", err)
	}

	// Stream 0 is now completed and closed.
	// Server retransmits the response packet (common network duplicate).
	err = clientConn.HandleStreamFrame(str.StreamID(), 0, []byte("response"), true)
	if err != nil {
		t.Fatalf("BUG: retransmission on closed locally-initiated stream produced error: %v", err)
	}
}

// TestReproduce_ResetStreamFlowControlViolation reproduces the bug where HandleResetStreamFrame
// does not check finalSize against flow control limits (RFC 9000 §4.5).
func TestReproduce_ResetStreamFlowControlViolation(t *testing.T) {
	serverConn := NewConn(ConnConfig{
		Perspective:                PerspectiveServer,
		InitialStreamReceiveWindow: 1000,
	})

	// Incoming stream frame within limits
	err := serverConn.HandleStreamFrame(2, 0, []byte("short"), false)
	if err != nil {
		t.Fatalf("HandleStreamFrame failed: %v", err)
	}

	// Peer sends RESET_STREAM with finalSize = 500,000,000, violating initial stream limit (1000)
	// RFC 9000 §4.5 requires:
	// "A receiver MUST treat receipt of a RESET_STREAM frame that increases the stream or
	// connection flow control limits as a connection error of type FLOW_CONTROL_ERROR."
	done := make(chan struct{})
	go func() {
		defer close(done)
		err = serverConn.HandleResetStreamFrame(2, 42, 500_000_000)
	}()

	select {
	case <-done:
		if err == nil {
			t.Fatal("RFC 9000 §4.5 violation: RESET_STREAM exceeding flow control limits was accepted without FLOW_CONTROL_ERROR!")
		}
		var tErr *TransportError
		if !errors.As(err, &tErr) || tErr.ErrorCode != FlowControlError {
			t.Fatalf("expected FlowControlError, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("RFC 9000 §4.5 violation: HandleResetStreamFrame failed to validate flow control limits and deadlocked!")
	}
}

// TestReproduce_ReassemblyBufferIntegerOverflow demonstrates integer overflow in ReassemblyBuffer.Push
// when offset + len(data) exceeds MaxVarint / wraps around uint64.
func TestReproduce_ReassemblyBufferIntegerOverflow(t *testing.T) {
	rb := NewReassemblyBuffer()
	_ = rb.Push(0, []byte("valid"), false)

	// RFC 9000 §4.5: offsets are varints, max offset is 2^62 - 1.
	// Frame with offset = 2^62 - 5 and len = 10 exceeds 2^62 - 1.
	err := rb.Push((1<<62)-5, make([]byte, 10), false)
	if err == nil {
		t.Fatal("RFC 9000 §4.5 violation: frame exceeding 2^62 - 1 allowed into reassembly buffer")
	}

	// Frame wrapping uint64: offset = ^uint64(0)
	err = rb.Push(^uint64(0), []byte("wrap"), false)
	if err == nil {
		t.Fatal("expected error on uint64 integer overflow offset, got nil")
	}
}

// TestReproduce_DecodePacketNumberExceedsMax demonstrates DecodePacketNumber returning
// a packet number >= 2^62 when largestPN is at MaxPacketNumber.
func TestReproduce_DecodePacketNumberExceedsMax(t *testing.T) {
	// RFC 9000 §17.1: Packet numbers are in the range 0 to 2^62 - 1.
	largestPN := wire.MaxPacketNumber // (1 << 62) - 1
	pn := wire.DecodePacketNumber(largestPN, 0, 1)
	if pn > wire.MaxPacketNumber {
		t.Fatalf("RFC 9000 §17.1 violation: DecodePacketNumber returned %d (exceeds MaxPacketNumber %d)", pn, wire.MaxPacketNumber)
	}
}

// TestReproduce_ResetCreditsConnFlowControl demonstrates that resetting a stream
// credits unread bytes to the connection flow controller, preventing flow control starvation.
func TestReproduce_ResetCreditsConnFlowControl(t *testing.T) {
	conn := NewConn(ConnConfig{
		Perspective:                    PerspectiveServer,
		InitialConnectionReceiveWindow: 1000,
		MaxConnectionReceiveWindow:     2000,
	})

	// Stream 2 receives 500 bytes (half of connection window)
	err := conn.HandleStreamFrame(2, 0, make([]byte, 500), false)
	if err != nil {
		t.Fatalf("HandleStreamFrame failed: %v", err)
	}

	// Peer resets stream 2 with finalSize = 500
	err = conn.HandleResetStreamFrame(2, 1, 500)
	if err != nil {
		t.Fatalf("HandleResetStreamFrame failed: %v", err)
	}

	// The unread 500 bytes must be credited to connection flow controller
	if conn.connFlowController.bytesRead < 500 {
		t.Fatalf("connection flow controller bytesRead=%d, expected >= 500", conn.connFlowController.bytesRead)
	}
}

// TestStress_ConcurrentStreamCreationAndReset stress tests high concurrent stream allocation,
// writing, resetting, and closing to ensure no deadlocks or data races.
func TestStress_ConcurrentStreamCreationAndReset(t *testing.T) {
	clientConn := NewConn(ConnConfig{
		Perspective:               PerspectiveClient,
		InitialMaxStreamsBidi:     10000,
		InitialPeerMaxStreamsBidi: 10000,
	})

	var wg sync.WaitGroup
	const numGoroutines = 40
	const numOps = 25

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(gid int) {
			defer wg.Done()
			for j := 0; j < numOps; j++ {
				str, err := clientConn.OpenStream()
				if err != nil {
					return
				}
				if (gid+j)%2 == 0 {
					_, _ = str.Write([]byte("quick data"))
					str.CancelWrite(10)
				} else {
					_, _ = str.Write([]byte("quick data"))
					_ = str.Close()
					_, _, _, _ = str.SendStream().PopStreamFrame(100)
					str.SendStream().HandleAck(true)
				}
			}
		}(i)
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Passed without deadlock
	case <-time.After(10 * time.Second):
		t.Fatal("DEADLOCK in concurrent stream creation/reset/close")
	}
}

