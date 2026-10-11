// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package stream

import "fmt"

// Perspective indicates whether an endpoint acts as a client or server (RFC 9000 §2.1).
type Perspective uint8

const (
	// PerspectiveClient identifies the client endpoint.
	PerspectiveClient Perspective = 0
	// PerspectiveServer identifies the server endpoint.
	PerspectiveServer Perspective = 1
)

func (p Perspective) String() string {
	if p == PerspectiveClient {
		return "client"
	}
	return "server"
}

// StreamType categorizes a QUIC stream by initiator and directionality (RFC 9000 §2.1).
type StreamType uint8

const (
	// StreamTypeClientBidi represents a client-initiated bidirectional stream (0x00).
	StreamTypeClientBidi StreamType = 0x00
	// StreamTypeServerBidi represents a server-initiated bidirectional stream (0x01).
	StreamTypeServerBidi StreamType = 0x01
	// StreamTypeClientUni represents a client-initiated unidirectional stream (0x02).
	StreamTypeClientUni StreamType = 0x02
	// StreamTypeServerUni represents a server-initiated unidirectional stream (0x03).
	StreamTypeServerUni StreamType = 0x03
)

func (st StreamType) String() string {
	switch st {
	case StreamTypeClientBidi:
		return "client-bidi"
	case StreamTypeServerBidi:
		return "server-bidi"
	case StreamTypeClientUni:
		return "client-uni"
	case StreamTypeServerUni:
		return "server-uni"
	default:
		return fmt.Sprintf("stream-type(%d)", uint8(st))
	}
}

// StreamID represents a 62-bit QUIC Stream ID (RFC 9000 §2.1).
// The least significant two bits identify the initiator and directionality:
// - Bit 0 (0x01): Initiator (0 = Client, 1 = Server)
// - Bit 1 (0x02): Direction (0 = Bidirectional, 1 = Unidirectional)
type StreamID uint64

// InvalidStreamID represents an uninitialized or sentinel stream ID.
const InvalidStreamID StreamID = ^StreamID(0)

// NewStreamID constructs a StreamID from stream type and 1-based sequential stream index.
func NewStreamID(st StreamType, streamNum uint64) StreamID {
	if streamNum == 0 {
		return StreamID(st)
	}
	return StreamID(((streamNum - 1) << 2) | uint64(st))
}

// Type returns the StreamType derived from the lowest 2 bits.
func (id StreamID) Type() StreamType {
	return StreamType(id & 0x03)
}

// Initiator returns whether the stream was initiated by client or server.
func (id StreamID) Initiator() Perspective {
	if id&0x01 == 0 {
		return PerspectiveClient
	}
	return PerspectiveServer
}

// IsClientInitiated reports whether the stream was initiated by the client.
func (id StreamID) IsClientInitiated() bool {
	return id&0x01 == 0
}

// IsServerInitiated reports whether the stream was initiated by the server.
func (id StreamID) IsServerInitiated() bool {
	return id&0x01 != 0
}

// IsBidi reports whether the stream is bidirectional.
func (id StreamID) IsBidi() bool {
	return id&0x02 == 0
}

// IsUni reports whether the stream is unidirectional.
func (id StreamID) IsUni() bool {
	return id&0x02 != 0
}

// StreamNum returns the 1-based ordinal stream number within its stream type class.
func (id StreamID) StreamNum() uint64 {
	return (uint64(id) >> 2) + 1
}

// String returns a human-readable representation of the stream ID.
func (id StreamID) String() string {
	return fmt.Sprintf("stream-%d(%s,#%d)", uint64(id), id.Type(), id.StreamNum())
}
