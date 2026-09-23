// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h2

// WindowUpdate implements stream and connection-level flow control credits (RFC 9113 §6.9).
//
// WINDOW_UPDATE frames can be sent on stream 0x00 (connection flow control) or on an active stream.
// The window size increment MUST NOT be zero (treated as PROTOCOL_ERROR per RFC 9113 §6.9).
//
// Concurrency: Not thread-safe; owned by a single goroutine.
// Lifecycle: POD struct. Eligible for off-heap slab allocation via [ConnectionFramePool] or [framePools].
type WindowUpdate struct {
	increment int
}

// Type returns the FrameType identifier FrameWindowUpdate (0x8, RFC 9113 §6.9).
func (wu *WindowUpdate) Type() FrameType { return FrameWindowUpdate }

// Reset clears the window increment to zero for pool reuse.
func (wu *WindowUpdate) Reset() { wu.increment = 0 }

// Increment returns the flow-control window increment in octets (RFC 9113 §6.9).
func (wu *WindowUpdate) Increment() int { return wu.increment }

// SetIncrement sets the flow-control window increment in octets (RFC 9113 §6.9).
func (wu *WindowUpdate) SetIncrement(inc int) { wu.increment = inc }

// Deserialize decodes the 4-octet window size increment from fr, validating non-zero increments (RFC 9113 §6.9).
func (wu *WindowUpdate) Deserialize(fr *FrameHeader) error {
	if len(fr.payload) != 4 {
		wu.increment = 0
		return NewGoAwayError(FrameSizeError, "invalid WINDOW_UPDATE frame size (RFC 9113 §6.9)")
	}

	wu.increment = int(bytesToUint32(fr.payload) & (1<<31 - 1))
	if wu.increment == 0 {
		if fr.Stream() == 0 {
			return NewGoAwayError(ProtocolError, "window increment of zero on connection")
		}

		return NewResetStreamError(ProtocolError, "window increment of zero on stream")
	}

	return nil
}

// Serialize encodes the 4-octet window size increment into the destination FrameHeader (RFC 9113 §6.9).
func (wu *WindowUpdate) Serialize(fr *FrameHeader) {
	fr.payload = appendUint32Bytes(fr.payload[:0], uint32(wu.increment)) //nolint:gosec
	fr.length = 4
}
