// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package stream

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"
)

// ReceiveStream implements the receiving half of a QUIC stream (RFC 9000 §3.2).
// It coordinates frame reassembly, receive flow control, window auto-tuning,
// deadlines, and stream reset handling.
//
// Thread-safety: ReceiveStream is safe for concurrent use. Concurrent Read calls are serialized.
type ReceiveStream struct {
	readMu sync.Mutex // serializes concurrent Read and Peek invocations
	mu     sync.Mutex // protects internal state and coordination

	streamID       StreamID
	sender         StreamSender
	flowController *StreamFlowController
	reassembly     *ReassemblyBuffer

	state RecvState

	ctx       context.Context
	ctxCancel context.CancelCauseFunc

	finalSizeKnown           bool
	receiveFinalSizeCallback func(int64)

	stopSendingQueued bool
	stopSendingCode   StreamErrorCode

	cancelErr   *StreamError
	shutdownErr error

	readDeadline  time.Time
	deadlineTimer *time.Timer
	deadlineCh    chan struct{}

	dataReadyCh chan struct{} // signaled when new contiguous data arrives or state changes
}

var _ io.ReadCloser = &ReceiveStream{}

// NewReceiveStream initializes a receive stream.
func NewReceiveStream(
	ctx context.Context,
	streamID StreamID,
	sender StreamSender,
	flowController *StreamFlowController,
) *ReceiveStream {
	rCtx, cancel := context.WithCancelCause(ctx)
	s := &ReceiveStream{
		streamID:       streamID,
		sender:         sender,
		flowController: flowController,
		reassembly:     NewReassemblyBuffer(),
		state:          RecvStateRecv,
		ctx:            rCtx,
		ctxCancel:      cancel,
		deadlineCh:     make(chan struct{}),
		dataReadyCh:    make(chan struct{}, 1),
	}
	return s
}

// StreamID returns the unique 62-bit stream identifier.
func (s *ReceiveStream) StreamID() StreamID {
	return s.streamID
}

// SetReadDeadline sets the deadline for future Read and Peek calls.
func (s *ReceiveStream) SetReadDeadline(t time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.readDeadline = t
	if s.deadlineTimer != nil {
		s.deadlineTimer.Stop()
		s.deadlineTimer = nil
	}

	if s.deadlineCh != nil {
		select {
		case <-s.deadlineCh:
		default:
			close(s.deadlineCh)
		}
	}
	s.deadlineCh = make(chan struct{})

	if !t.IsZero() {
		dur := time.Until(t)
		if dur <= 0 {
			close(s.deadlineCh)
		} else {
			ch := s.deadlineCh
			s.deadlineTimer = time.AfterFunc(dur, func() {
				s.mu.Lock()
				defer s.mu.Unlock()
				select {
				case <-ch:
				default:
					close(ch)
				}
			})
		}
	}
	return nil
}

// SetReceiveFinalSizeCallback registers a callback invoked when the stream's final size is determined.
func (s *ReceiveStream) SetReceiveFinalSizeCallback(callback func(int64)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.receiveFinalSizeCallback = callback
	if s.finalSizeKnown {
		sz, _ := s.reassembly.FinalSize()
		callback(int64(sz))
	}
}

// FinalSize returns the receive side's final size in bytes, and true if known.
func (s *ReceiveStream) FinalSize() (int64, bool) {
	sz, known := s.reassembly.FinalSize()
	return int64(sz), known
}

// Read reads contiguous stream data into p (RFC 9000 §2.2).
// It blocks until data is available, FIN is reached (io.EOF), a deadline expires, or the stream is reset.
func (s *ReceiveStream) Read(p []byte) (int, error) {
	s.readMu.Lock()
	defer s.readMu.Unlock()

	if len(p) == 0 {
		return 0, nil
	}

	for {
		s.mu.Lock()

		if s.cancelErr != nil {
			s.mu.Unlock()
			return 0, s.cancelErr
		}
		if s.shutdownErr != nil {
			s.mu.Unlock()
			return 0, s.shutdownErr
		}
		if s.state == RecvStateDataRead || s.reassembly.IsAllDataRead() {
			s.mu.Unlock()
			return 0, io.EOF
		}
		if !s.readDeadline.IsZero() && time.Now().After(s.readDeadline) {
			s.mu.Unlock()
			return 0, ErrDeadline
		}
		select {
		case <-s.ctx.Done():
			s.mu.Unlock()
			return 0, context.Cause(s.ctx)
		default:
		}

		n, err := s.reassembly.Read(p)
		if n > 0 {
			s.flowController.AddBytesRead(uint64(n))

			if s.reassembly.IsAllDataRead() {
				s.state = RecvStateDataRead
				if s.sender != nil {
					s.sender.OnStreamCompleted(s.streamID)
				}
			}
			s.mu.Unlock()

			s.maybeTriggerWindowUpdate()
			return n, err
		}

		if errors.Is(err, io.EOF) {
			s.state = RecvStateDataRead
			if s.sender != nil {
				s.sender.OnStreamCompleted(s.streamID)
			}
			s.mu.Unlock()
			return 0, io.EOF
		}

		deadlineCh := s.deadlineCh
		dataCh := s.dataReadyCh
		s.mu.Unlock()

		select {
		case <-dataCh:
			continue
		case <-deadlineCh:
			return 0, ErrDeadline
		case <-s.ctx.Done():
			return 0, context.Cause(s.ctx)
		}
	}
}

// Peek fills b with contiguous stream data without advancing the read offset.
// It blocks until len(b) bytes are available, the stream ends with io.EOF, or an error occurs.
func (s *ReceiveStream) Peek(b []byte) (int, error) {
	s.readMu.Lock()
	defer s.readMu.Unlock()

	if len(b) == 0 {
		return 0, nil
	}

	for {
		s.mu.Lock()

		if s.cancelErr != nil {
			s.mu.Unlock()
			return 0, s.cancelErr
		}
		if s.shutdownErr != nil {
			s.mu.Unlock()
			return 0, s.shutdownErr
		}
		if s.state == RecvStateDataRead || s.reassembly.IsAllDataRead() {
			s.mu.Unlock()
			return 0, io.EOF
		}
		if !s.readDeadline.IsZero() && time.Now().After(s.readDeadline) {
			s.mu.Unlock()
			return 0, ErrDeadline
		}

		n, err := s.reassembly.Peek(b)
		if n == len(b) || (errors.Is(err, io.EOF) && n > 0) {
			s.mu.Unlock()
			return n, err
		}
		if errors.Is(err, io.EOF) && n == 0 {
			s.mu.Unlock()
			return 0, io.EOF
		}

		deadlineCh := s.deadlineCh
		dataCh := s.dataReadyCh
		s.mu.Unlock()

		select {
		case <-dataCh:
			continue
		case <-deadlineCh:
			return 0, ErrDeadline
		case <-s.ctx.Done():
			return 0, context.Cause(s.ctx)
		}
	}
}

// CancelRead terminates reading with STOP_SENDING (RFC 9000 §3.5).
func (s *ReceiveStream) CancelRead(code StreamErrorCode) {
	s.mu.Lock()
	if s.state.IsTerminal() || s.state == RecvStateResetRead {
		s.mu.Unlock()
		return
	}

	s.state = RecvStateResetRead
	s.stopSendingQueued = true
	s.stopSendingCode = code
	s.cancelErr = &StreamError{
		StreamID:  s.streamID,
		ErrorCode: code,
		Remote:    false,
	}

	s.ctxCancel(s.cancelErr)
	s.signalDataReady()
	s.mu.Unlock()

	if s.sender != nil {
		s.sender.OnHasStreamControlFrame(s.streamID)
		s.sender.OnStreamCompleted(s.streamID)
	}
}

// Close is an alias for CloseRead, closing the receive side.
func (s *ReceiveStream) Close() error {
	s.CancelRead(0)
	return nil
}

// CloseRead closes the receive side by canceling with error code 0.
func (s *ReceiveStream) CloseRead() error {
	s.CancelRead(0)
	return nil
}

// HandleStreamFrame processes an incoming STREAM frame (RFC 9000 §2.2 & §4.5).
func (s *ReceiveStream) HandleStreamFrame(offset uint64, data []byte, fin bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state.IsTerminal() || s.state == RecvStateResetRecvd {
		return nil
	}

	// Flow control check (RFC 9000 §4.3)
	end := offset + uint64(len(data))
	if err := s.flowController.UpdateHighestReceived(end); err != nil {
		return err
	}

	// Push into reassembly buffer
	if err := s.reassembly.Push(offset, data, fin); err != nil {
		return err
	}

	if fin && !s.finalSizeKnown {
		s.finalSizeKnown = true
		sz, _ := s.reassembly.FinalSize()
		if s.receiveFinalSizeCallback != nil {
			s.receiveFinalSizeCallback(int64(sz))
		}
		if s.reassembly.AvailableContiguous() >= int(sz-s.reassembly.ReadOffset()) {
			s.state = RecvStateDataRecvd
		} else {
			s.state = RecvStateSizeKnown
		}
	} else if s.state == RecvStateSizeKnown {
		sz, _ := s.reassembly.FinalSize()
		if s.reassembly.AvailableContiguous() >= int(sz-s.reassembly.ReadOffset()) {
			s.state = RecvStateDataRecvd
		}
	}

	s.signalDataReady()
	return nil
}

// HandleResetStreamFrame processes an incoming RESET_STREAM frame from the peer (RFC 9000 §3.2).
func (s *ReceiveStream) HandleResetStreamFrame(code StreamErrorCode, finalSize uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state.IsTerminal() || s.state == RecvStateResetRecvd {
		return nil
	}

	// RFC 9000 §4.5: Check final size consistency
	if s.finalSizeKnown {
		existingSz, _ := s.reassembly.FinalSize()
		if finalSize != existingSz {
			return &TransportError{
				ErrorCode: FinalSizeError,
				ErrorMessage: fmt.Sprintf(
					"RESET_STREAM final size %d mismatch with previously known size %d",
					finalSize, existingSz,
				),
			}
		}
	}

	if finalSize < s.reassembly.HighestOffset() {
		return &TransportError{
			ErrorCode: FinalSizeError,
			ErrorMessage: fmt.Sprintf(
				"RESET_STREAM final size %d < highest received offset %d",
				finalSize, s.reassembly.HighestOffset(),
			),
		}
	}

	s.state = RecvStateResetRecvd
	s.cancelErr = &StreamError{
		StreamID:  s.streamID,
		ErrorCode: code,
		Remote:    true,
	}

	s.ctxCancel(s.cancelErr)
	s.signalDataReady()

	if s.sender != nil {
		s.sender.OnStreamCompleted(s.streamID)
	}
	return nil
}

func (s *ReceiveStream) signalDataReady() {
	select {
	case s.dataReadyCh <- struct{}{}:
	default:
	}
}

func (s *ReceiveStream) maybeTriggerWindowUpdate() {
	if _, ok := s.flowController.MaybeGetWindowUpdate(); ok {
		if s.sender != nil {
			s.sender.OnHasStreamControlFrame(s.streamID)
		}
	}
}

// GetWindowUpdate retrieves the new MAX_STREAM_DATA limit if one is pending.
func (s *ReceiveStream) GetWindowUpdate() (newOffset uint64, ok bool) {
	return s.flowController.MaybeGetWindowUpdate()
}

// GetStopSendingFrame retrieves the STOP_SENDING parameters if queued.
func (s *ReceiveStream) GetStopSendingFrame() (code StreamErrorCode, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.stopSendingQueued {
		s.stopSendingQueued = false
		return s.stopSendingCode, true
	}
	return 0, false
}

// State returns the current RFC 9000 §3.2 RecvState.
func (s *ReceiveStream) State() RecvState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
}

// CloseForShutdown abruptly tears down the receive stream during connection shutdown.
func (s *ReceiveStream) CloseForShutdown(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.shutdownErr = err
	s.ctxCancel(err)
	s.signalDataReady()
}
