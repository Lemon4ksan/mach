// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ws

import (
	"errors"
	"fmt"
)

// Opcode aliases for channel FSM compatibility.
const (
	OpContinuation = OpcodeContinuation
	OpText         = OpcodeText
	OpBinary       = OpcodeBinary
	OpClose        = OpcodeClose
	OpPing         = OpcodePing
	OpPong         = OpcodePong
)

// ConnState represents the WebSocket connection lifecycle state.
type ConnState uint32

const (
	StateConnecting ConnState = 0
	StateOpen       ConnState = 1
	StateClosing    ConnState = 2
	StateClosed     ConnState = 3
)

// String returns the human-readable string representation of ConnState.
func (s ConnState) String() string {
	switch s {
	case StateConnecting:
		return "Connecting"
	case StateOpen:
		return "Open"
	case StateClosing:
		return "Closing"
	case StateClosed:
		return "Closed"
	default:
		return fmt.Sprintf("UnknownState(%d)", uint32(s))
	}
}

// CloseCode status aliases for channel FSM compatibility.
const (
	StatusReserved1004       CloseCode = 1004 // Reserved; MUST NOT be set on wire
	StatusNoStatusRcvd       CloseCode = 1005 // Reserved; MUST NOT be set on wire
	StatusInvalidPayloadData CloseCode = 1007
	StatusServiceRestart     CloseCode = 1012
	StatusTryAgainLater      CloseCode = 1013
	StatusBadGateway         CloseCode = 1014 // Reserved / Bad Gateway; MUST NOT be set on wire
)

// IsValidCloseCode reports whether code can be legitimately sent on the wire.
func IsValidCloseCode(code CloseCode) bool {
	switch {
	case code >= 1000 && code <= 1003:
		return true
	case code >= 1007 && code <= 1013:
		return true
	case code >= 3000 && code <= 4999:
		return true
	default:
		return false
	}
}

// IsValidReceivedCloseCode reports whether code received on the wire is valid.
func IsValidReceivedCloseCode(code CloseCode) bool {
	return IsValidCloseCode(code)
}

// CloseError is returned when a Close frame is received or sent.
type CloseError struct {
	Code   CloseCode
	Reason string
}

func (e *CloseError) Error() string {
	if e.Reason != "" {
		return fmt.Sprintf("websocket: close %d: %s", e.Code, e.Reason)
	}
	return fmt.Sprintf("websocket: close %d", e.Code)
}

// Sentinel errors for WebSocket operations.
var (
	ErrClosed                 = errors.New("websocket: connection closed")
	ErrClosing                = errors.New("websocket: connection closing")
	ErrMessageTooBig          = errors.New("websocket: message too big")
	ErrUnexpectedContinuation = errors.New("websocket: unexpected continuation frame")
	ErrExpectedContinuation   = errors.New("websocket: expected continuation frame")
	ErrMaskRequired           = errors.New("websocket: unmasked frame received by server")
	ErrMaskForbidden          = errors.New("websocket: masked frame received by client")
	ErrRSVSet                 = errors.New("websocket: reserved bits RSV1/2/3 set without extension")
	ErrUnknownOpcode          = errors.New("websocket: unknown opcode")
	ErrCloseReasonTooLong     = errors.New("websocket: close reason exceeds 123 bytes")
	ErrDataAfterClose         = errors.New("websocket: data frame received after close")
)
