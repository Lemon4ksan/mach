// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ws

import (
	"fmt"
	"strconv"
)

// Opcode represents a 4-bit RFC 6455 frame opcode (§5.2).
type Opcode uint8

const (
	// OpcodeContinuation denotes a continuation frame (0x0).
	OpcodeContinuation Opcode = 0x0

	// OpcodeText denotes a text data frame (0x1).
	OpcodeText Opcode = 0x1

	// OpcodeBinary denotes a binary data frame (0x2).
	OpcodeBinary Opcode = 0x2

	// Reserved non-control opcodes: 0x3 - 0x7.
	OpcodeDataUnused3 Opcode = 0x3
	OpcodeDataUnused4 Opcode = 0x4
	OpcodeDataUnused5 Opcode = 0x5
	OpcodeDataUnused6 Opcode = 0x6
	OpcodeDataUnused7 Opcode = 0x7

	// OpcodeClose denotes a connection close control frame (0x8).
	OpcodeClose Opcode = 0x8

	// OpcodePing denotes a ping control frame (0x9).
	OpcodePing Opcode = 0x9

	// OpcodePong denotes a pong control frame (0xA).
	OpcodePong Opcode = 0xA

	// Reserved control opcodes: 0xB - 0xF.
	OpcodeControlUnusedB Opcode = 0xB
	OpcodeControlUnusedC Opcode = 0xC
	OpcodeControlUnusedD Opcode = 0xD
	OpcodeControlUnusedE Opcode = 0xE
	OpcodeControlUnusedF Opcode = 0xF
)

// IsControl reports whether the opcode denotes a control frame (RFC 6455 §5.5).
// Control frames have their most significant bit set (opcodes 0x8 through 0xF).
//
//go:inline
func (op Opcode) IsControl() bool {
	return op >= OpcodeClose && op <= 0xF
}

// IsData reports whether the opcode denotes a valid data frame (0x0, 0x1, 0x2).
//
//go:inline
func (op Opcode) IsData() bool {
	return op <= OpcodeBinary
}

// IsReserved reports whether the opcode is reserved for future definition (RFC 6455 §5.2).
// Reserved data opcodes: 0x3..0x7; reserved control opcodes: 0xB..0xF.
//
//go:inline
func (op Opcode) IsReserved() bool {
	return (op >= OpcodeDataUnused3 && op <= OpcodeDataUnused7) ||
		(op >= OpcodeControlUnusedB && op <= OpcodeControlUnusedF)
}

// IsValid reports whether the opcode is one of the standard known opcodes.
//
//go:inline
func (op Opcode) IsValid() bool {
	switch op {
	case OpcodeContinuation, OpcodeText, OpcodeBinary,
		OpcodeClose, OpcodePing, OpcodePong:
		return true
	default:
		return false
	}
}

// String returns the human-readable representation of the opcode.
func (op Opcode) String() string {
	switch op {
	case OpcodeContinuation:
		return "continuation"
	case OpcodeText:
		return "text"
	case OpcodeBinary:
		return "binary"
	case OpcodeClose:
		return "close"
	case OpcodePing:
		return "ping"
	case OpcodePong:
		return "pong"
	default:
		return "opcode(" + strconv.Itoa(int(op)) + ")"
	}
}

// CloseCode represents an RFC 6455 status code used in Close control frames (§7.4).
type CloseCode uint16

const (
	// StatusNormalClosure indicates normal closure (1000).
	StatusNormalClosure CloseCode = 1000

	// StatusGoingAway indicates endpoint is departing (e.g. server shutdown or navigating away) (1001).
	StatusGoingAway CloseCode = 1001

	// StatusProtocolError indicates endpoint received a frame violating the protocol (1002).
	StatusProtocolError CloseCode = 1002

	// StatusUnsupportedData indicates endpoint received data type it cannot accept (1003).
	StatusUnsupportedData CloseCode = 1003

	// StatusNoStatusReceived indicates no status code was received.
	// Designated for internal use; MUST NOT be set in a Close frame on the wire (1005).
	StatusNoStatusReceived CloseCode = 1005

	// StatusAbnormalClosure indicates abnormal closure (e.g. without sending Close frame).
	// Designated for internal use; MUST NOT be set in a Close frame on the wire (1006).
	StatusAbnormalClosure CloseCode = 1006

	// StatusInvalidFramePayloadData indicates message payload contains data inconsistent with type (e.g. invalid UTF-8) (1007).
	StatusInvalidFramePayloadData CloseCode = 1007

	// StatusPolicyViolation indicates generic policy violation (1008).
	StatusPolicyViolation CloseCode = 1008

	// StatusMessageTooBig indicates message is too large to process (1009).
	StatusMessageTooBig CloseCode = 1009

	// StatusMandatoryExtension indicates client required server to negotiate extension(s) that server failed to (1010).
	StatusMandatoryExtension CloseCode = 1010

	// StatusInternalServerError indicates unexpected condition encountered (1011).
	StatusInternalServerError CloseCode = 1011

	// StatusTLSHandshake indicates TLS handshake failed.
	// Designated for internal use; MUST NOT be set in a Close frame on the wire (1015).
	StatusTLSHandshake CloseCode = 1015
)

// IsValidForWire reports whether the status code is permitted to be transmitted
// in a Close control frame payload according to RFC 6455 §7.4.1 and §7.4.2.
func (c CloseCode) IsValidForWire() bool {
	switch {
	case c < 1000 || c > 4999:
		return false
	case c == StatusNoStatusReceived || c == StatusAbnormalClosure || c == StatusTLSHandshake:
		// RFC 6455 §7.4.2: Reserved for internal use only.
		return false
	case c >= 1000 && c <= 1003:
		return true
	case c >= 1007 && c <= 1011:
		return true
	case c >= 3000 && c <= 4999:
		// 3000-3999: Registered with IANA or frameworks
		// 4000-4999: Reserved for private use
		return true
	default:
		// Reserved or undefined within 1000-2999 range (e.g. 1004, 1012, 1013, 1014, 1016..2999)
		return false
	}
}

// String returns the descriptive representation of the status code.
func (c CloseCode) String() string {
	switch c {
	case StatusNormalClosure:
		return "Normal Closure (1000)"
	case StatusGoingAway:
		return "Going Away (1001)"
	case StatusProtocolError:
		return "Protocol Error (1002)"
	case StatusUnsupportedData:
		return "Unsupported Data (1003)"
	case StatusNoStatusReceived:
		return "No Status Received (1005)"
	case StatusAbnormalClosure:
		return "Abnormal Closure (1006)"
	case StatusInvalidFramePayloadData:
		return "Invalid Frame Payload Data (1007)"
	case StatusPolicyViolation:
		return "Policy Violation (1008)"
	case StatusMessageTooBig:
		return "Message Too Big (1009)"
	case StatusMandatoryExtension:
		return "Mandatory Extension (1010)"
	case StatusInternalServerError:
		return "Internal Server Error (1011)"
	case StatusTLSHandshake:
		return "TLS Handshake Failed (1015)"
	default:
		return fmt.Sprintf("CloseCode(%d)", c)
	}
}

// Opcode aliases for compatibility with alternative naming conventions.
const (
	OpContinuation = OpcodeContinuation
	OpText         = OpcodeText
	OpBinary       = OpcodeBinary
	OpClose        = OpcodeClose
	OpPing         = OpcodePing
	OpPong         = OpcodePong
)

// CloseCode aliases for compatibility.
const (
	StatusReserved1004       = CloseCode(1004)
	StatusNoStatusRcvd       = StatusNoStatusReceived
	StatusInvalidPayloadData = StatusInvalidFramePayloadData
	StatusServiceRestart     = CloseCode(1012)
	StatusTryAgainLater      = CloseCode(1013)
	StatusBadGateway         = CloseCode(1014)
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
		return fmt.Sprintf("ConnState(%d)", uint32(s))
	}
}

// CloseError is returned when a Close frame is received or sent.
type CloseError struct {
	Code   CloseCode
	Reason string
}

func (e *CloseError) Error() string {
	if e.Reason != "" {
		return fmt.Sprintf("ws: close %d: %s", e.Code, e.Reason)
	}

	return fmt.Sprintf("ws: close %d", e.Code)
}
