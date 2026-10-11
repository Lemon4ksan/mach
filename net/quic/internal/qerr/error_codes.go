// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package qerr

import (
	"crypto/tls"
	"fmt"
)

// TransportErrorCode represents a QUIC transport error code transmitted over wire (RFC 9000 §20.1).
type TransportErrorCode uint64

// QUIC Transport Error Codes (RFC 9000 §20.1 & RFC 9001 §4.8).
const (
	// NoError signals connection closure without an error (RFC 9000 §20.1: 0x0).
	NoError TransportErrorCode = 0x0

	// InternalError signals stack or host defect (RFC 9000 §20.1: 0x1).
	InternalError TransportErrorCode = 0x1

	// ConnectionRefused signals peer refused to accept connection (RFC 9000 §20.1: 0x2).
	ConnectionRefused TransportErrorCode = 0x2

	// FlowControlError signals stream or connection flow-control limit exceeded (RFC 9000 §20.1: 0x3).
	FlowControlError TransportErrorCode = 0x3

	// StreamLimitError signals stream limit exceeded (RFC 9000 §20.1: 0x4).
	StreamLimitError TransportErrorCode = 0x4

	// StreamStateError signals frame received in invalid stream state (RFC 9000 §20.1: 0x5).
	StreamStateError TransportErrorCode = 0x5

	// FinalSizeError signals offset or final size inconsistency (RFC 9000 §20.1: 0x6).
	FinalSizeError TransportErrorCode = 0x6

	// FrameEncodingError signals malformed frame syntax or length (RFC 9000 §20.1: 0x7).
	FrameEncodingError TransportErrorCode = 0x7

	// TransportParameterError signals invalid or inconsistent transport parameters (RFC 9000 §20.1: 0x8).
	TransportParameterError TransportErrorCode = 0x8

	// ConnectionIDLimitError signals number of active connection IDs exceeded limit (RFC 9000 §20.1: 0x9).
	ConnectionIDLimitError TransportErrorCode = 0x9

	// ProtocolViolation signals general unclassified protocol violation (RFC 9000 §20.1: 0xa).
	ProtocolViolation TransportErrorCode = 0xa

	// InvalidToken signals token in Initial packet failed verification (RFC 9000 §20.1: 0xb).
	InvalidToken TransportErrorCode = 0xb

	// ApplicationErrorErrorCode signals application-layer error (RFC 9000 §20.1: 0xc).
	ApplicationErrorErrorCode TransportErrorCode = 0xc

	// CryptoBufferExceeded signals buffering limit for CRYPTO data exceeded (RFC 9000 §20.1: 0xd).
	CryptoBufferExceeded TransportErrorCode = 0xd

	// KeyUpdateError signals peer rejected cryptographic key update (RFC 9000 §20.1: 0xe).
	KeyUpdateError TransportErrorCode = 0xe

	// AEADLimitReached signals packet protection confidentiality or integrity limit reached (RFC 9000 §20.1: 0xf).
	AEADLimitReached TransportErrorCode = 0xf

	// NoViablePathError signals network path incapable of supporting minimum MTU (RFC 9000 §20.1: 0x10).
	NoViablePathError TransportErrorCode = 0x10
)

// IsCryptoError reports whether e is a TLS alert error mapped into the 0x0100-0x01ff space (RFC 9000 §20.1 & RFC 9001 §4.8).
func (e TransportErrorCode) IsCryptoError() bool {
	return e >= 0x100 && e < 0x200
}

// Message is a description of the error.
// It only returns a non-empty string for crypto errors.
func (e TransportErrorCode) Message() string {
	if !e.IsCryptoError() {
		return ""
	}

	return tls.AlertError(e - 0x100).Error()
}

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
	case NoViablePathError:
		return "NO_VIABLE_PATH"
	default:
		if e.IsCryptoError() {
			return fmt.Sprintf("CRYPTO_ERROR %#x", uint16(e))
		}

		return fmt.Sprintf("unknown error code: %#x", uint16(e))
	}
}
