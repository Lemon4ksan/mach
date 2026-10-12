// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package varint

import (
	"encoding/binary"
	"errors"
	"iter"
)

const (
	// Max is the maximum representable QUIC variable-length integer (2^62 - 1).
	Max = 4611686018427387903

	// Max1Byte is the maximum value encodable in 1 byte (63).
	Max1Byte = 63
	// Max2Byte is the maximum value encodable in 2 bytes (16383).
	Max2Byte = 16383
	// Max4Byte is the maximum value encodable in 4 bytes (1073741823).
	Max4Byte = 1073741823
	// Max8Byte is the maximum value encodable in 8 bytes (4611686018427387903).
	Max8Byte = 4611686018427387903
)

var (
	ErrTruncated  = errors.New("quic/varint: truncated varint payload")
	ErrInvalidTag = errors.New("quic/varint: invalid varint tag")
)

// EncodeSlice encodes v into b using QUIC variable-length integer encoding (RFC 9000 Section 16).
func EncodeSlice(v uint64, b []byte) int {
	switch {
	case v < 1<<6:
		b[0] = byte(v)
		return 1
	case v < 1<<14:
		_ = b[1]
		binary.BigEndian.PutUint16(b[:2], uint16(v)|0x4000)
		return 2
	case v < 1<<30:
		_ = b[3]
		binary.BigEndian.PutUint32(b[:4], uint32(v)|0x80000000)
		return 4
	case v < 1<<62:
		_ = b[7]
		binary.BigEndian.PutUint64(b[:8], v|0xc000000000000000)
		return 8
	default:
		panic("quic/varint: value too large for QUIC varint")
	}
}

// Append appends v to b using QUIC variable-length integer encoding (RFC 9000 Section 16).
func Append(b []byte, v uint64) []byte {
	switch {
	case v < 1<<6:
		return append(b, byte(v))
	case v < 1<<14:
		return append(b, byte(v>>8)|0x40, byte(v))
	case v < 1<<30:
		return append(b, byte(v>>24)|0x80, byte(v>>16), byte(v>>8), byte(v))
	case v < 1<<62:
		return append(b,
			byte(v>>56)|0xc0, byte(v>>48), byte(v>>40), byte(v>>32),
			byte(v>>24), byte(v>>16), byte(v>>8), byte(v),
		)
	default:
		panic("quic/varint: value too large for QUIC varint")
	}
}

// Len returns the number of bytes required to encode v as a QUIC varint.
func Len(v uint64) int {
	switch {
	case v < 1<<6:
		return 1
	case v < 1<<14:
		return 2
	case v < 1<<30:
		return 4
	case v < 1<<62:
		return 8
	default:
		panic("quic/varint: value too large for QUIC varint")
	}
}

// Encode encodes val as a QUIC-style variable length integer.
func Encode(val uint64) []byte {
	switch {
	case val <= 63:
		return []byte{byte(val)}

	case val <= 16383:
		b := make([]byte, 2)
		binary.BigEndian.PutUint16(b, uint16(val)|0x4000)
		return b

	case val <= 1073741823:
		b := make([]byte, 4)
		binary.BigEndian.PutUint32(b, uint32(val)|0x80000000)
		return b

	case val < 1<<62:
		b := make([]byte, 8)
		binary.BigEndian.PutUint64(b, val|0xc000000000000000)
		return b

	default:
		panic("quic/varint: value too large for QUIC varint")
	}
}

// Decode decodes a QUIC-style variable length integer from payload.
func Decode(payload []byte) (val uint64, readLen int, err error) {
	if len(payload) == 0 {
		return 0, 0, ErrTruncated
	}

	first := payload[0]
	switch first >> 6 {
	case 0:
		return uint64(first), 1, nil

	case 1:
		if len(payload) < 2 {
			return 0, 0, ErrTruncated
		}
		_ = payload[1]
		return uint64(binary.BigEndian.Uint16(payload[:2]) & 0x3fff), 2, nil

	case 2:
		if len(payload) < 4 {
			return 0, 0, ErrTruncated
		}
		_ = payload[3]
		return uint64(binary.BigEndian.Uint32(payload[:4]) & 0x3fffffff), 4, nil

	default: // case 3
		if len(payload) < 8 {
			return 0, 0, ErrTruncated
		}
		_ = payload[7]
		return binary.BigEndian.Uint64(payload[:8]) & 0x3fffffffffffffff, 8, nil
	}
}

// DecodeSeq returns a push iterator yielding successive decoded varint values and their byte lengths from payload (RFC 9000 §16).
// Iteration stops when payload is exhausted, a truncated or malformed varint is encountered, or yield returns false.
//
// Complexity:
//   - Time: O(N) where N is len(payload). Consumes variable-length integers (1, 2, 4, or 8 bytes) sequentially in linear time.
//   - Space: O(1) zero heap allocations; yields directly via stack frame parameters.
func DecodeSeq(payload []byte) iter.Seq2[uint64, int] {
	return func(yield func(uint64, int) bool) {
		for len(payload) > 0 {
			val, n, err := Decode(payload)
			if err != nil {
				return
			}

			if !yield(val, n) {
				return
			}

			payload = payload[n:]
		}
	}
}

// Values returns a push iterator yielding successive decoded varint values from payload (RFC 9000 §16).
// Iteration stops when payload is exhausted, a truncated or malformed varint is encountered, or yield returns false.
//
// Complexity:
//   - Time: O(N) where N is len(payload).
//   - Space: O(1) zero heap allocations.
func Values(payload []byte) iter.Seq[uint64] {
	return func(yield func(uint64) bool) {
		for len(payload) > 0 {
			val, n, err := Decode(payload)
			if err != nil {
				return
			}

			if !yield(val) {
				return
			}

			payload = payload[n:]
		}
	}
}
