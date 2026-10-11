// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package stream

import (
	"errors"
	"fmt"
	"net"
	"os"
)

// StreamErrorCode represents an application-specified error code sent in
// RESET_STREAM and STOP_SENDING frames (RFC 9000 §19.19).
type StreamErrorCode uint64

// ApplicationErrorCode represents an application protocol error code sent in
// CONNECTION_CLOSE frames (RFC 9000 §20.2).
type ApplicationErrorCode uint64

// TransportErrorCode represents a QUIC transport error code (RFC 9000 §20.1).
type TransportErrorCode uint64

const (
	// NoError indicates the absence of an error (0x00).
	NoError TransportErrorCode = 0x00
	// InternalError indicates an internal error within the QUIC stack (0x01).
	InternalError TransportErrorCode = 0x01
	// ConnectionRefused indicates the server rejected the connection (0x02).
	ConnectionRefused TransportErrorCode = 0x02
	// FlowControlError indicates a flow control credit limit violation (0x03).
	FlowControlError TransportErrorCode = 0x03
	// StreamLimitError indicates the peer exceeded the advertised stream count limit (0x04).
	StreamLimitError TransportErrorCode = 0x04
	// StreamStateError indicates a frame was received in an invalid stream state (0x05).
	StreamStateError TransportErrorCode = 0x05
	// FinalSizeError indicates an inconsistent or invalid final size was received (0x06).
	FinalSizeError TransportErrorCode = 0x06
	// FrameEncodingError indicates an error occurred while parsing a frame (0x07).
	FrameEncodingError TransportErrorCode = 0x07
	// TransportParameterError indicates invalid transport parameters were negotiated (0x08).
	TransportParameterError TransportErrorCode = 0x08
	// ConnectionIDLimitError indicates the peer provided more Connection IDs than allowed (0x09).
	ConnectionIDLimitError TransportErrorCode = 0x09
	// ProtocolViolation indicates an unspecific protocol rule was violated (0x0a).
	ProtocolViolation TransportErrorCode = 0x0a
	// InvalidToken indicates an invalid retry or address token (0x0b).
	InvalidToken TransportErrorCode = 0x0b
	// ApplicationErrorErrorCode indicates an application-level closure (0x0c).
	ApplicationErrorErrorCode TransportErrorCode = 0x0c
	// CryptoBufferExceeded indicates the cryptographic buffer limit was exceeded (0x0d).
	CryptoBufferExceeded TransportErrorCode = 0x0d
	// KeyUpdateError indicates an error during TLS key update (0x0e).
	KeyUpdateError TransportErrorCode = 0x0e
	// AEADLimitReached indicates the cryptographic packet limit was reached (0x0f).
	AEADLimitReached TransportErrorCode = 0x0f
	// NoViablePath indicates path probing detected no viable network path (0x10).
	NoViablePath TransportErrorCode = 0x10
)

func (e TransportErrorCode) String() string {
	switch e {
	case NoError:
		return "NO_ERROR"
	case InternalError:
		return "INTERNAL_ERROR"
	case ConnectionRefused:
		return "CONNECTION_REFUSED"
	case FlowControlError:
		return "FLOW_CONTROL_ERROR"
	case StreamLimitError:
		return "STREAM_LIMIT_ERROR"
	case StreamStateError:
		return "STREAM_STATE_ERROR"
	case FinalSizeError:
		return "FINAL_SIZE_ERROR"
	case FrameEncodingError:
		return "FRAME_ENCODING_ERROR"
	case TransportParameterError:
		return "TRANSPORT_PARAMETER_ERROR"
	case ConnectionIDLimitError:
		return "CONNECTION_ID_LIMIT_ERROR"
	case ProtocolViolation:
		return "PROTOCOL_VIOLATION"
	case InvalidToken:
		return "INVALID_TOKEN"
	case ApplicationErrorErrorCode:
		return "APPLICATION_ERROR"
	case CryptoBufferExceeded:
		return "CRYPTO_BUFFER_EXCEEDED"
	case KeyUpdateError:
		return "KEY_UPDATE_ERROR"
	case AEADLimitReached:
		return "AEAD_LIMIT_REACHED"
	case NoViablePath:
		return "NO_VIABLE_PATH"
	default:
		return fmt.Sprintf("TRANSPORT_ERROR_0x%x", uint64(e))
	}
}

// StreamError is returned when a stream operation fails due to stream reset or cancellation.
type StreamError struct {
	StreamID  StreamID
	ErrorCode StreamErrorCode
	Remote    bool
}

func (e *StreamError) Error() string {
	orig := "locally"
	if e.Remote {
		orig = "remotely"
	}
	return fmt.Sprintf("stream %d was reset %s with error code %d", e.StreamID, orig, e.ErrorCode)
}

// TransportError represents an RFC 9000 §20.1 connection-level transport error.
type TransportError struct {
	ErrorCode    TransportErrorCode
	ErrorMessage string
}

func (e *TransportError) Error() string {
	if e.ErrorMessage != "" {
		return fmt.Sprintf("%s: %s", e.ErrorCode, e.ErrorMessage)
	}
	return e.ErrorCode.String()
}

// ApplicationError represents an RFC 9000 §20.2 connection-level application closure error.
type ApplicationError struct {
	ErrorCode    ApplicationErrorCode
	ErrorMessage string
}

func (e *ApplicationError) Error() string {
	if e.ErrorMessage != "" {
		return fmt.Sprintf("application error 0x%x: %s", uint64(e.ErrorCode), e.ErrorMessage)
	}
	return fmt.Sprintf("application error 0x%x", uint64(e.ErrorCode))
}

type deadlineError struct{}

func (deadlineError) Error() string   { return "deadline exceeded" }
func (deadlineError) Temporary() bool { return true }
func (deadlineError) Timeout() bool   { return true }
func (deadlineError) Unwrap() error   { return os.ErrDeadlineExceeded }

// ErrDeadline is returned when an I/O operation on a stream times out.
var ErrDeadline net.Error = &deadlineError{}

// Standard sentinel errors.
var (
	ErrStreamClosed      = errors.New("stream closed")
	ErrStreamReset       = errors.New("stream reset")
	ErrConnClosed        = errors.New("connection closed")
	ErrWouldBlock        = errors.New("operation would block")
	ErrWriteLimitReached = errors.New("write limit reached")
	Err0RTTRejected      = errors.New("0-RTT rejected")
)
