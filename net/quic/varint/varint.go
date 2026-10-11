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
		binary.BigEndian.PutUint16(b[:2], uint16(v)|0x4000)
		return 2
	case v < 1<<30:
		binary.BigEndian.PutUint32(b[:4], uint32(v)|0x80000000)
		return 4
	case v < 1<<62:
		binary.BigEndian.PutUint64(b[:8], v|0xc000000000000000)
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
	tag := first >> 6

	switch tag {
	case 0:
		return uint64(first & 0x3f), 1, nil

	case 1:
		if len(payload) < 2 {
			return 0, 0, ErrTruncated
		}
		v := binary.BigEndian.Uint16(payload[:2]) & 0x3fff
		return uint64(v), 2, nil

	case 2:
		if len(payload) < 4 {
			return 0, 0, ErrTruncated
		}
		v := binary.BigEndian.Uint32(payload[:4]) & 0x3fffffff
		return uint64(v), 4, nil

	case 3:
		if len(payload) < 8 {
			return 0, 0, ErrTruncated
		}
		v := binary.BigEndian.Uint64(payload[:8]) & 0x3fffffffffffffff
		return v, 8, nil
	}

	return 0, 0, ErrInvalidTag
}

// DecodeSeq returns a push iterator yielding successive decoded varint values and their byte lengths from payload.
// Iteration stops when payload is exhausted, a truncated or malformed varint is encountered, or yield returns false.
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

// Values returns a push iterator yielding successive decoded varint values from payload.
// Iteration stops when payload is exhausted, a truncated or malformed varint is encountered, or yield returns false.
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
