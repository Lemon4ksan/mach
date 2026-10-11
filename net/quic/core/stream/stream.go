// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package stream

import (
	"context"
	"io"
	"net"
	"sync"
	"time"
)

// AddrProvider exposes endpoint network addresses.
type AddrProvider interface {
	LocalAddr() net.Addr
	RemoteAddr() net.Addr
}

// Stream coordinates the sending and receiving halves of a bidirectional QUIC stream (RFC 9000 §2.1 & §3).
// It implements net.Conn, io.ReadWriteCloser, and full HTTP/3 transport methods.
//
// Thread-safety: Stream is fully safe for concurrent readers and writers on independent goroutines.
type Stream struct {
	sendStr *SendStream
	recvStr *ReceiveStream

	mu             sync.Mutex
	sender         StreamSender
	addrProvider   AddrProvider
	sendCompleted  bool
	recvCompleted  bool
	streamComplete bool
}

var (
	_ net.Conn           = &Stream{}
	_ io.ReadWriteCloser = &Stream{}
)

// NewStream creates a new bidirectional Stream with coordinating send and receive halves.
func NewStream(
	ctx context.Context,
	streamID StreamID,
	sender StreamSender,
	flowController *StreamFlowController,
	addrProvider AddrProvider,
) *Stream {
	s := &Stream{
		sender:       sender,
		addrProvider: addrProvider,
	}

	sendCoordSender := &coordStreamSender{
		parent: s,
		isSend: true,
	}
	recvCoordSender := &coordStreamSender{
		parent: s,
		isSend: false,
	}

	s.sendStr = NewSendStream(ctx, streamID, sendCoordSender, flowController)
	s.recvStr = NewReceiveStream(ctx, streamID, recvCoordSender, flowController)
	return s
}

// StreamID returns the unique 62-bit stream identifier.
func (s *Stream) StreamID() StreamID {
	return s.sendStr.StreamID()
}

// LocalAddr returns the local network address of the underlying QUIC connection.
func (s *Stream) LocalAddr() net.Addr {
	if s.addrProvider != nil {
		return s.addrProvider.LocalAddr()
	}
	return nil
}

// RemoteAddr returns the remote network address of the underlying QUIC connection.
func (s *Stream) RemoteAddr() net.Addr {
	if s.addrProvider != nil {
		return s.addrProvider.RemoteAddr()
	}
	return nil
}

// Context returns a context canceled when the send side is closed or reset.
func (s *Stream) Context() context.Context {
	return s.sendStr.Context()
}

// SetPriority sets the HTTP/3 priority parameters (urgency and incremental).
func (s *Stream) SetPriority(urgency int8, incremental bool) {
	s.sendStr.SetPriority(urgency, incremental)
}

// Read reads data from the stream's receive half.
func (s *Stream) Read(p []byte) (int, error) {
	return s.recvStr.Read(p)
}

// Peek fills b with incoming data without consuming it from the receive buffer.
func (s *Stream) Peek(b []byte) (int, error) {
	return s.recvStr.Peek(b)
}

// Write writes data to the stream's send half.
func (s *Stream) Write(p []byte) (int, error) {
	return s.sendStr.Write(p)
}

// WriteWithLimit writes data subject to a rate or frame size limiter.
func (s *Stream) WriteWithLimit(p []byte, limiter func(maxBytes int) int) (int, error) {
	return s.sendStr.WriteWithLimit(p, limiter)
}

// TryWriteAll writes data if all of it can be immediately queued without blocking.
func (s *Stream) TryWriteAll(p []byte) error {
	return s.sendStr.TryWriteAll(p)
}

// SetReliableBoundary is a no-op stub for the RESET_STREAM_AT extension.
func (s *Stream) SetReliableBoundary() {}

// Close closes the send direction of the stream by transmitting a FIN.
func (s *Stream) Close() error {
	return s.sendStr.Close()
}

// CloseWrite closes the send direction of the stream (RFC 9000 §3.1).
func (s *Stream) CloseWrite() error {
	return s.sendStr.CloseWrite()
}

// CloseRead closes the receive direction of the stream by canceling reading with code 0 (RFC 9000 §3.2).
func (s *Stream) CloseRead() error {
	return s.recvStr.CloseRead()
}

// CancelWrite aborts the send direction with RESET_STREAM.
func (s *Stream) CancelWrite(code StreamErrorCode) {
	s.sendStr.CancelWrite(code)
}

// CancelRead aborts the receive direction with STOP_SENDING.
func (s *Stream) CancelRead(code StreamErrorCode) {
	s.recvStr.CancelRead(code)
}

// FinalSize returns the receive side final size in bytes, and true if known.
func (s *Stream) FinalSize() (int64, bool) {
	return s.recvStr.FinalSize()
}

// SetReceiveFinalSizeCallback registers a callback invoked when final size is determined.
func (s *Stream) SetReceiveFinalSizeCallback(callback func(int64)) {
	s.recvStr.SetReceiveFinalSizeCallback(callback)
}

// SetReadDeadline sets the deadline for future Read calls.
func (s *Stream) SetReadDeadline(t time.Time) error {
	return s.recvStr.SetReadDeadline(t)
}

// SetWriteDeadline sets the deadline for future Write calls.
func (s *Stream) SetWriteDeadline(t time.Time) error {
	return s.sendStr.SetWriteDeadline(t)
}

// SetDeadline sets both read and write deadlines.
func (s *Stream) SetDeadline(t time.Time) error {
	rErr := s.recvStr.SetReadDeadline(t)
	wErr := s.sendStr.SetWriteDeadline(t)
	if rErr != nil {
		return rErr
	}
	return wErr
}

// CloseForShutdown tears down both halves immediately during connection abort.
func (s *Stream) CloseForShutdown(err error) {
	s.sendStr.CloseForShutdown(err)
	s.recvStr.CloseForShutdown(err)
}

// SendStream returns the underlying SendStream half.
func (s *Stream) SendStream() *SendStream {
	return s.sendStr
}

// ReceiveStream returns the underlying ReceiveStream half.
func (s *Stream) ReceiveStream() *ReceiveStream {
	return s.recvStr
}

func (s *Stream) onHalfCompleted(isSend bool) {
	s.mu.Lock()
	if isSend {
		s.sendCompleted = true
	} else {
		s.recvCompleted = true
	}

	complete := s.sendCompleted && s.recvCompleted && !s.streamComplete
	if complete {
		s.streamComplete = true
	}
	s.mu.Unlock()

	if complete && s.sender != nil {
		s.sender.OnStreamCompleted(s.StreamID())
	}
}

type coordStreamSender struct {
	parent *Stream
	isSend bool
}

func (c *coordStreamSender) OnHasStreamData(id StreamID) {
	if c.parent.sender != nil {
		c.parent.sender.OnHasStreamData(id)
	}
}

func (c *coordStreamSender) OnHasStreamControlFrame(id StreamID) {
	if c.parent.sender != nil {
		c.parent.sender.OnHasStreamControlFrame(id)
	}
}

func (c *coordStreamSender) OnStreamCompleted(id StreamID) {
	c.parent.onHalfCompleted(c.isSend)
}
