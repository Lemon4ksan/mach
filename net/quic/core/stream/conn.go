// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package stream

import (
	"context"
	"fmt"
	"net"
	"sync"
)

// ConnConfig configures the stream multiplexer and flow controller defaults.
type ConnConfig struct {
	Perspective Perspective

	InitialStreamReceiveWindow uint64
	MaxStreamReceiveWindow     uint64

	InitialConnectionReceiveWindow uint64
	MaxConnectionReceiveWindow     uint64

	InitialMaxStreamsBidi uint64
	InitialMaxStreamsUni  uint64

	InitialPeerMaxStreamsBidi uint64
	InitialPeerMaxStreamsUni  uint64

	InitialPeerMaxData uint64

	LocalAddr  net.Addr
	RemoteAddr net.Addr

	RTTProvider RTTProvider
}

// Conn manages multiplexed bidirectional and unidirectional QUIC streams over a single connection session (RFC 9000 §2).
// It implements ConnI, providing full drop-in compatibility for HTTP/3 client and server stacks.
//
// Thread-safety: Conn is completely safe for concurrent access across multiple goroutines.
type Conn struct {
	mu sync.Mutex

	config ConnConfig

	ctx       context.Context
	ctxCancel context.CancelCauseFunc

	connFlowController *ConnectionFlowController

	// Maps of active streams
	bidiStreams map[StreamID]*Stream
	sendStreams map[StreamID]*SendStream
	recvStreams map[StreamID]*ReceiveStream

	// Next stream counters (1-based)
	nextLocalBidiStreamNum uint64
	nextLocalUniStreamNum  uint64

	// Concurrency limits for locally-opened streams
	maxPeerBidiStreams uint64
	maxPeerUniStreams  uint64
	bidiStreamsWaiters []chan struct{}
	uniStreamsWaiters  []chan struct{}

	// Concurrency limits for peer-initiated streams
	maxLocalBidiStreams uint64
	maxLocalUniStreams  uint64

	closedStreams map[StreamID]struct{}

	// Incoming streams acceptance queues
	acceptBidiCh chan *Stream
	acceptUniCh  chan *ReceiveStream

	// Lifecycle
	closeOnce sync.Once
	closedCh  chan struct{}
	closeErr  error
}

var _ ConnI = &Conn{}

// NewConn initializes a clean-room QUIC stream multiplexer and flow control manager.
func NewConn(cfg ConnConfig) *Conn {
	if cfg.InitialStreamReceiveWindow == 0 {
		cfg.InitialStreamReceiveWindow = DefaultInitialStreamReceiveWindow
	}
	if cfg.MaxStreamReceiveWindow == 0 {
		cfg.MaxStreamReceiveWindow = DefaultMaxStreamReceiveWindow
	}
	if cfg.InitialConnectionReceiveWindow == 0 {
		cfg.InitialConnectionReceiveWindow = DefaultInitialConnReceiveWindow
	}
	if cfg.MaxConnectionReceiveWindow == 0 {
		cfg.MaxConnectionReceiveWindow = DefaultMaxConnReceiveWindow
	}
	if cfg.InitialMaxStreamsBidi == 0 {
		cfg.InitialMaxStreamsBidi = 100
	}
	if cfg.InitialMaxStreamsUni == 0 {
		cfg.InitialMaxStreamsUni = 100
	}
	if cfg.InitialPeerMaxStreamsBidi == 0 {
		cfg.InitialPeerMaxStreamsBidi = 100
	}
	if cfg.InitialPeerMaxStreamsUni == 0 {
		cfg.InitialPeerMaxStreamsUni = 100
	}
	if cfg.InitialPeerMaxData == 0 {
		cfg.InitialPeerMaxData = DefaultInitialConnReceiveWindow
	}

	ctx, cancel := context.WithCancelCause(context.Background())

	cfc := NewConnectionFlowController(
		cfg.InitialConnectionReceiveWindow,
		cfg.MaxConnectionReceiveWindow,
		cfg.InitialPeerMaxData,
		cfg.RTTProvider,
		nil,
	)

	c := &Conn{
		config:                 cfg,
		ctx:                    ctx,
		ctxCancel:              cancel,
		connFlowController:     cfc,
		bidiStreams:            make(map[StreamID]*Stream),
		sendStreams:            make(map[StreamID]*SendStream),
		recvStreams:            make(map[StreamID]*ReceiveStream),
		nextLocalBidiStreamNum: 1,
		nextLocalUniStreamNum:  1,
		maxPeerBidiStreams:     cfg.InitialPeerMaxStreamsBidi,
		maxPeerUniStreams:      cfg.InitialPeerMaxStreamsUni,
		maxLocalBidiStreams:    cfg.InitialMaxStreamsBidi,
		maxLocalUniStreams:     cfg.InitialMaxStreamsUni,
		acceptBidiCh:           make(chan *Stream, cfg.InitialMaxStreamsBidi),
		acceptUniCh:            make(chan *ReceiveStream, cfg.InitialMaxStreamsUni),
		closedStreams:          make(map[StreamID]struct{}),
		closedCh:               make(chan struct{}),
	}
	return c
}

// Context returns the connection context, canceled when the connection is closed.
func (c *Conn) Context() context.Context {
	return c.ctx
}

// IsClosed reports whether the connection session has closed.
func (c *Conn) IsClosed() bool {
	select {
	case <-c.closedCh:
		return true
	default:
		return false
	}
}

// LocalAddr returns the local network address.
func (c *Conn) LocalAddr() net.Addr {
	return c.config.LocalAddr
}

// RemoteAddr returns the remote network address.
func (c *Conn) RemoteAddr() net.Addr {
	return c.config.RemoteAddr
}

// OpenStream opens a new bidirectional stream without blocking.
// Returns StreamLimitError if the peer's stream concurrency limit has been reached.
func (c *Conn) OpenStream() (*Stream, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.IsClosed() {
		return nil, ErrConnClosed
	}

	if (c.nextLocalBidiStreamNum - 1) >= c.maxPeerBidiStreams {
		return nil, &TransportError{
			ErrorCode:    StreamLimitError,
			ErrorMessage: "peer bidirectional stream limit exceeded",
		}
	}

	st := StreamTypeClientBidi
	if c.config.Perspective == PerspectiveServer {
		st = StreamTypeServerBidi
	}
	streamID := NewStreamID(st, c.nextLocalBidiStreamNum)
	c.nextLocalBidiStreamNum++

	str := c.createBidiStreamLocked(streamID)
	return str, nil
}

// OpenStreamSync opens a new bidirectional stream, blocking if the peer's stream limit is reached (RFC 9000 §4.6).
func (c *Conn) OpenStreamSync(ctx context.Context) (*Stream, error) {
	for {
		c.mu.Lock()
		if c.IsClosed() {
			c.mu.Unlock()
			return nil, ErrConnClosed
		}

		if (c.nextLocalBidiStreamNum - 1) < c.maxPeerBidiStreams {
			st := StreamTypeClientBidi
			if c.config.Perspective == PerspectiveServer {
				st = StreamTypeServerBidi
			}
			streamID := NewStreamID(st, c.nextLocalBidiStreamNum)
			c.nextLocalBidiStreamNum++

			str := c.createBidiStreamLocked(streamID)
			c.mu.Unlock()
			return str, nil
		}

		waiter := make(chan struct{})
		c.bidiStreamsWaiters = append(c.bidiStreamsWaiters, waiter)
		c.mu.Unlock()

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-c.ctx.Done():
			return nil, ErrConnClosed
		case <-waiter:
			continue
		}
	}
}

// OpenUniStream opens a new outbound unidirectional stream without blocking.
// Returns StreamLimitError if the peer's stream concurrency limit has been reached.
func (c *Conn) OpenUniStream() (*SendStream, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.IsClosed() {
		return nil, ErrConnClosed
	}

	if (c.nextLocalUniStreamNum - 1) >= c.maxPeerUniStreams {
		return nil, &TransportError{
			ErrorCode:    StreamLimitError,
			ErrorMessage: "peer unidirectional stream limit exceeded",
		}
	}

	st := StreamTypeClientUni
	if c.config.Perspective == PerspectiveServer {
		st = StreamTypeServerUni
	}
	streamID := NewStreamID(st, c.nextLocalUniStreamNum)
	c.nextLocalUniStreamNum++

	sfc := NewStreamFlowController(
		streamID,
		c.connFlowController,
		c.config.InitialStreamReceiveWindow,
		c.config.MaxStreamReceiveWindow,
		c.config.InitialStreamReceiveWindow,
		c.config.RTTProvider,
		nil,
	)

	str := NewSendStream(c.ctx, streamID, c, sfc)
	c.sendStreams[streamID] = str
	return str, nil
}

// OpenUniStreamSync opens a new outbound unidirectional stream, blocking if stream limits are reached.
func (c *Conn) OpenUniStreamSync(ctx context.Context) (*SendStream, error) {
	for {
		c.mu.Lock()
		if c.IsClosed() {
			c.mu.Unlock()
			return nil, ErrConnClosed
		}

		if (c.nextLocalUniStreamNum - 1) < c.maxPeerUniStreams {
			st := StreamTypeClientUni
			if c.config.Perspective == PerspectiveServer {
				st = StreamTypeServerUni
			}
			streamID := NewStreamID(st, c.nextLocalUniStreamNum)
			c.nextLocalUniStreamNum++

			sfc := NewStreamFlowController(
				streamID,
				c.connFlowController,
				c.config.InitialStreamReceiveWindow,
				c.config.MaxStreamReceiveWindow,
				c.config.InitialStreamReceiveWindow,
				c.config.RTTProvider,
				nil,
			)

			str := NewSendStream(c.ctx, streamID, c, sfc)
			c.sendStreams[streamID] = str
			c.mu.Unlock()
			return str, nil
		}

		waiter := make(chan struct{})
		c.uniStreamsWaiters = append(c.uniStreamsWaiters, waiter)
		c.mu.Unlock()

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-c.ctx.Done():
			return nil, ErrConnClosed
		case <-waiter:
			continue
		}
	}
}

// AcceptStream accepts the next incoming peer-initiated bidirectional stream (RFC 9000 §2.1).
func (c *Conn) AcceptStream(ctx context.Context) (*Stream, error) {
	select {
	case str, ok := <-c.acceptBidiCh:
		if !ok {
			return nil, ErrConnClosed
		}
		return str, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-c.ctx.Done():
		return nil, ErrConnClosed
	}
}

// AcceptUniStream accepts the next incoming peer-initiated unidirectional stream (RFC 9000 §2.1).
func (c *Conn) AcceptUniStream(ctx context.Context) (*ReceiveStream, error) {
	select {
	case str, ok := <-c.acceptUniCh:
		if !ok {
			return nil, ErrConnClosed
		}
		return str, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-c.ctx.Done():
		return nil, ErrConnClosed
	}
}

// CloseWithError terminates the QUIC connection session with an application error code (RFC 9000 §10.2).
func (c *Conn) CloseWithError(code ApplicationErrorCode, reason string) error {
	c.closeOnce.Do(func() {
		c.mu.Lock()
		c.closeErr = &ApplicationError{
			ErrorCode:    code,
			ErrorMessage: reason,
		}
		close(c.closedCh)
		c.ctxCancel(c.closeErr)

		// Wake all pending stream openers
		for _, w := range c.bidiStreamsWaiters {
			close(w)
		}
		c.bidiStreamsWaiters = nil
		for _, w := range c.uniStreamsWaiters {
			close(w)
		}
		c.uniStreamsWaiters = nil

		// Shutdown all active streams
		for _, str := range c.bidiStreams {
			str.CloseForShutdown(c.closeErr)
		}
		for _, str := range c.sendStreams {
			str.CloseForShutdown(c.closeErr)
		}
		for _, str := range c.recvStreams {
			str.CloseForShutdown(c.closeErr)
		}
		c.mu.Unlock()
	})
	return nil
}

// Close closes the connection session gracefully with application error code 0.
func (c *Conn) Close() error {
	return c.CloseWithError(0, "closed")
}

func (c *Conn) createBidiStreamLocked(id StreamID) *Stream {
	sfc := NewStreamFlowController(
		id,
		c.connFlowController,
		c.config.InitialStreamReceiveWindow,
		c.config.MaxStreamReceiveWindow,
		c.config.InitialStreamReceiveWindow,
		c.config.RTTProvider,
		nil,
	)
	str := NewStream(c.ctx, id, c, sfc, c)
	c.bidiStreams[id] = str
	return str
}

// HandleStreamFrame routes an incoming STREAM frame payload to the target stream (RFC 9000 §2.2).
func (c *Conn) HandleStreamFrame(id StreamID, offset uint64, data []byte, fin bool) error {
	c.mu.Lock()
	if c.IsClosed() {
		c.mu.Unlock()
		return ErrConnClosed
	}

	// Verify initiator and direction
	isPeerInitiated := (id.IsClientInitiated() && c.config.Perspective == PerspectiveServer) ||
		(id.IsServerInitiated() && c.config.Perspective == PerspectiveClient)

	if id.IsBidi() {
		str, exists := c.bidiStreams[id]
		if !exists {
			if _, isClosed := c.closedStreams[id]; isClosed {
				c.mu.Unlock()
				return nil
			}
			if !isPeerInitiated {
				if id.StreamNum() < c.nextLocalBidiStreamNum {
					c.mu.Unlock()
					return nil
				}
				c.mu.Unlock()
				return &TransportError{
					ErrorCode:    StreamStateError,
					ErrorMessage: fmt.Sprintf("received frame for non-existent locally-initiated stream %d", id),
				}
			}
			if id.StreamNum() > c.maxLocalBidiStreams {
				c.mu.Unlock()
				return &TransportError{
					ErrorCode:    StreamLimitError,
					ErrorMessage: fmt.Sprintf("peer stream %d exceeded max bidirectional streams %d", id, c.maxLocalBidiStreams),
				}
			}
			str = c.createBidiStreamLocked(id)
			select {
			case c.acceptBidiCh <- str:
			default:
			}
		}
		c.mu.Unlock()
		return str.recvStr.HandleStreamFrame(offset, data, fin)
	}

	// Unidirectional stream
	if _, isClosed := c.closedStreams[id]; isClosed {
		c.mu.Unlock()
		return nil
	}
	if !isPeerInitiated {
		if id.StreamNum() < c.nextLocalUniStreamNum {
			c.mu.Unlock()
			return nil
		}
		c.mu.Unlock()
		return &TransportError{
			ErrorCode:    StreamStateError,
			ErrorMessage: fmt.Sprintf("received stream frame for outbound-only uni stream %d", id),
		}
	}

	rStr, exists := c.recvStreams[id]
	if !exists {
		if id.StreamNum() > c.maxLocalUniStreams {
			c.mu.Unlock()
			return &TransportError{
				ErrorCode:    StreamLimitError,
				ErrorMessage: fmt.Sprintf("peer stream %d exceeded max unidirectional streams %d", id, c.maxLocalUniStreams),
			}
		}
		sfc := NewStreamFlowController(
			id,
			c.connFlowController,
			c.config.InitialStreamReceiveWindow,
			c.config.MaxStreamReceiveWindow,
			c.config.InitialStreamReceiveWindow,
			c.config.RTTProvider,
			nil,
		)
		rStr = NewReceiveStream(c.ctx, id, c, sfc)
		c.recvStreams[id] = rStr
		select {
		case c.acceptUniCh <- rStr:
		default:
		}
	}
	c.mu.Unlock()
	return rStr.HandleStreamFrame(offset, data, fin)
}

// HandleResetStreamFrame routes an incoming RESET_STREAM frame (RFC 9000 §3.2).
func (c *Conn) HandleResetStreamFrame(id StreamID, code StreamErrorCode, finalSize uint64) error {
	c.mu.Lock()
	if c.IsClosed() {
		c.mu.Unlock()
		return ErrConnClosed
	}

	isPeerInitiated := (id.IsClientInitiated() && c.config.Perspective == PerspectiveServer) ||
		(id.IsServerInitiated() && c.config.Perspective == PerspectiveClient)

	// Validate stream limits on RESET_STREAM per RFC 9000 §4.6
	if isPeerInitiated {
		if id.IsBidi() && id.StreamNum() > c.maxLocalBidiStreams {
			c.mu.Unlock()
			return &TransportError{
				ErrorCode:    StreamLimitError,
				ErrorMessage: fmt.Sprintf("RESET_STREAM for stream %d exceeded max bidirectional streams %d", id, c.maxLocalBidiStreams),
			}
		}
		if !id.IsBidi() && id.StreamNum() > c.maxLocalUniStreams {
			c.mu.Unlock()
			return &TransportError{
				ErrorCode:    StreamLimitError,
				ErrorMessage: fmt.Sprintf("RESET_STREAM for stream %d exceeded max unidirectional streams %d", id, c.maxLocalUniStreams),
			}
		}
	}

	var recvStr *ReceiveStream
	if id.IsBidi() {
		if str, ok := c.bidiStreams[id]; ok {
			recvStr = str.recvStr
		} else if _, isClosed := c.closedStreams[id]; isClosed {
			c.mu.Unlock()
			return nil
		} else if isPeerInitiated {
			str = c.createBidiStreamLocked(id)
			select {
			case c.acceptBidiCh <- str:
			default:
			}
			recvStr = str.recvStr
		}
	} else {
		if rStr, ok := c.recvStreams[id]; ok {
			recvStr = rStr
		} else if _, isClosed := c.closedStreams[id]; isClosed {
			c.mu.Unlock()
			return nil
		} else if isPeerInitiated {
			sfc := NewStreamFlowController(
				id,
				c.connFlowController,
				c.config.InitialStreamReceiveWindow,
				c.config.MaxStreamReceiveWindow,
				c.config.InitialStreamReceiveWindow,
				c.config.RTTProvider,
				nil,
			)
			rStr = NewReceiveStream(c.ctx, id, c, sfc)
			c.recvStreams[id] = rStr
			select {
			case c.acceptUniCh <- rStr:
			default:
			}
			recvStr = rStr
		}
	}
	c.mu.Unlock()

	if recvStr != nil {
		return recvStr.HandleResetStreamFrame(code, finalSize)
	}
	return nil
}

// HandleStopSendingFrame routes an incoming STOP_SENDING frame to the sending half (RFC 9000 §3.5).
func (c *Conn) HandleStopSendingFrame(id StreamID, code StreamErrorCode) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if id.IsBidi() {
		if str, ok := c.bidiStreams[id]; ok {
			str.sendStr.HandleStopSending(code)
		}
	} else {
		if sStr, ok := c.sendStreams[id]; ok {
			sStr.HandleStopSending(code)
		}
	}
}

// HandleMaxDataFrame updates the connection-level send limit (RFC 9000 §4.1).
func (c *Conn) HandleMaxDataFrame(maxData uint64) {
	c.connFlowController.UpdateSendWindow(maxData)
}

// HandleMaxStreamDataFrame updates the stream-level send limit (RFC 9000 §4.1).
func (c *Conn) HandleMaxStreamDataFrame(id StreamID, maxStreamData uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if id.IsBidi() {
		if str, ok := c.bidiStreams[id]; ok {
			str.sendStr.UpdateSendWindow(maxStreamData)
		}
	} else {
		if sStr, ok := c.sendStreams[id]; ok {
			sStr.UpdateSendWindow(maxStreamData)
		}
	}
}

// HandleMaxStreamsFrame updates the peer's advertised stream limits (RFC 9000 §4.6).
func (c *Conn) HandleMaxStreamsFrame(st StreamType, maxStreams uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if st == StreamTypeClientBidi || st == StreamTypeServerBidi {
		if maxStreams > c.maxPeerBidiStreams {
			c.maxPeerBidiStreams = maxStreams
			for _, w := range c.bidiStreamsWaiters {
				close(w)
			}
			c.bidiStreamsWaiters = nil
		}
	} else {
		if maxStreams > c.maxPeerUniStreams {
			c.maxPeerUniStreams = maxStreams
			for _, w := range c.uniStreamsWaiters {
				close(w)
			}
			c.uniStreamsWaiters = nil
		}
	}
}

// OnHasStreamData is invoked when a stream has queued new payload bytes.
func (c *Conn) OnHasStreamData(id StreamID) {}

// OnHasStreamControlFrame is invoked when a stream has a pending control frame.
func (c *Conn) OnHasStreamControlFrame(id StreamID) {}

// OnStreamCompleted is invoked when a stream reaches terminal states.
func (c *Conn) OnStreamCompleted(id StreamID) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.bidiStreams, id)
	delete(c.sendStreams, id)
	delete(c.recvStreams, id)
	c.closedStreams[id] = struct{}{}
}

// GetSendStream retrieves the active SendStream for id (either bidirectional or unidirectional).
func (c *Conn) GetSendStream(id StreamID) (*SendStream, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if id.IsBidi() {
		if str, ok := c.bidiStreams[id]; ok {
			return str.sendStr, true
		}
	} else {
		if sStr, ok := c.sendStreams[id]; ok {
			return sStr, true
		}
	}
	return nil, false
}

// GetReceiveStream retrieves the active ReceiveStream for id (either bidirectional or unidirectional).
func (c *Conn) GetReceiveStream(id StreamID) (*ReceiveStream, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if id.IsBidi() {
		if str, ok := c.bidiStreams[id]; ok {
			return str.recvStr, true
		}
	} else {
		if rStr, ok := c.recvStreams[id]; ok {
			return rStr, true
		}
	}
	return nil, false
}

// MaybeGetConnWindowUpdate checks if a MAX_DATA frame should be sent, auto-tuning the connection receive window.
func (c *Conn) MaybeGetConnWindowUpdate() (newWindow uint64, ok bool) {
	return c.connFlowController.MaybeGetWindowUpdate()
}

