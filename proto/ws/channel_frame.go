// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ws

import (
	"encoding/binary"
	"io"
	"math"
	"unicode/utf8"
)

// MaxHeaderSize is the maximum size of a WebSocket frame header in bytes
// (2 base + 8 extended length + 4 masking key = 14 bytes).
const MaxHeaderSize = 14

// ReadHeader reads and parses a WebSocket frame header from r using buf as scratch.
// buf must have a capacity of at least 14 bytes.
// Validates minimal length encoding, control frame constraints, and opcode validity.
func ReadHeader(r io.Reader, buf []byte) (Header, error) {
	if len(buf) < MaxHeaderSize {
		buf = make([]byte, MaxHeaderSize)
	}

	// Read initial 2 bytes
	if _, err := io.ReadFull(r, buf[:2]); err != nil {
		return Header{}, err
	}

	b0 := buf[0]
	b1 := buf[1]

	h := Header{
		Fin:    (b0 & 0x80) != 0,
		Rsv1:   (b0 & 0x40) != 0,
		Rsv2:   (b0 & 0x20) != 0,
		Rsv3:   (b0 & 0x10) != 0,
		Opcode: Opcode(b0 & 0x0F),
		Masked: (b1 & 0x80) != 0,
	}

	// Check opcode validity
	if !h.Opcode.IsValid() {
		return Header{}, ErrUnknownOpcode
	}

	payloadLen := int64(b1 & 0x7F)
	extraBytes := 0

	if payloadLen == 126 {
		extraBytes += 2
	} else if payloadLen == 127 {
		extraBytes += 8
	}

	if h.Masked {
		extraBytes += 4
	}

	if extraBytes > 0 {
		if _, err := io.ReadFull(r, buf[2:2+extraBytes]); err != nil {
			return Header{}, err
		}
	}

	offset := 2
	if payloadLen == 126 {
		extLen := int64(binary.BigEndian.Uint16(buf[offset : offset+2]))
		// RFC 6455 §5.2: minimal number of bytes MUST be used.
		if extLen < 126 {
			return Header{}, ErrProtocolViolation
		}
		h.Length = extLen
		h.PayloadLen = extLen
		offset += 2
	} else if payloadLen == 127 {
		extLen := binary.BigEndian.Uint64(buf[offset : offset+8])
		// RFC 6455 §5.2: the most significant bit MUST be 0.
		if extLen > math.MaxInt64 {
			return Header{}, ErrProtocolViolation
		}
		if extLen < 65536 {
			return Header{}, ErrProtocolViolation
		}
		h.Length = int64(extLen)
		h.PayloadLen = int64(extLen)
		offset += 8
	} else {
		h.Length = payloadLen
		h.PayloadLen = payloadLen
	}

	if h.Masked {
		copy(h.MaskKey[:], buf[offset:offset+4])
	}

	// RFC 6455 §5.5: Control frames MUST NOT be fragmented (FIN=1) and payload length <= 125.
	if h.Opcode.IsControl() {
		if !h.Fin {
			return Header{}, ErrControlFrameFragmented
		}
		if h.Length > 125 {
			return Header{}, ErrControlFrameTooLarge
		}
	}

	return h, nil
}

// AppendHeader serializes the frame header h into dst and returns the extended slice.
// It uses minimal encoding for payload length.
func AppendHeader(dst []byte, h Header) []byte {
	length := h.Length
	if length == 0 && h.PayloadLen > 0 {
		length = h.PayloadLen
	}

	var b0 byte = byte(h.Opcode & 0x0F)
	if h.Fin {
		b0 |= 0x80
	}
	if h.Rsv1 {
		b0 |= 0x40
	}
	if h.Rsv2 {
		b0 |= 0x20
	}
	if h.Rsv3 {
		b0 |= 0x10
	}

	var maskBit byte
	if h.Masked {
		maskBit = 0x80
	}

	switch {
	case length <= 125:
		dst = append(dst, b0, maskBit|byte(length))
	case length <= 0xFFFF:
		dst = append(dst, b0, maskBit|126, byte(length>>8), byte(length))
	default:
		dst = append(dst, b0, maskBit|127,
			byte(length>>56), byte(length>>48), byte(length>>40), byte(length>>32),
			byte(length>>24), byte(length>>16), byte(length>>8), byte(length))
	}

	if h.Masked {
		dst = append(dst, h.MaskKey[0], h.MaskKey[1], h.MaskKey[2], h.MaskKey[3])
	}

	return dst
}

// MaskBytes applies in-place XOR masking to data using mask.
// Directly delegates to vectorized SWAR Mask kernel.
func MaskBytes(data []byte, mask [4]byte) {
	Mask(data, mask)
}

// AppendClosePayload encodes a CloseCode and optional reason into a byte slice.
func AppendClosePayload(code CloseCode, reason string) ([]byte, error) {
	if code == StatusNoStatusRcvd || code == StatusNoStatusReceived {
		return nil, nil
	}
	if !IsValidCloseCode(code) {
		return nil, ErrInvalidCloseCode
	}
	if len(reason) > 123 {
		return nil, ErrCloseReasonTooLong
	}
	if !utf8.ValidString(reason) {
		return nil, ErrInvalidPayload
	}

	buf := make([]byte, 2+len(reason))
	binary.BigEndian.PutUint16(buf[:2], uint16(code))
	copy(buf[2:], reason)
	return buf, nil
}
