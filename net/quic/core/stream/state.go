// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package stream

import "fmt"

// SendState represents the state of the sending part of a stream (RFC 9000 §3.1).
type SendState uint8

const (
	// SendStateReady indicates the stream has been opened and is ready to send data.
	SendStateReady SendState = iota
	// SendStateSend indicates data has been sent and more may be sent.
	SendStateSend
	// SendStateDataSent indicates all data including FIN has been sent, awaiting ACKs.
	SendStateDataSent
	// SendStateResetSent indicates RESET_STREAM has been sent, awaiting ACK.
	SendStateResetSent
	// SendStateDataRecvd indicates all data including FIN has been acknowledged (Terminal).
	SendStateDataRecvd
	// SendStateResetRecvd indicates RESET_STREAM has been acknowledged (Terminal).
	SendStateResetRecvd
)

func (s SendState) String() string {
	switch s {
	case SendStateReady:
		return "Ready"
	case SendStateSend:
		return "Send"
	case SendStateDataSent:
		return "DataSent"
	case SendStateResetSent:
		return "ResetSent"
	case SendStateDataRecvd:
		return "DataRecvd"
	case SendStateResetRecvd:
		return "ResetRecvd"
	default:
		return fmt.Sprintf("SendState(%d)", uint8(s))
	}
}

// IsTerminal reports whether the send state machine has completed (RFC 9000 §3.1).
func (s SendState) IsTerminal() bool {
	return s == SendStateDataRecvd || s == SendStateResetRecvd
}

// CanWrite reports whether the send state allows writing new stream data.
func (s SendState) CanWrite() bool {
	return s == SendStateReady || s == SendStateSend
}

// RecvState represents the state of the receiving part of a stream (RFC 9000 §3.2).
type RecvState uint8

const (
	// RecvStateRecv indicates the stream is actively receiving data frames.
	RecvStateRecv RecvState = iota
	// RecvStateSizeKnown indicates FIN has been received, but gaps remain.
	RecvStateSizeKnown
	// RecvStateDataRecvd indicates all data up to final size has been received into buffer.
	RecvStateDataRecvd
	// RecvStateDataRead indicates all data has been read by the application (Terminal).
	RecvStateDataRead
	// RecvStateResetRecvd indicates RESET_STREAM has been received from peer.
	RecvStateResetRecvd
	// RecvStateResetRead indicates the reset has been delivered to the application (Terminal).
	RecvStateResetRead
)

func (s RecvState) String() string {
	switch s {
	case RecvStateRecv:
		return "Recv"
	case RecvStateSizeKnown:
		return "SizeKnown"
	case RecvStateDataRecvd:
		return "DataRecvd"
	case RecvStateDataRead:
		return "DataRead"
	case RecvStateResetRecvd:
		return "ResetRecvd"
	case RecvStateResetRead:
		return "ResetRead"
	default:
		return fmt.Sprintf("RecvState(%d)", uint8(s))
	}
}

// IsTerminal reports whether the receive state machine has completed (RFC 9000 §3.2).
func (s RecvState) IsTerminal() bool {
	return s == RecvStateDataRead || s == RecvStateResetRead
}

// CanRead reports whether the application may still read buffered data from the stream.
func (s RecvState) CanRead() bool {
	return s == RecvStateRecv || s == RecvStateSizeKnown || s == RecvStateDataRecvd
}

// ValidateSendTransition checks whether a state transition from `from` to `to` is valid according to RFC 9000 §3.1.
func ValidateSendTransition(from, to SendState) bool {
	if from == to {
		return true
	}

	switch from {
	case SendStateReady:
		return to == SendStateSend || to == SendStateDataSent || to == SendStateResetSent
	case SendStateSend:
		return to == SendStateDataSent || to == SendStateResetSent
	case SendStateDataSent:
		return to == SendStateDataRecvd || to == SendStateResetSent
	case SendStateResetSent:
		return to == SendStateResetRecvd
	case SendStateDataRecvd, SendStateResetRecvd:
		return false // terminal states
	default:
		return false
	}
}

// ValidateRecvTransition checks whether a state transition from `from` to `to` is valid according to RFC 9000 §3.2.
func ValidateRecvTransition(from, to RecvState) bool {
	if from == to {
		return true
	}

	switch from {
	case RecvStateRecv:
		return to == RecvStateSizeKnown || to == RecvStateDataRecvd || to == RecvStateResetRecvd
	case RecvStateSizeKnown:
		return to == RecvStateDataRecvd || to == RecvStateResetRecvd
	case RecvStateDataRecvd:
		return to == RecvStateDataRead || to == RecvStateResetRecvd
	case RecvStateResetRecvd:
		return to == RecvStateResetRead
	case RecvStateDataRead, RecvStateResetRead:
		return false // terminal states
	default:
		return false
	}
}
