// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package stream

import (
	"context"
	"io"
	"sync"
	"sync/atomic"
	"time"
)

// StreamSender notifies the connection engine about stream events.
type StreamSender interface {
	OnHasStreamData(id StreamID)
	OnHasStreamControlFrame(id StreamID)
	OnStreamCompleted(id StreamID)
}

// SendStream implements the sending half of a QUIC stream (RFC 9000 §3.1).
// It coordinates write operations, flow control credit enforcement, deadlines,
// priority hints, and frame production.
//
// Thread-safety: SendStream is safe for concurrent use. Concurrent Write calls are serialized.
type SendStream struct {
	writeMu sync.Mutex // serializes concurrent Write invocations
	mu      sync.Mutex // protects internal state, queues, and metadata

	streamID       StreamID
	sender         StreamSender
	flowController *StreamFlowController

	state SendState

	ctx       context.Context
	ctxCancel context.CancelCauseFunc

	writeOffset uint64 // total bytes queued into writeBuffer
	sentOffset  uint64 // total bytes popped into STREAM frames

	writeBuffer []byte // pending stream payload bytes ready for packetization

	finQueued bool   // true once Close/CloseWrite is called
	finSent   bool   // true once a STREAM frame with FIN bit is popped
	finalSize uint64 // stream length when FIN is queued

	resetQueued   bool
	resetSent     bool
	resetCode     StreamErrorCode
	resetFinalPos uint64
	resetErr      *StreamError

	stopSendingReceived bool
	stopSendingCode     StreamErrorCode

	shutdownErr error

	urgency     atomic.Int32
	incremental atomic.Bool

	writeDeadline time.Time
	deadlineTimer *time.Timer
	deadlineCh    chan struct{}

	dataAvailableCh chan struct{} // signaled when new data is written
	wakeWaitersCh   chan struct{} // signaled to wake blocked writers on window update / reset / cancel
}

var _ io.WriteCloser = &SendStream{}

// NewSendStream initializes a send stream.
func NewSendStream(
	ctx context.Context,
	streamID StreamID,
	sender StreamSender,
	flowController *StreamFlowController,
) *SendStream {
	sCtx, cancel := context.WithCancelCause(ctx)
	s := &SendStream{
		streamID:        streamID,
		sender:          sender,
		flowController:  flowController,
		state:           SendStateReady,
		ctx:             sCtx,
		ctxCancel:       cancel,
		deadlineCh:      make(chan struct{}),
		dataAvailableCh: make(chan struct{}, 1),
		wakeWaitersCh:   make(chan struct{}),
	}

	return s
}

// StreamID returns the unique 62-bit stream identifier.
func (s *SendStream) StreamID() StreamID {
	return s.streamID
}

// Context returns a context that is canceled when the send stream is closed or reset.
func (s *SendStream) Context() context.Context {
	return s.ctx
}

// SetPriority sets the HTTP/3 priority parameters (RFC 9218).
func (s *SendStream) SetPriority(urgency int8, incremental bool) {
	s.urgency.Store(int32(urgency))
	s.incremental.Store(incremental)
}

// Priority returns the current stream urgency and incremental hints.
func (s *SendStream) Priority() (urgency int8, incremental bool) {
	return int8(s.urgency.Load()), s.incremental.Load()
}

// SetWriteDeadline sets the deadline for future Write calls.
func (s *SendStream) SetWriteDeadline(t time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.writeDeadline = t
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

// Write writes data to the stream, blocking if flow control windows are exhausted (RFC 9000 §4).
func (s *SendStream) Write(p []byte) (int, error) {
	return s.WriteWithLimit(p, nil)
}

// WriteWithLimit writes data subject to an optional limiter callback.
func (s *SendStream) WriteWithLimit(p []byte, limiter func(maxBytes int) int) (int, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	if len(p) == 0 {
		return 0, nil
	}

	totalWritten := 0

	for len(p) > 0 {
		s.mu.Lock()

		if s.resetErr != nil {
			s.mu.Unlock()
			return totalWritten, s.resetErr
		}

		if s.shutdownErr != nil {
			s.mu.Unlock()
			return totalWritten, s.shutdownErr
		}

		if s.state == SendStateDataSent || s.state == SendStateDataRecvd {
			s.mu.Unlock()
			return totalWritten, ErrStreamClosed
		}

		if !s.writeDeadline.IsZero() && time.Now().After(s.writeDeadline) {
			s.mu.Unlock()
			return totalWritten, ErrDeadline
		}

		select {
		case <-s.ctx.Done():
			s.mu.Unlock()
			return totalWritten, context.Cause(s.ctx)
		default:
		}

		deadlineCh := s.deadlineCh
		wakeCh := s.wakeWaitersCh

		// Check flow control credits
		streamCredit, connCredit, effective := s.flowController.SendCreditAvailable()
		s.mu.Unlock()

		if effective == 0 {
			// Flow control blocked. Signal blocked frames per RFC 9000 §4.1.
			if streamCredit <= connCredit {
				s.flowController.CheckAndQueueStreamDataBlocked()
			}

			if s.flowController.connection != nil && connCredit <= streamCredit {
				s.flowController.connection.CheckAndQueueDataBlocked()
			}

			if s.sender != nil {
				s.sender.OnHasStreamControlFrame(s.streamID)
			}

			// Block until credit arrives or deadline/context expires.
			streamWaiter := s.flowController.RegisterSendWaiter()

			var connWaiter <-chan struct{}
			if s.flowController.connection != nil {
				connWaiter = s.flowController.connection.RegisterSendWaiter()
			}

			select {
			case <-s.ctx.Done():
				return totalWritten, context.Cause(s.ctx)
			case <-deadlineCh:
				return totalWritten, ErrDeadline
			case <-wakeCh:
				continue
			case <-streamWaiter:
				continue
			case <-connWaiter:
				continue
			}
		}

		// Available credit allows writing up to effective bytes
		chunkLen := min(uint64(len(p)), effective)
		if limiter != nil {
			lim := limiter(int(chunkLen))
			if lim < 0 {
				lim = 0
			}

			if lim < int(chunkLen) {
				chunkLen = uint64(lim)
			}
		}

		if chunkLen == 0 {
			return totalWritten, ErrWriteLimitReached
		}

		s.mu.Lock()
		s.writeBuffer = append(s.writeBuffer, p[:chunkLen]...)
		s.writeOffset += chunkLen
		s.flowController.AddBytesSent(chunkLen)

		if s.state == SendStateReady {
			s.state = SendStateSend
		}

		s.mu.Unlock()

		totalWritten += int(chunkLen)
		p = p[chunkLen:]

		if s.sender != nil {
			s.sender.OnHasStreamData(s.streamID)
		}
	}

	return totalWritten, nil
}

// TryWriteAll writes the entire slice p if sufficient flow control credit is immediately available.
// If not, it returns ErrWouldBlock without modifying buffer state.
func (s *SendStream) TryWriteAll(p []byte) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	if len(p) == 0 {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.resetErr != nil {
		return s.resetErr
	}

	if s.shutdownErr != nil {
		return s.shutdownErr
	}

	if s.state == SendStateDataSent || s.state == SendStateDataRecvd {
		return ErrStreamClosed
	}

	_, _, effective := s.flowController.SendCreditAvailable()
	if effective < uint64(len(p)) {
		return ErrWouldBlock
	}

	s.writeBuffer = append(s.writeBuffer, p...)
	s.writeOffset += uint64(len(p))
	s.flowController.AddBytesSent(uint64(len(p)))

	if s.state == SendStateReady {
		s.state = SendStateSend
	}

	if s.sender != nil {
		s.sender.OnHasStreamData(s.streamID)
	}

	return nil
}

// Close gracefully closes the write half of the stream, transmitting FIN (RFC 9000 §3.1).
func (s *SendStream) Close() error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	s.mu.Lock()
	if s.resetErr != nil {
		s.mu.Unlock()
		return s.resetErr
	}

	if s.state == SendStateDataSent || s.state == SendStateDataRecvd {
		s.mu.Unlock()
		return nil
	}

	s.finQueued = true
	s.finalSize = s.writeOffset
	s.state = SendStateDataSent
	s.ctxCancel(nil)
	s.mu.Unlock()

	if s.sender != nil {
		s.sender.OnHasStreamData(s.streamID)
	}

	return nil
}

// CloseWrite is an alias for Close, matching TCP Conn interface semantics.
func (s *SendStream) CloseWrite() error {
	return s.Close()
}

// CancelWrite abruptly terminates the sending half with a RESET_STREAM frame (RFC 9000 §3.1).
func (s *SendStream) CancelWrite(code StreamErrorCode) {
	s.mu.Lock()
	if s.state.IsTerminal() || s.state == SendStateResetSent {
		s.mu.Unlock()
		return
	}

	s.state = SendStateResetSent
	s.resetQueued = true
	s.resetCode = code
	s.resetFinalPos = s.sentOffset // RFC 9000 §4.5: Final size is the exact number of bytes sent on the wire

	// Reclaim flow control credits for unsent buffered data
	if s.writeOffset > s.sentOffset {
		unsent := s.writeOffset - s.sentOffset
		s.flowController.ReclaimBytesSent(unsent)
	}

	s.writeBuffer = nil

	s.resetErr = &StreamError{
		StreamID:  s.streamID,
		ErrorCode: code,
		Remote:    false,
	}

	s.ctxCancel(s.resetErr)
	s.broadcastWakeWaiters()
	s.mu.Unlock()

	if s.sender != nil {
		s.sender.OnHasStreamControlFrame(s.streamID)
	}
}

// HandleStopSending processes an incoming STOP_SENDING frame from the peer (RFC 9000 §3.5).
func (s *SendStream) HandleStopSending(code StreamErrorCode) {
	s.mu.Lock()
	if s.state.IsTerminal() || s.state == SendStateResetSent {
		s.mu.Unlock()
		return
	}

	s.stopSendingReceived = true
	s.stopSendingCode = code
	s.state = SendStateResetSent
	s.resetQueued = true
	s.resetCode = code
	s.resetFinalPos = s.sentOffset // RFC 9000 §4.5: Final size is the exact number of bytes sent on the wire

	// Reclaim flow control credits for unsent buffered data
	if s.writeOffset > s.sentOffset {
		unsent := s.writeOffset - s.sentOffset
		s.flowController.ReclaimBytesSent(unsent)
	}

	s.writeBuffer = nil

	s.resetErr = &StreamError{
		StreamID:  s.streamID,
		ErrorCode: code,
		Remote:    true,
	}

	s.ctxCancel(s.resetErr)
	s.broadcastWakeWaiters()
	s.mu.Unlock()

	if s.sender != nil {
		s.sender.OnHasStreamControlFrame(s.streamID)
	}
}

// UpdateSendWindow updates the peer's flow control limit and unblocks waiting writers.
func (s *SendStream) UpdateSendWindow(limit uint64) {
	s.flowController.UpdateSendWindow(limit)
	s.mu.Lock()
	s.broadcastWakeWaiters()
	s.mu.Unlock()
}

func (s *SendStream) broadcastWakeWaiters() {
	close(s.wakeWaitersCh)
	s.wakeWaitersCh = make(chan struct{})
}

// PopStreamFrame extracts queued bytes up to maxBytes into a STREAM frame payload.
func (s *SendStream) PopStreamFrame(maxBytes int) (data []byte, offset uint64, fin, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.resetSent || s.state == SendStateResetSent || s.state == SendStateResetRecvd {
		return nil, 0, false, false
	}

	bufLen := len(s.writeBuffer)
	if bufLen == 0 && (!s.finQueued || s.finSent) {
		return nil, 0, false, false
	}

	offset = s.sentOffset
	take := min(bufLen, maxBytes)

	if take > 0 {
		data = append([]byte(nil), s.writeBuffer[:take]...)

		s.writeBuffer = s.writeBuffer[take:]
		if len(s.writeBuffer) == 0 {
			s.writeBuffer = s.writeBuffer[:0]
		}

		s.sentOffset += uint64(take)
	}

	// Attach FIN if write buffer is completely emptied and Close was called
	if s.finQueued && len(s.writeBuffer) == 0 && !s.finSent {
		fin = true
		s.finSent = true
	}

	return data, offset, fin, true
}

// HandleAck processes an acknowledgment for transmitted data or FIN.
func (s *SendStream) HandleAck(allDataAcked bool) {
	s.mu.Lock()

	var (
		completed bool
		sender    StreamSender
		streamID  StreamID
	)
	if allDataAcked && s.finSent && s.state == SendStateDataSent {
		s.state = SendStateDataRecvd
		completed = true
		sender = s.sender
		streamID = s.streamID
	}

	s.mu.Unlock()

	if completed && sender != nil {
		sender.OnStreamCompleted(streamID)
	}
}

// HandleResetAck acknowledges the RESET_STREAM frame.
func (s *SendStream) HandleResetAck() {
	s.mu.Lock()

	var (
		completed bool
		sender    StreamSender
		streamID  StreamID
	)
	if s.state == SendStateResetSent {
		s.state = SendStateResetRecvd
		completed = true
		sender = s.sender
		streamID = s.streamID
	}

	s.mu.Unlock()

	if completed && sender != nil {
		sender.OnStreamCompleted(streamID)
	}
}

// GetResetStreamFrame retrieves the RESET_STREAM parameters if queued.
func (s *SendStream) GetResetStreamFrame() (code StreamErrorCode, finalSize uint64, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.resetQueued {
		s.resetSent = true
		return s.resetCode, s.resetFinalPos, true
	}

	return 0, 0, false
}

// GetStreamDataBlockedFrame retrieves the queued STREAM_DATA_BLOCKED limit if pending.
func (s *SendStream) GetStreamDataBlockedFrame() (offset uint64, ok bool) {
	return s.flowController.GetStreamDataBlockedFrame()
}

// State returns the current RFC 9000 §3.1 SendState.
func (s *SendStream) State() SendState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
}

// CloseForShutdown shuts down the stream immediately due to connection closure.
func (s *SendStream) CloseForShutdown(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.shutdownErr = err
	s.ctxCancel(err)
	s.broadcastWakeWaiters()
}
