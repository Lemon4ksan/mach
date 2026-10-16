// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package quic

import (
	"errors"

	"github.com/lemon4ksan/mach/net/quic/core/stream"
)

type (
	// StreamErrorCode represents an application-specified error code sent in
	// RESET_STREAM and STOP_SENDING frames (RFC 9000 §19.19).
	StreamErrorCode = stream.StreamErrorCode

	// ApplicationErrorCode represents an application protocol error code sent in
	// CONNECTION_CLOSE frames (RFC 9000 §20.2).
	ApplicationErrorCode = stream.ApplicationErrorCode

	// TransportErrorCode represents a QUIC transport error code (RFC 9000 §20.1).
	TransportErrorCode = stream.TransportErrorCode

	// ApplicationError represents an RFC 9000 §20.2 connection-level application closure error.
	ApplicationError = stream.ApplicationError

	// TransportError represents an RFC 9000 §20.1 connection-level transport error.
	TransportError = stream.TransportError

	// StreamError is returned when a stream operation fails due to stream reset or cancellation.
	StreamError = stream.StreamError
)

const (
	NoError                   = stream.NoError
	InternalError             = stream.InternalError
	ConnectionRefused         = stream.ConnectionRefused
	FlowControlError          = stream.FlowControlError
	StreamLimitError          = stream.StreamLimitError
	StreamStateError          = stream.StreamStateError
	FinalSizeError            = stream.FinalSizeError
	FrameEncodingError        = stream.FrameEncodingError
	TransportParameterError   = stream.TransportParameterError
	ConnectionIDLimitError    = stream.ConnectionIDLimitError
	ProtocolViolation         = stream.ProtocolViolation
	InvalidToken              = stream.InvalidToken
	ApplicationErrorErrorCode = stream.ApplicationErrorErrorCode
	CryptoBufferExceeded      = stream.CryptoBufferExceeded
	KeyUpdateError            = stream.KeyUpdateError
	AEADLimitReached          = stream.AEADLimitReached
	NoViablePathError         = stream.NoViablePath
)

var (
	// ErrConnectionClosed is returned when an operation is attempted on a closed connection.
	ErrConnectionClosed = stream.ErrConnClosed

	// ErrStreamReset is returned when a stream was canceled or reset.
	ErrStreamReset = stream.ErrStreamReset

	// Err0RTTRejected is returned when 0-RTT is rejected.
	Err0RTTRejected = stream.Err0RTTRejected

	// ErrWouldBlock is returned when an operation would block.
	ErrWouldBlock = stream.ErrWouldBlock

	// ErrWriteLimitReached is returned when a write limit is reached.
	ErrWriteLimitReached = stream.ErrWriteLimitReached

	// ErrDeadline is returned when an I/O deadline is exceeded.
	ErrDeadline = stream.ErrDeadline

	// ErrServerClosed indicates the listener or server has been closed.
	ErrServerClosed = errors.New("quic: server closed")

	// ErrTransportClosed indicates the transport has been closed.
	ErrTransportClosed = errors.New("quic: transport closed")
)
