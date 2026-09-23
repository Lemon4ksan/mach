// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h2

import (
	"fmt"
)

// Ping measures round-trip time and verifies connection liveness with an 8-octet opaque payload (RFC 9113 §6.7).
//
// PING frames MUST be sent on stream 0x00. Receipt of a PING frame with any other stream identifier
// MUST be treated as a connection error of type PROTOCOL_ERROR.
//
// Concurrency: Not thread-safe; owned by a single goroutine.
// Lifecycle: POD struct. Eligible for off-heap slab allocation via [ConnectionFramePool] or [framePools].
type Ping struct {
	ack  bool
	data [8]byte
}

// Type returns the FrameType identifier FramePing (0x6, RFC 9113 §6.7).
func (p *Ping) Type() FrameType { return FramePing }

// IsAck reports whether the ACK flag bit (0x1) is set, indicating a PING response (RFC 9113 §6.7).
func (p *Ping) IsAck() bool { return p.ack }

// SetAck sets or clears the ACK flag bit (0x1) on the PING frame (RFC 9113 §6.7).
func (p *Ping) SetAck(ack bool) { p.ack = ack }

// Reset clears the ACK flag and resets the PING frame state for pool reuse.
func (p *Ping) Reset() { p.ack = false }

// Data returns the 8-octet opaque payload of the PING frame (RFC 9113 §6.7).
func (p *Ping) Data() []byte { return p.data[:] }

// SetData copies up to 8 octets from b into the PING frame opaque payload (RFC 9113 §6.7).
func (p *Ping) SetData(b []byte) { copy(p.data[:], b) }

// Write copies up to 8 octets from b into the PING payload, satisfying the [io.Writer] interface.
func (p *Ping) Write(b []byte) (int, error) { copy(p.data[:], b); return len(b), nil }

// Deserialize decodes the 8-octet PING payload and ACK flag from frh, validating stream 0x00 binding (RFC 9113 §6.7).
func (p *Ping) Deserialize(frh *FrameHeader) error {
	if frh.Stream() != 0 {
		return NewGoAwayError(ProtocolError, "PING frame must be on stream 0")
	}

	p.ack = frh.Flags().Has(FlagAck)
	if len(frh.payload) != 8 {
		return NewGoAwayError(FrameSizeError, "invalid PING frame size (RFC 9113 §6.7)")
	}

	p.SetData(frh.payload)

	return nil
}

// Serialize encodes the 8-octet PING payload and ACK flag into the destination FrameHeader (RFC 9113 §6.7).
func (p *Ping) Serialize(fr *FrameHeader) {
	if p.ack {
		fr.SetFlags(fr.Flags().Add(FlagAck))
	}

	fr.setPayload(p.data[:])
}

// GoAway initiates connection shutdown or reports fatal connection-level protocol violations (RFC 9113 §6.8).
//
// GOAWAY frames MUST be sent on stream 0x00. It allows an endpoint to gracefully stop accepting new
// streams while finishing processing of previously established streams.
//
// Concurrency: Not thread-safe; owned by a single goroutine.
// Lifecycle: Managed by global [framePools] (sync.Pool) due to dynamic debug data slice. Call [GoAway.Reset] prior to reuse.
type GoAway struct {
	stream uint32
	code   ErrorCode
	data   []byte
}

// Type returns the FrameType identifier FrameGoAway (0x7, RFC 9113 §6.8).
func (ga *GoAway) Type() FrameType { return FrameGoAway }

// Reset clears the stream identifier, error code, and debug data buffer for pool reuse.
func (ga *GoAway) Reset() { ga.stream = 0; ga.code = 0; ga.data = ga.data[:0] }

// Code returns the 32-bit HTTP/2 error code describing the shutdown cause (RFC 9113 §6.8 & §7).
func (ga *GoAway) Code() ErrorCode { return ga.code }

// SetCode sets the 32-bit HTTP/2 error code on the GOAWAY frame (RFC 9113 §6.8 & §7).
func (ga *GoAway) SetCode(code ErrorCode) { ga.code = code & (1<<31 - 1) }

// Stream returns the 31-bit last stream identifier processed by the sender (RFC 9113 §6.8).
func (ga *GoAway) Stream() uint32 { return ga.stream }

// SetStream sets the 31-bit last stream identifier processed by the sender (RFC 9113 §6.8).
func (ga *GoAway) SetStream(stream uint32) { ga.stream = stream & (1<<31 - 1) }

// Data returns the additional opaque debug data octets carried by the GOAWAY frame (RFC 9113 §6.8).
func (ga *GoAway) Data() []byte { return ga.data }

// SetData replaces the opaque debug data with a copy of b.
func (ga *GoAway) SetData(b []byte) { ga.data = append(ga.data[:0], b...) }

// Error formats the GOAWAY stream ID, error code, and debug data into a human-readable diagnostic string.
func (ga *GoAway) Error() string {
	return fmt.Sprintf("stream=%d, code=%s, data=%s", ga.stream, ga.code, ga.data)
}

// Deserialize decodes the last stream identifier, error code, and debug data from fr (RFC 9113 §6.8).
func (ga *GoAway) Deserialize(fr *FrameHeader) error {
	if fr.Stream() != 0 {
		return NewGoAwayError(ProtocolError, "GOAWAY frame must be on stream 0")
	}

	if len(fr.payload) < 8 {
		return NewGoAwayError(FrameSizeError, "invalid GOAWAY frame size (RFC 9113 §6.8)")
	}

	ga.stream = bytesToUint32(fr.payload) & (1<<31 - 1)
	ga.code = ErrorCode(bytesToUint32(fr.payload[4:]))

	if len(fr.payload) > 8 {
		ga.data = append(ga.data[:0], fr.payload[8:]...)
	}

	return nil
}

// Serialize encodes the last stream ID, error code, and debug data into the destination FrameHeader (RFC 9113 §6.8).
func (ga *GoAway) Serialize(fr *FrameHeader) {
	fr.payload = appendUint32Bytes(fr.payload[:0], ga.stream)
	fr.payload = appendUint32Bytes(fr.payload, uint32(ga.code))
	fr.payload = append(fr.payload, ga.data...)
}

// RstStream signals immediate termination of an individual stream (RFC 9113 §6.4).
//
// RST_STREAM frames MUST be associated with a specific stream. If received on stream 0x00,
// the recipient MUST treat it as a connection error of type PROTOCOL_ERROR.
//
// Concurrency: Not thread-safe; owned by a single goroutine.
// Lifecycle: POD struct. Eligible for off-heap slab allocation via [ConnectionFramePool] or [framePools].
type RstStream struct {
	code ErrorCode
}

// Type returns the FrameType identifier FrameResetStream (0x3, RFC 9113 §6.4).
func (rst *RstStream) Type() FrameType { return FrameResetStream }

// Code returns the 32-bit error code indicating why the stream was reset (RFC 9113 §6.4 & §7).
func (rst *RstStream) Code() ErrorCode { return rst.code }

// SetCode sets the 32-bit error code on the RST_STREAM frame (RFC 9113 §6.4 & §7).
func (rst *RstStream) SetCode(code ErrorCode) { rst.code = code }

// Reset clears the error code to zero for pool reuse.
func (rst *RstStream) Reset() { rst.code = 0 }

// Error returns the underlying [ErrorCode] as a standard Go error.
func (rst *RstStream) Error() error { return rst.code }

// Deserialize decodes the 4-octet error code from fr, validating stream binding (RFC 9113 §6.4).
func (rst *RstStream) Deserialize(fr *FrameHeader) error {
	if fr.Stream() == 0 {
		return NewGoAwayError(ProtocolError, "RST_STREAM frame must be on a specific stream, not 0")
	}

	if len(fr.payload) != 4 {
		return NewGoAwayError(FrameSizeError, "invalid RST_STREAM frame size (RFC 9113 §6.4)")
	}

	rst.code = ErrorCode(bytesToUint32(fr.payload))

	return nil
}

// Serialize encodes the 4-octet error code into the destination FrameHeader (RFC 9113 §6.4).
func (rst *RstStream) Serialize(fr *FrameHeader) {
	fr.payload = appendUint32Bytes(fr.payload[:0], uint32(rst.code))
	fr.length = 4
}

// Priority specifies stream dependency and weight (RFC 9113 §6.3, deprecated per §5.3.2).
//
// PRIORITY frames can be sent on any stream state. Priority signaling does not alter stream state.
//
// Concurrency: Not thread-safe; owned by a single goroutine.
// Lifecycle: POD struct. Eligible for off-heap slab allocation via [ConnectionFramePool] or [framePools].
type Priority struct {
	exclusive bool
	stream    uint32
	weight    byte
}

// Type returns the FrameType identifier FramePriority (0x2, RFC 9113 §6.3).
func (pry *Priority) Type() FrameType { return FramePriority }

// Reset clears priority parameters to their zero values for pool reuse.
func (pry *Priority) Reset() { pry.exclusive = false; pry.stream = 0; pry.weight = 0 }

// Stream returns the 31-bit stream dependency identifier (RFC 9113 §6.3 & §5.3.1).
func (pry *Priority) Stream() uint32 { return pry.stream }

// SetStream sets the 31-bit stream dependency identifier (RFC 9113 §6.3 & §5.3.1).
func (pry *Priority) SetStream(stream uint32) { pry.stream = stream & (1<<31 - 1) }

// Weight returns the priority weight octet representing value weight+1 in range [1, 256] (RFC 9113 §6.3 & §5.3.2).
func (pry *Priority) Weight() byte { return pry.weight }

// SetWeight sets the priority weight octet (RFC 9113 §6.3 & §5.3.2).
func (pry *Priority) SetWeight(w byte) { pry.weight = w }

// Exclusive reports whether the exclusive dependency bit is enabled (RFC 9113 §6.3 & §5.3.1).
func (pry *Priority) Exclusive() bool { return pry.exclusive }

// SetExclusive sets or clears the exclusive dependency bit (RFC 9113 §6.3 & §5.3.1).
func (pry *Priority) SetExclusive(v bool) { pry.exclusive = v }

// Deserialize decodes the 5-octet stream dependency and weight payload from fr (RFC 9113 §6.3).
func (pry *Priority) Deserialize(fr *FrameHeader) error {
	if fr.Stream() == 0 {
		return NewGoAwayError(ProtocolError, "PRIORITY frame must be on a specific stream, not 0")
	}

	if len(fr.payload) != 5 {
		return NewGoAwayError(FrameSizeError, "invalid PRIORITY frame size (RFC 9113 §6.3)")
	}

	pry.exclusive = (fr.payload[0] & 0x80) != 0

	pry.stream = bytesToUint32(fr.payload) & (1<<31 - 1)
	if pry.stream == fr.Stream() {
		return NewGoAwayError(ProtocolError, "stream cannot depend on itself (RFC 9113 §5.3.1)")
	}

	pry.weight = fr.payload[4]

	return nil
}

// Serialize encodes the 5-octet stream dependency, exclusive bit, and weight into fr (RFC 9113 §6.3).
func (pry *Priority) Serialize(fr *FrameHeader) {
	fr.payload = appendUint32Bytes(fr.payload[:0], pry.stream)
	if pry.exclusive {
		fr.payload[0] |= 0x80
	}

	fr.payload = append(fr.payload, pry.weight)
}
