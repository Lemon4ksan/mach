// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package wire

import (
	"io"

	"github.com/lemon4ksan/mach/quic/internal/protocol"
)

// StreamFrameOverlay is an in-situ zero-copy wrapper over a QUIC STREAM frame.
type StreamFrameOverlay struct {
	data     []byte
	typ      FrameType
	streamID protocol.StreamID
	offset   protocol.ByteCount
}

// ParseStreamFrameOverlay creates an in-situ overlay for a STREAM frame.
func ParseStreamFrameOverlay(b []byte, typ FrameType) (StreamFrameOverlay, int, error) {
	startLen := len(b)
	hasOffset := typ&0b100 > 0
	hasDataLen := typ&0b10 > 0

	if len(b) == 0 {
		return StreamFrameOverlay{}, 0, io.EOF
	}

	// 1. StreamID
	first := b[0]

	l1 := int(1 << (first >> 6))
	if len(b) < l1 {
		return StreamFrameOverlay{}, 0, io.EOF
	}

	var streamID uint64
	switch l1 {
	case 1:
		streamID = uint64(first & 0x3f)
	case 2:
		_ = b[1]
		streamID = uint64(first&0x3f)<<8 | uint64(b[1])
	case 4:
		_ = b[3]
		streamID = uint64(first&0x3f)<<24 | uint64(b[1])<<16 | uint64(b[2])<<8 | uint64(b[3])
	case 8:
		_ = b[7]
		streamID = uint64(first&0x3f)<<56 | uint64(b[1])<<48 | uint64(b[2])<<40 | uint64(b[3])<<32 |
			uint64(b[4])<<24 | uint64(b[5])<<16 | uint64(b[6])<<8 | uint64(b[7])
	}

	b = b[l1:]

	// 2. Offset
	var offset uint64
	if hasOffset {
		if len(b) == 0 {
			return StreamFrameOverlay{}, 0, io.EOF
		}

		first = b[0]

		l2 := int(1 << (first >> 6))
		if len(b) < l2 {
			return StreamFrameOverlay{}, 0, io.EOF
		}

		switch l2 {
		case 1:
			offset = uint64(first & 0x3f)
		case 2:
			_ = b[1]
			offset = uint64(first&0x3f)<<8 | uint64(b[1])
		case 4:
			_ = b[3]
			offset = uint64(first&0x3f)<<24 | uint64(b[1])<<16 | uint64(b[2])<<8 | uint64(b[3])
		case 8:
			_ = b[7]
			offset = uint64(first&0x3f)<<56 | uint64(b[1])<<48 | uint64(b[2])<<40 | uint64(b[3])<<32 |
				uint64(b[4])<<24 | uint64(b[5])<<16 | uint64(b[6])<<8 | uint64(b[7])
		}

		b = b[l2:]
	}

	// 3. DataLen
	var dataLen int
	if hasDataLen {
		if len(b) == 0 {
			return StreamFrameOverlay{}, 0, io.EOF
		}

		first = b[0]

		l3 := int(1 << (first >> 6))
		if len(b) < l3 {
			return StreamFrameOverlay{}, 0, io.EOF
		}

		var v uint64
		switch l3 {
		case 1:
			v = uint64(first & 0x3f)
		case 2:
			_ = b[1]
			v = uint64(first&0x3f)<<8 | uint64(b[1])
		case 4:
			_ = b[3]
			v = uint64(first&0x3f)<<24 | uint64(b[1])<<16 | uint64(b[2])<<8 | uint64(b[3])
		case 8:
			_ = b[7]
			v = uint64(first&0x3f)<<56 | uint64(b[1])<<48 | uint64(b[2])<<40 | uint64(b[3])<<32 |
				uint64(b[4])<<24 | uint64(b[5])<<16 | uint64(b[6])<<8 | uint64(b[7])
		}

		dataLen = int(v)
		b = b[l3:]

		if dataLen > len(b) {
			return StreamFrameOverlay{}, 0, io.EOF
		}
	} else {
		dataLen = len(b)
	}

	consumed := startLen - len(b)

	return StreamFrameOverlay{
		data:     b[:dataLen], // Zero-copy payload slicing
		typ:      typ,
		streamID: protocol.StreamID(streamID),
		offset:   protocol.ByteCount(offset),
	}, consumed + dataLen, nil
}

func (o StreamFrameOverlay) Data() []byte                { return o.data }
func (o StreamFrameOverlay) Fin() bool                   { return o.typ&0b1 > 0 }
func (o StreamFrameOverlay) StreamID() protocol.StreamID { return o.streamID }
func (o StreamFrameOverlay) Offset() protocol.ByteCount  { return o.offset }
