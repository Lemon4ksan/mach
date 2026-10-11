// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package wire

import (
	"errors"
	"fmt"
)

// TransportErrorCode represents a QUIC transport error code (RFC 9000 §20.1).
type TransportErrorCode uint64

const (
	// NoError indicates that the connection is closing cleanly.
	NoError TransportErrorCode = 0x00
	// InternalError indicates that the endpoint encountered an internal error.
	InternalError TransportErrorCode = 0x01
	// ConnectionRefused indicates that the server refused the connection.
	ConnectionRefused TransportErrorCode = 0x02
	// FlowControlError indicates that an endpoint received more data than permitted.
	FlowControlError TransportErrorCode = 0x03
	// StreamLimitError indicates that an endpoint received a frame on a stream exceeding the limit.
	StreamLimitError TransportErrorCode = 0x04
	// StreamStateError indicates that a frame was received in an invalid stream state.
	StreamStateError TransportErrorCode = 0x05
	// FinalSizeError indicates an inconsistent final size for a stream.
	FinalSizeError TransportErrorCode = 0x06
	// FrameEncodingError indicates that a frame was improperly encoded.
	FrameEncodingError TransportErrorCode = 0x07
	// TransportParameterError indicates that transport parameters were malformed.
	TransportParameterError TransportErrorCode = 0x08
	// ConnectionIDLimitError indicates an endpoint received more connection IDs than permitted.
	ConnectionIDLimitError TransportErrorCode = 0x09
	// ProtocolViolation indicates an unclassified protocol rule was violated.
	ProtocolViolation TransportErrorCode = 0x0a
	// InvalidToken indicates an invalid Retry or NEW_TOKEN was received.
	InvalidToken TransportErrorCode = 0x0b
	// ApplicationError indicates application-specific error.
	ApplicationError TransportErrorCode = 0x0c
	// CryptoBufferExceeded indicates cryptographic handshake buffer limit exceeded.
	CryptoBufferExceeded TransportErrorCode = 0x0d
	// KeyUpdateError indicates an error occurred during key update.
	KeyUpdateError TransportErrorCode = 0x0e
	// AEADLimitReached indicates confidentiality or integrity limits reached.
	AEADLimitReached TransportErrorCode = 0x0f
	// NoViablePath indicates no network path could be validated.
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
	case ApplicationError:
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

// Common wire engine errors.
var (
	ErrTruncatedPacket        = errors.New("wire: packet is truncated")
	ErrTruncatedHeader        = errors.New("wire: header is truncated")
	ErrTruncatedFrame         = errors.New("wire: frame is truncated")
	ErrInvalidFixedBit        = errors.New("wire: fixed bit is zero (must be 1)")
	ErrInvalidReservedBits    = errors.New("wire: reserved bits are non-zero")
	ErrInvalidPacketNumber    = errors.New("wire: invalid packet number")
	ErrInvalidVersion         = errors.New("wire: unsupported QUIC version")
	ErrInvalidConnectionIDLen = errors.New("wire: connection ID length exceeds maximum of 20 bytes")
	ErrInvalidFrameType       = errors.New("wire: unrecognized or invalid frame type")
	ErrSampleOutOfBounds      = errors.New("wire: sample bytes exceed packet boundaries")
	ErrDecryptionFailed       = errors.New("wire: AEAD authentication / decryption failed")
)
