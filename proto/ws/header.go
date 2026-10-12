// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ws

import (
	"encoding/binary"
	"io"
	"slices"
	"unicode/utf8"

	"github.com/lemon4ksan/foundation/silicon/bytesconv"
)

// MaskKey represents a 4-byte RFC 6455 masking key.
type MaskKey = [4]byte

// MaxHeaderSize is the maximum size of a WebSocket frame header in bytes
// (2 base + 8 extended length + 4 masking key = 14 bytes).
const MaxHeaderSize = 14

// Header represents an RFC 6455 frame header (§5.2).
type Header struct {
	// Fin indicates that this is the final fragment in a message.
	Fin bool

	// Rsv1, Rsv2, Rsv3 are reserved extension negotiation bits.
	Rsv1 bool
	Rsv2 bool
	Rsv3 bool

	// Opcode defines the interpretation of the payload data.
	Opcode Opcode

	// Masked indicates whether the payload data is masked with MaskKey.
	// Frames sent from client to server MUST be masked.
	// Frames sent from server to client MUST NOT be masked.
	Masked bool

	// MaskKey is the 4-byte key used to mask the payload.
	MaskKey MaskKey

	// PayloadLen is the payload length in bytes (unextended or extended).
	PayloadLen int64

	// Length is an alias field for PayloadLen for FSM compatibility.
	Length int64
}

// Frame represents a complete RFC 6455 frame with its header and payload slice.
type Frame struct {
	Header  Header
	Payload []byte
}

// HeaderLen calculates the exact byte length needed to encode h into wire format.
//
// Minimum header size is 2 bytes; maximum is 14 bytes (10-byte extended length + 4-byte mask).
//
//go:inline
func HeaderLen(h Header) int {
	n := 2
	if h.PayloadLen > 125 && h.PayloadLen <= 0xFFFF {
		n += 2
	} else if h.PayloadLen > 0xFFFF {
		n += 8
	}
	if h.Masked {
		n += 4
	}
	return n
}

// ParseHeader parses an RFC 6455 frame header from b.
//
// Returns the parsed Header, the total number of bytes consumed by the header,
// or an error if the header is incomplete or invalid.
//
// Zero-allocation: testing.AllocsPerRun == 0.
func ParseHeader(b []byte) (h Header, headerLen int, err error) {
	if len(b) < 2 {
		return Header{}, 0, ErrIncomplete
	}

	b0 := b[0]
	h.Fin = (b0 & 0x80) != 0
	h.Rsv1 = (b0 & 0x40) != 0
	h.Rsv2 = (b0 & 0x20) != 0
	h.Rsv3 = (b0 & 0x10) != 0
	h.Opcode = Opcode(b0 & 0x0F)

	b1 := b[1]
	h.Masked = (b1 & 0x80) != 0
	len7 := b1 & 0x7F

	hlen := 2
	switch len7 {
	case 126:
		hlen += 2
		if len(b) < hlen {
			return Header{}, 0, ErrIncomplete
		}
		val := binary.BigEndian.Uint16(b[2:4])
		// RFC 6455 §5.2: minimal number of bytes MUST be used
		if val < 126 {
			return Header{}, 0, ErrInvalidLength
		}
		h.PayloadLen = int64(val)
	case 127:
		hlen += 8
		if len(b) < hlen {
			return Header{}, 0, ErrIncomplete
		}
		val := binary.BigEndian.Uint64(b[2:10])
		// RFC 6455 §5.2: most significant bit MUST be 0
		if val > 0x7FFFFFFFFFFFFFFF {
			return Header{}, 0, ErrPayloadTooLarge
		}
		// Minimal encoding check: if it fits in 16 bits, 64-bit encoding is forbidden
		if val <= 0xFFFF {
			return Header{}, 0, ErrInvalidLength
		}
		h.PayloadLen = int64(val)
	default:
		h.PayloadLen = int64(len7)
	}

	if h.Masked {
		maskOffset := hlen
		hlen += 4
		if len(b) < hlen {
			return Header{}, 0, ErrIncomplete
		}
		copy(h.MaskKey[:], b[maskOffset:maskOffset+4])
	}

	// RFC 6455 §5.5 Control Frames:
	// "All control frames MUST have a payload length of 125 bytes or less and MUST NOT be fragmented."
	if h.Opcode.IsControl() {
		if !h.Fin {
			return Header{}, 0, ErrControlFrameFragmented
		}
		if h.PayloadLen > 125 {
			return Header{}, 0, ErrControlFrameTooLarge
		}
	}

	return h, hlen, nil
}

// WriteHeader writes the wire-encoded header h into dst.
//
// Returns the number of bytes written or ErrBufferTooSmall if dst does not have enough capacity.
//
// Zero-allocation: testing.AllocsPerRun == 0.
func WriteHeader(dst []byte, h Header) (int, error) {
	if h.PayloadLen < 0 {
		return 0, ErrInvalidLength
	}
	if h.PayloadLen > 0x7FFFFFFFFFFFFFFF {
		return 0, ErrPayloadTooLarge
	}
	if h.Opcode.IsControl() {
		if !h.Fin {
			return 0, ErrControlFrameFragmented
		}
		if h.PayloadLen > 125 {
			return 0, ErrControlFrameTooLarge
		}
	}

	needed := HeaderLen(h)
	if len(dst) < needed {
		return 0, ErrBufferTooSmall
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
	dst[0] = b0

	var b1 byte
	if h.Masked {
		b1 = 0x80
	}

	idx := 2
	if h.PayloadLen <= 125 {
		b1 |= byte(h.PayloadLen)
	} else if h.PayloadLen <= 0xFFFF {
		b1 |= 126
		binary.BigEndian.PutUint16(dst[2:4], uint16(h.PayloadLen))
		idx = 4
	} else {
		b1 |= 127
		binary.BigEndian.PutUint64(dst[2:10], uint64(h.PayloadLen))
		idx = 10
	}
	dst[1] = b1

	if h.Masked {
		copy(dst[idx:idx+4], h.MaskKey[:])
		idx += 4
	}

	return idx, nil
}

// WriteFrame writes a complete frame (header + payload) into dst.
//
// If h.Masked is true, the payload will be masked in-place inside dst using h.MaskKey.
// The source payload slice is NOT modified.
//
// Returns total bytes written or an error.
//
// Zero-allocation: testing.AllocsPerRun == 0.
func WriteFrame(dst []byte, h Header, payload []byte) (int, error) {
	h.PayloadLen = int64(len(payload))
	hlen := HeaderLen(h)
	total := hlen + len(payload)
	if len(dst) < total {
		return 0, ErrBufferTooSmall
	}

	n, err := WriteHeader(dst[:hlen], h)
	if err != nil {
		return 0, err
	}

	if len(payload) > 0 {
		payloadDst := dst[n:total]
		copy(payloadDst, payload)
		if h.Masked {
			Mask(payloadDst, h.MaskKey)
		}
	}

	return total, nil
}

// AppendFrame appends a complete frame (header + payload) to dst and returns
// the extended slice.
//
// If h.Masked is true, payload is copied and masked in dst without mutating the input payload.
func AppendFrame(dst []byte, h Header, payload []byte) []byte {
	h.PayloadLen = int64(len(payload))
	hlen := HeaderLen(h)
	start := len(dst)
	total := hlen + len(payload)

	dst = slices.Grow(dst, total)
	dst = dst[:start+total]

	_, _ = WriteHeader(dst[start:start+hlen], h)

	if len(payload) > 0 {
		payloadDst := dst[start+hlen:]
		copy(payloadDst, payload)
		if h.Masked {
			Mask(payloadDst, h.MaskKey)
		}
	}

	return dst
}

// ParseClosePayload parses the payload of a Close control frame (RFC 6455 §5.5.1).
//
// An empty payload indicates no status code was received (StatusNoStatusReceived).
// A 1-byte payload is invalid according to the RFC and returns ErrInvalidClosePayload.
// If a 2-byte status code is present, it is validated for wire transmission.
// Any trailing bytes represent a UTF-8 encoded close reason string.
func ParseClosePayload(payload []byte) (code CloseCode, reason string, err error) {
	if len(payload) == 0 {
		return StatusNoStatusReceived, "", nil
	}
	if len(payload) == 1 {
		return 0, "", ErrInvalidClosePayload
	}

	code = CloseCode(binary.BigEndian.Uint16(payload[:2]))
	if !code.IsValidForWire() {
		return 0, "", ErrInvalidCloseCode
	}

	if len(payload) > 2 {
		reasonBytes := payload[2:]
		if !utf8.Valid(reasonBytes) {
			return code, "", ErrInvalidUTF8
		}
		reason = bytesconv.B2S(reasonBytes)
	}

	return code, reason, nil
}

// FormatClosePayload encodes a CloseCode and optional reason into dst.
//
// dst must have capacity >= 2 + len(reason).
// Returns the number of bytes written.
func FormatClosePayload(dst []byte, code CloseCode, reason string) (int, error) {
	if len(dst) < 2+len(reason) {
		return 0, ErrBufferTooSmall
	}
	binary.BigEndian.PutUint16(dst[:2], uint16(code))
	n := copy(dst[2:], reason)
	return 2 + n, nil
}

// AppendClosePayload encodes a close status code and optional reason into a newly allocated byte slice.
func AppendClosePayload(code CloseCode, reason string) ([]byte, error) {
	if code == StatusNoStatusReceived {
		return nil, nil
	}
	if !code.IsValidForWire() {
		return nil, ErrInvalidCloseCode
	}
	if len(reason) > 123 {
		return nil, ErrCloseReasonTooLong
	}
	if !utf8.ValidString(reason) {
		return nil, ErrInvalidUTF8
	}
	dst := make([]byte, 2+len(reason))
	_, err := FormatClosePayload(dst, code, reason)
	return dst, err
}

// ReadHeader reads and parses an RFC 6455 frame header from r using buf as scratch.
// buf must have capacity >= 14 bytes (or a new slice will be allocated).
func ReadHeader(r io.Reader, buf []byte) (Header, error) {
	if len(buf) < 14 {
		buf = make([]byte, 14)
	}
	if _, err := io.ReadFull(r, buf[:2]); err != nil {
		return Header{}, err
	}
	len7 := buf[1] & 0x7F
	extra := 0
	if len7 == 126 {
		extra += 2
	} else if len7 == 127 {
		extra += 8
	}
	if (buf[1] & 0x80) != 0 {
		extra += 4
	}
	if extra > 0 {
		if _, err := io.ReadFull(r, buf[2:2+extra]); err != nil {
			return Header{}, err
		}
	}
	h, _, err := ParseHeader(buf[:2+extra])
	if err == nil {
		h.Length = h.PayloadLen
	}
	return h, err
}

// AppendHeader serializes the frame header h into dst and returns the extended slice.
func AppendHeader(dst []byte, h Header) []byte {
	length := h.PayloadLen
	if length == 0 && h.Length > 0 {
		length = h.Length
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
func MaskBytes(data []byte, mask [4]byte) {
	Mask(data, mask)
}

