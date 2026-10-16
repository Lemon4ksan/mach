// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package stream

import (
	"fmt"
	"sync"
	"time"
)

const (
	// DefaultInitialStreamReceiveWindow is 512 KB (RFC 9000 §4).
	DefaultInitialStreamReceiveWindow uint64 = 512 * 1024
	// DefaultMaxStreamReceiveWindow is 6 MB.
	DefaultMaxStreamReceiveWindow uint64 = 6 * 1024 * 1024
	// DefaultInitialConnReceiveWindow is 512 KB.
	DefaultInitialConnReceiveWindow uint64 = 512 * 1024
	// DefaultMaxConnReceiveWindow is 15 MB.
	DefaultMaxConnReceiveWindow uint64 = 15 * 1024 * 1024

	// WindowUpdateThreshold is the fraction of window consumed before sending an update.
	WindowUpdateThreshold float64 = 0.5
)

// RTTProvider provides the current smoothed RTT for window auto-tuning.
type RTTProvider interface {
	SmoothedRTT() time.Duration
}

type staticRTT time.Duration

func (s staticRTT) SmoothedRTT() time.Duration {
	return time.Duration(s)
}

// ConnectionFlowController manages credit-based flow control at the connection level (RFC 9000 §4.1).
type ConnectionFlowController struct {
	mu sync.Mutex

	// Send side
	bytesSent          uint64
	sendWindow         uint64
	lastBlockedOffset  uint64
	dataBlockedQueued  bool
	hasSentBlocked     bool
	sendBlockedWaiters []chan struct{}

	// Receive side
	bytesRead            uint64
	highestReceived      uint64
	receiveWindow        uint64
	receiveWindowSize    uint64
	maxReceiveWindowSize uint64
	windowUpdateQueued   bool
	queuedWindowUpdate   uint64

	// Auto-tuning
	epochStartTime   time.Time
	epochStartOffset uint64
	rttProvider      RTTProvider

	allowWindowIncrease func(delta uint64) bool
}

// NewConnectionFlowController initializes a connection-level flow controller.
func NewConnectionFlowController(
	initialReceiveWindow uint64,
	maxReceiveWindow uint64,
	initialSendWindow uint64,
	rttProvider RTTProvider,
	allowIncrease func(delta uint64) bool,
) *ConnectionFlowController {
	if initialReceiveWindow == 0 {
		initialReceiveWindow = DefaultInitialConnReceiveWindow
	}

	if maxReceiveWindow == 0 {
		maxReceiveWindow = DefaultMaxConnReceiveWindow
	}

	if rttProvider == nil {
		rttProvider = staticRTT(50 * time.Millisecond)
	}

	return &ConnectionFlowController{
		sendWindow:           initialSendWindow,
		receiveWindow:        initialReceiveWindow,
		receiveWindowSize:    initialReceiveWindow,
		maxReceiveWindowSize: maxReceiveWindow,
		epochStartTime:       time.Now(),
		rttProvider:          rttProvider,
		allowWindowIncrease:  allowIncrease,
	}
}

// SendCreditAvailable returns the current connection-level send credit remaining.
func (c *ConnectionFlowController) SendCreditAvailable() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.sendWindow > c.bytesSent {
		return c.sendWindow - c.bytesSent
	}

	return 0
}

// AddBytesSent accounts for bytes transmitted across all connection streams.
func (c *ConnectionFlowController) AddBytesSent(n uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.bytesSent += n
}

// UpdateSendWindow updates the remote peer's advertised connection receive limit (MAX_DATA).
func (c *ConnectionFlowController) UpdateSendWindow(newLimit uint64) {
	c.mu.Lock()
	if newLimit > c.sendWindow {
		c.sendWindow = newLimit
		c.hasSentBlocked = false
		// Unblock all waiting writers
		for _, ch := range c.sendBlockedWaiters {
			close(ch)
		}

		c.sendBlockedWaiters = nil
	}

	c.mu.Unlock()
}

// RegisterSendWaiter returns a channel that is closed when connection send window increases.
func (c *ConnectionFlowController) RegisterSendWaiter() <-chan struct{} {
	c.mu.Lock()
	defer c.mu.Unlock()

	ch := make(chan struct{})
	c.sendBlockedWaiters = append(c.sendBlockedWaiters, ch)

	return ch
}

// CheckAndQueueDataBlocked reports whether the connection is blocked from sending and marks DATA_BLOCKED.
func (c *ConnectionFlowController) CheckAndQueueDataBlocked() (offset uint64, shouldSend bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.bytesSent >= c.sendWindow && !c.hasSentBlocked {
		c.hasSentBlocked = true
		c.dataBlockedQueued = true
		c.lastBlockedOffset = c.sendWindow

		return c.sendWindow, true
	}

	return 0, false
}

// UpdateHighestReceived registers the highest offset received on any stream.
// Returns TransportError with FlowControlError if highestReceived exceeds receiveWindow (RFC 9000 §4.3).
func (c *ConnectionFlowController) UpdateHighestReceived(streamIncrement uint64) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.highestReceived += streamIncrement
	if c.highestReceived > c.receiveWindow {
		return &TransportError{
			ErrorCode: FlowControlError,
			ErrorMessage: fmt.Sprintf(
				"connection flow control violation: received %d > window %d",
				c.highestReceived, c.receiveWindow,
			),
		}
	}

	return nil
}

// AddBytesRead accounts for bytes read by the application across all streams.
func (c *ConnectionFlowController) AddBytesRead(n uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.bytesRead += n
}

// CheckAndQueueWindowUpdate checks if a MAX_DATA frame should be sent, auto-tuning the window if needed,
// and queues the update for transmission.
func (c *ConnectionFlowController) CheckAndQueueWindowUpdate() (newWindow uint64, ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	var bytesRemaining uint64
	if c.receiveWindow > c.bytesRead {
		bytesRemaining = c.receiveWindow - c.bytesRead
	}

	threshold := uint64(float64(c.receiveWindowSize) * (1.0 - WindowUpdateThreshold))
	if bytesRemaining > threshold {
		if c.windowUpdateQueued {
			return c.queuedWindowUpdate, true
		}

		return 0, false
	}

	c.maybeAdjustWindowSize(time.Now())
	c.receiveWindow = c.bytesRead + c.receiveWindowSize
	c.queuedWindowUpdate = c.receiveWindow
	c.windowUpdateQueued = true

	return c.receiveWindow, true
}

// GetWindowUpdate retrieves the queued MAX_DATA limit if one is pending, clearing the queued flag.
func (c *ConnectionFlowController) GetWindowUpdate() (newWindow uint64, ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.windowUpdateQueued {
		c.windowUpdateQueued = false
		return c.queuedWindowUpdate, true
	}

	return 0, false
}

// MaybeGetWindowUpdate checks if a MAX_DATA frame should be sent, auto-tuning the window if needed.
func (c *ConnectionFlowController) MaybeGetWindowUpdate() (newWindow uint64, ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.windowUpdateQueued {
		c.windowUpdateQueued = false
		return c.queuedWindowUpdate, true
	}

	var bytesRemaining uint64
	if c.receiveWindow > c.bytesRead {
		bytesRemaining = c.receiveWindow - c.bytesRead
	}

	threshold := uint64(float64(c.receiveWindowSize) * (1.0 - WindowUpdateThreshold))
	if bytesRemaining > threshold {
		return 0, false
	}

	c.maybeAdjustWindowSize(time.Now())
	c.receiveWindow = c.bytesRead + c.receiveWindowSize

	return c.receiveWindow, true
}

// GetDataBlockedFrame retrieves the queued DATA_BLOCKED limit if pending, clearing the queued flag.
func (c *ConnectionFlowController) GetDataBlockedFrame() (offset uint64, ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.dataBlockedQueued {
		c.dataBlockedQueued = false
		c.hasSentBlocked = true
		return c.lastBlockedOffset, true
	}

	if c.bytesSent >= c.sendWindow && !c.hasSentBlocked {
		c.hasSentBlocked = true
		c.lastBlockedOffset = c.sendWindow
		return c.sendWindow, true
	}

	return 0, false
}

// ReclaimBytesSent reduces connection bytesSent by n when buffered data is discarded on stream reset.
func (c *ConnectionFlowController) ReclaimBytesSent(n uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.bytesSent >= n {
		c.bytesSent -= n
	} else {
		c.bytesSent = 0
	}
}

func (c *ConnectionFlowController) maybeAdjustWindowSize(now time.Time) {
	bytesReadInEpoch := c.bytesRead - c.epochStartOffset
	if bytesReadInEpoch < c.receiveWindowSize/2 {
		return
	}

	rtt := c.rttProvider.SmoothedRTT()
	if rtt > 0 {
		fraction := float64(bytesReadInEpoch) / float64(c.receiveWindowSize)
		thresholdDuration := time.Duration(4.0 * fraction * float64(rtt))

		if now.Sub(c.epochStartTime) < thresholdDuration {
			newSize := min(2*c.receiveWindowSize, c.maxReceiveWindowSize)
			if newSize > c.receiveWindowSize {
				if c.allowWindowIncrease == nil || c.allowWindowIncrease(newSize-c.receiveWindowSize) {
					c.receiveWindowSize = newSize
				}
			}
		}
	}

	c.epochStartTime = now
	c.epochStartOffset = c.bytesRead
}

// StreamFlowController manages credit-based flow control for an individual stream (RFC 9000 §4.1).
type StreamFlowController struct {
	mu sync.Mutex

	streamID   StreamID
	connection *ConnectionFlowController

	// Send side
	bytesSent               uint64
	sendWindow              uint64
	lastBlockedOffset       uint64
	streamDataBlockedQueued bool
	hasSentBlocked          bool
	sendBlockedWaiters      []chan struct{}

	// Receive side
	bytesRead            uint64
	highestReceived      uint64
	receiveWindow        uint64
	receiveWindowSize    uint64
	maxReceiveWindowSize uint64
	windowUpdateQueued   bool
	queuedWindowUpdate   uint64

	// Auto-tuning
	epochStartTime   time.Time
	epochStartOffset uint64
	rttProvider      RTTProvider

	allowWindowIncrease func(delta uint64) bool
}

// NewStreamFlowController initializes a stream-level flow controller.
func NewStreamFlowController(
	streamID StreamID,
	connection *ConnectionFlowController,
	initialReceiveWindow uint64,
	maxReceiveWindow uint64,
	initialSendWindow uint64,
	rttProvider RTTProvider,
	allowIncrease func(delta uint64) bool,
) *StreamFlowController {
	if initialReceiveWindow == 0 {
		initialReceiveWindow = DefaultInitialStreamReceiveWindow
	}

	if maxReceiveWindow == 0 {
		maxReceiveWindow = DefaultMaxStreamReceiveWindow
	}

	if rttProvider == nil {
		rttProvider = staticRTT(50 * time.Millisecond)
	}

	return &StreamFlowController{
		streamID:             streamID,
		connection:           connection,
		sendWindow:           initialSendWindow,
		receiveWindow:        initialReceiveWindow,
		receiveWindowSize:    initialReceiveWindow,
		maxReceiveWindowSize: maxReceiveWindow,
		epochStartTime:       time.Now(),
		rttProvider:          rttProvider,
		allowWindowIncrease:  allowIncrease,
	}
}

// SendCreditAvailable returns the effective credit available to send on this stream,
// bounded by both stream-level and connection-level flow control windows (RFC 9000 §4.1).
func (s *StreamFlowController) SendCreditAvailable() (streamCredit, connCredit, effective uint64) {
	s.mu.Lock()
	if s.sendWindow > s.bytesSent {
		streamCredit = s.sendWindow - s.bytesSent
	}

	s.mu.Unlock()

	if s.connection != nil {
		connCredit = s.connection.SendCreditAvailable()
		effective = min(streamCredit, connCredit)
	} else {
		connCredit = streamCredit
		effective = streamCredit
	}

	return streamCredit, connCredit, effective
}

// AddBytesSent records bytes transmitted on this stream and the parent connection.
func (s *StreamFlowController) AddBytesSent(n uint64) {
	s.mu.Lock()
	s.bytesSent += n
	s.mu.Unlock()

	if s.connection != nil {
		s.connection.AddBytesSent(n)
	}
}

// UpdateSendWindow updates the peer's advertised stream receive limit (MAX_STREAM_DATA).
func (s *StreamFlowController) UpdateSendWindow(newLimit uint64) {
	s.mu.Lock()
	if newLimit > s.sendWindow {
		s.sendWindow = newLimit

		s.hasSentBlocked = false
		for _, ch := range s.sendBlockedWaiters {
			close(ch)
		}

		s.sendBlockedWaiters = nil
	}

	s.mu.Unlock()
}

// RegisterSendWaiter returns a channel that is closed when the stream send window increases.
func (s *StreamFlowController) RegisterSendWaiter() <-chan struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()

	ch := make(chan struct{})
	s.sendBlockedWaiters = append(s.sendBlockedWaiters, ch)

	return ch
}

// CheckAndQueueStreamDataBlocked checks if the stream is blocked on stream-level flow control.
func (s *StreamFlowController) CheckAndQueueStreamDataBlocked() (offset uint64, shouldSend bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.bytesSent >= s.sendWindow && !s.hasSentBlocked {
		s.hasSentBlocked = true
		s.streamDataBlockedQueued = true
		s.lastBlockedOffset = s.sendWindow

		return s.sendWindow, true
	}

	return 0, false
}

// UpdateHighestReceived checks an incoming stream frame against the stream receive window.
// Returns TransportError with FlowControlError if highestReceived > receiveWindow (RFC 9000 §4.3).
func (s *StreamFlowController) UpdateHighestReceived(offset uint64) error {
	s.mu.Lock()

	var increment uint64
	if offset > s.highestReceived {
		increment = offset - s.highestReceived
		s.highestReceived = offset
	}

	currentWindow := s.receiveWindow
	s.mu.Unlock()

	if offset > currentWindow {
		return &TransportError{
			ErrorCode: FlowControlError,
			ErrorMessage: fmt.Sprintf(
				"stream %d flow control violation: received %d > window %d",
				s.streamID, offset, currentWindow,
			),
		}
	}

	if increment > 0 && s.connection != nil {
		return s.connection.UpdateHighestReceived(increment)
	}

	return nil
}

// AddBytesRead accounts for stream data consumed by the application.
func (s *StreamFlowController) AddBytesRead(n uint64) {
	s.mu.Lock()
	s.bytesRead += n
	s.mu.Unlock()

	if s.connection != nil {
		s.connection.AddBytesRead(n)
	}
}

// CheckAndQueueWindowUpdate checks if a MAX_STREAM_DATA frame should be sent, auto-tuning the window if needed,
// and queues the update for transmission.
func (s *StreamFlowController) CheckAndQueueWindowUpdate() (newWindow uint64, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var bytesRemaining uint64
	if s.receiveWindow > s.bytesRead {
		bytesRemaining = s.receiveWindow - s.bytesRead
	}

	threshold := uint64(float64(s.receiveWindowSize) * (1.0 - WindowUpdateThreshold))
	if bytesRemaining > threshold {
		if s.windowUpdateQueued {
			return s.queuedWindowUpdate, true
		}

		return 0, false
	}

	s.maybeAdjustWindowSize(time.Now())
	s.receiveWindow = s.bytesRead + s.receiveWindowSize
	s.queuedWindowUpdate = s.receiveWindow
	s.windowUpdateQueued = true

	return s.receiveWindow, true
}

// GetWindowUpdate retrieves the queued MAX_STREAM_DATA limit if one is pending, clearing the queued flag.
func (s *StreamFlowController) GetWindowUpdate() (newOffset uint64, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.windowUpdateQueued {
		s.windowUpdateQueued = false
		return s.queuedWindowUpdate, true
	}

	return 0, false
}

// MaybeGetWindowUpdate checks if a MAX_STREAM_DATA frame should be sent, auto-tuning the window if needed.
func (s *StreamFlowController) MaybeGetWindowUpdate() (newWindow uint64, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.windowUpdateQueued {
		s.windowUpdateQueued = false
		return s.queuedWindowUpdate, true
	}

	var bytesRemaining uint64
	if s.receiveWindow > s.bytesRead {
		bytesRemaining = s.receiveWindow - s.bytesRead
	}

	threshold := uint64(float64(s.receiveWindowSize) * (1.0 - WindowUpdateThreshold))
	if bytesRemaining > threshold {
		return 0, false
	}

	s.maybeAdjustWindowSize(time.Now())
	s.receiveWindow = s.bytesRead + s.receiveWindowSize

	return s.receiveWindow, true
}

// GetStreamDataBlockedFrame retrieves the queued STREAM_DATA_BLOCKED limit if pending, clearing the queued flag.
func (s *StreamFlowController) GetStreamDataBlockedFrame() (offset uint64, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.streamDataBlockedQueued {
		s.streamDataBlockedQueued = false
		s.hasSentBlocked = true
		return s.lastBlockedOffset, true
	}

	if s.bytesSent >= s.sendWindow && !s.hasSentBlocked {
		s.hasSentBlocked = true
		s.lastBlockedOffset = s.sendWindow
		return s.sendWindow, true
	}

	return 0, false
}

// ReclaimBytesSent reduces bytesSent by n when buffered data is discarded on stream reset.
func (s *StreamFlowController) ReclaimBytesSent(n uint64) {
	s.mu.Lock()
	if s.bytesSent >= n {
		s.bytesSent -= n
	} else {
		s.bytesSent = 0
	}

	s.mu.Unlock()

	if s.connection != nil {
		s.connection.ReclaimBytesSent(n)
	}
}

func (s *StreamFlowController) maybeAdjustWindowSize(now time.Time) {
	bytesReadInEpoch := s.bytesRead - s.epochStartOffset
	if bytesReadInEpoch < s.receiveWindowSize/2 {
		return
	}

	rtt := s.rttProvider.SmoothedRTT()
	if rtt > 0 {
		fraction := float64(bytesReadInEpoch) / float64(s.receiveWindowSize)
		thresholdDuration := time.Duration(4.0 * fraction * float64(rtt))

		if now.Sub(cStartTime(s.epochStartTime)) < thresholdDuration {
			newSize := min(2*s.receiveWindowSize, s.maxReceiveWindowSize)
			if newSize > s.receiveWindowSize {
				if s.allowWindowIncrease == nil || s.allowWindowIncrease(newSize-s.receiveWindowSize) {
					s.receiveWindowSize = newSize
				}
			}
		}
	}

	s.epochStartTime = now
	s.epochStartOffset = s.bytesRead
}

func cStartTime(t time.Time) time.Time {
	return t
}
