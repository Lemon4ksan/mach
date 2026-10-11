// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package stream

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"
)

type dummySender struct {
	mu           sync.Mutex
	dataEvents   []StreamID
	ctrlEvents   []StreamID
	streamDone   []StreamID
	onDoneSignal chan StreamID
}

func newDummySender() *dummySender {
	return &dummySender{
		onDoneSignal: make(chan StreamID, 10),
	}
}

func (d *dummySender) OnHasStreamData(id StreamID) {
	d.mu.Lock()
	d.dataEvents = append(d.dataEvents, id)
	d.mu.Unlock()
}

func (d *dummySender) OnHasStreamControlFrame(id StreamID) {
	d.mu.Lock()
	d.ctrlEvents = append(d.ctrlEvents, id)
	d.mu.Unlock()
}

func (d *dummySender) OnStreamCompleted(id StreamID) {
	d.mu.Lock()
	d.streamDone = append(d.streamDone, id)
	d.mu.Unlock()
	select {
	case d.onDoneSignal <- id:
	default:
	}
}

func TestStream_ConcurrentReadWriteRace(t *testing.T) {
	cfc := NewConnectionFlowController(10*1024*1024, 20*1024*1024, 10*1024*1024, nil, nil)
	sfc := NewStreamFlowController(0, cfc, 10*1024*1024, 20*1024*1024, 10*1024*1024, nil, nil)
	sender := newDummySender()

	str := NewStream(context.Background(), 0, sender, sfc, nil)

	dataLen := 256 * 1024 // 256 KB
	payload := make([]byte, dataLen)
	for i := range payload {
		payload[i] = byte(i % 251)
	}

	var wg sync.WaitGroup
	wg.Add(2)

	// Writer goroutine
	go func() {
		defer wg.Done()
		chunkSize := 4096
		for i := 0; i < dataLen; i += chunkSize {
			end := min(i+chunkSize, dataLen)
			_, err := str.Write(payload[i:end])
			if err != nil {
				t.Errorf("write error: %v", err)
				return
			}
		}
		_ = str.Close()
	}()

	// Loop pipeline: pipe popped send frames into the receive half
	go func() {
		for {
			data, offset, fin, ok := str.SendStream().PopStreamFrame(8192)
			if !ok {
				if str.SendStream().State().IsTerminal() || str.SendStream().State() == SendStateDataSent {
					// Check if all data popped
					if str.SendStream().sentOffset >= uint64(dataLen) {
						break
					}
				}
				time.Sleep(100 * time.Microsecond)
				continue
			}
			if len(data) > 0 || fin {
				_ = str.ReceiveStream().HandleStreamFrame(offset, data, fin)
			}
			if fin {
				break
			}
		}
	}()

	// Reader goroutine
	receivedData := make([]byte, 0, dataLen)
	go func() {
		defer wg.Done()
		buf := make([]byte, 2048)
		for {
			n, err := str.Read(buf)
			if n > 0 {
				receivedData = append(receivedData, buf[:n]...)
			}
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Errorf("read error: %v", err)
				return
			}
		}
	}()

	wg.Wait()

	if len(receivedData) != dataLen {
		t.Fatalf("expected %d bytes, got %d", dataLen, len(receivedData))
	}
	if !bytes.Equal(receivedData, payload) {
		t.Fatalf("payload content mismatch")
	}
}

func TestStream_Deadlines(t *testing.T) {
	cfc := NewConnectionFlowController(1000, 2000, 1000, nil, nil)
	sfc := NewStreamFlowController(0, cfc, 1000, 2000, 1000, nil, nil)
	sender := newDummySender()

	str := NewStream(context.Background(), 0, sender, sfc, nil)

	// Past read deadline
	err := str.SetReadDeadline(time.Now().Add(-1 * time.Second))
	if err != nil {
		t.Fatalf("SetReadDeadline error: %v", err)
	}

	buf := make([]byte, 10)
	_, err = str.Read(buf)
	if !errors.Is(err, ErrDeadline) {
		t.Fatalf("expected ErrDeadline, got %v", err)
	}

	// Reset read deadline to future and timeout
	_ = str.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
	start := time.Now()
	_, err = str.Read(buf)
	elapsed := time.Since(start)
	if !errors.Is(err, ErrDeadline) {
		t.Fatalf("expected ErrDeadline after timeout, got %v", err)
	}
	if elapsed < 40*time.Millisecond {
		t.Fatalf("timeout fired too early: %v", elapsed)
	}

	// Past write deadline
	_ = str.SetWriteDeadline(time.Now().Add(-1 * time.Second))
	_, err = str.Write([]byte("hello"))
	if !errors.Is(err, ErrDeadline) {
		t.Fatalf("expected ErrDeadline on write, got %v", err)
	}
}

func TestStream_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cfc := NewConnectionFlowController(1000, 2000, 1000, nil, nil)
	sfc := NewStreamFlowController(0, cfc, 1000, 2000, 1000, nil, nil)
	sender := newDummySender()

	str := NewStream(ctx, 0, sender, sfc, nil)

	readErrCh := make(chan error, 1)
	go func() {
		buf := make([]byte, 10)
		_, err := str.Read(buf)
		readErrCh <- err
	}()

	time.Sleep(20 * time.Millisecond)
	cancel() // cancel parent context

	select {
	case err := <-readErrCh:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context.Canceled, got %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("read did not unblock on context cancellation")
	}
}

func TestStream_CancellationAndStopSending(t *testing.T) {
	cfc := NewConnectionFlowController(1000, 2000, 1000, nil, nil)
	sfc := NewStreamFlowController(0, cfc, 1000, 2000, 1000, nil, nil)
	sender := newDummySender()

	str := NewStream(context.Background(), 0, sender, sfc, nil)

	// CancelWrite
	str.CancelWrite(42)
	if str.SendStream().State() != SendStateResetSent {
		t.Fatalf("expected SendStateResetSent, got %s", str.SendStream().State())
	}
	code, finalSize, ok := str.SendStream().GetResetStreamFrame()
	if !ok || code != 42 || finalSize != 0 {
		t.Fatalf("unexpected reset frame: code=%d, finalSize=%d, ok=%v", code, finalSize, ok)
	}

	// Subsequent write returns StreamError
	_, err := str.Write([]byte("fail"))
	var sErr *StreamError
	if !errors.As(err, &sErr) || sErr.ErrorCode != 42 {
		t.Fatalf("expected StreamError with code 42, got %v", err)
	}

	// CancelRead
	str.CancelRead(99)
	if str.ReceiveStream().State() != RecvStateResetRead {
		t.Fatalf("expected RecvStateResetRead, got %s", str.ReceiveStream().State())
	}
	stopCode, ok := str.ReceiveStream().GetStopSendingFrame()
	if !ok || stopCode != 99 {
		t.Fatalf("unexpected STOP_SENDING: code=%d, ok=%v", stopCode, ok)
	}
}

func TestSendStream_TryWriteAll_and_WriteWithLimit(t *testing.T) {
	cfc := NewConnectionFlowController(1000, 2000, 100, nil, nil)
	sfc := NewStreamFlowController(0, cfc, 1000, 2000, 100, nil, nil)
	sender := newDummySender()

	ss := NewSendStream(context.Background(), 0, sender, sfc)

	// TryWriteAll with payload exceeding available credit (150 > 100)
	err := ss.TryWriteAll(make([]byte, 150))
	if !errors.Is(err, ErrWouldBlock) {
		t.Fatalf("expected ErrWouldBlock, got %v", err)
	}

	// TryWriteAll fitting within credit
	err = ss.TryWriteAll(make([]byte, 60))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// WriteWithLimit
	limiter := func(maxBytes int) int {
		return 10 // only allow 10 bytes
	}
	n, err := ss.WriteWithLimit(make([]byte, 40), limiter)
	if err != nil {
		t.Fatalf("WriteWithLimit unexpected error: %v", err)
	}
	if n != 40 { // loops until all 40 written in 10-byte chunks
		t.Fatalf("expected 40 bytes written, got %d", n)
	}

	// Now send credit is 0 (60 + 40 = 100)
	zeroLimiter := func(maxBytes int) int {
		return 0
	}
	sfc.UpdateSendWindow(200)
	cfc.UpdateSendWindow(200)
	_, err = ss.WriteWithLimit([]byte("test"), zeroLimiter)
	if !errors.Is(err, ErrWriteLimitReached) {
		t.Fatalf("expected ErrWriteLimitReached, got %v", err)
	}
}
