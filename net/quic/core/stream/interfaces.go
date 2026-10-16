// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package stream

import (
	"context"
	"io"
	"net"
	"time"
)

// StreamI defines the complete public interface for bidirectional QUIC streams.
type StreamI interface {
	net.Conn
	io.ReadWriteCloser

	StreamID() StreamID
	Peek(b []byte) (int, error)
	WriteWithLimit(p []byte, limiter func(maxBytes int) int) (int, error)
	TryWriteAll(p []byte) error
	CloseWrite() error
	CloseRead() error
	CancelWrite(code StreamErrorCode)
	CancelRead(code StreamErrorCode)
	Context() context.Context
	SetPriority(urgency int8, incremental bool)
	FinalSize() (int64, bool)
	SetReceiveFinalSizeCallback(callback func(int64))
}

// SendStreamI defines the public interface for outbound unidirectional streams.
type SendStreamI interface {
	io.WriteCloser

	StreamID() StreamID
	WriteWithLimit(p []byte, limiter func(maxBytes int) int) (int, error)
	TryWriteAll(p []byte) error
	CancelWrite(code StreamErrorCode)
	Context() context.Context
	SetWriteDeadline(t time.Time) error
	SetPriority(urgency int8, incremental bool)
}

// ReceiveStreamI defines the public interface for inbound unidirectional streams.
type ReceiveStreamI interface {
	io.ReadCloser

	StreamID() StreamID
	Peek(b []byte) (int, error)
	CancelRead(code StreamErrorCode)
	SetReadDeadline(t time.Time) error
	FinalSize() (int64, bool)
	SetReceiveFinalSizeCallback(callback func(int64))
}

// ConnI defines the public connection session interface required by client/h3 and server/h3.
type ConnI interface {
	OpenStream() (*Stream, error)
	OpenStreamSync(ctx context.Context) (*Stream, error)
	OpenUniStream() (*SendStream, error)
	OpenUniStreamSync(ctx context.Context) (*SendStream, error)
	AcceptStream(ctx context.Context) (*Stream, error)
	AcceptUniStream(ctx context.Context) (*ReceiveStream, error)
	CloseWithError(code ApplicationErrorCode, reason string) error
	Close() error
	Context() context.Context
	IsClosed() bool
	LocalAddr() net.Addr
	RemoteAddr() net.Addr
}

var (
	_ StreamI        = &Stream{}
	_ SendStreamI    = &SendStream{}
	_ ReceiveStreamI = &ReceiveStream{}
)
