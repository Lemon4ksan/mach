// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package handshake

import (
	"errors"
)

// MaxVarint is the maximum value that can be encoded as a QUIC variable-length integer (2^62 - 1).
const MaxVarint uint64 = (1 << 62) - 1

var (
	errVarintUnexpected = errors.New("quic: unexpected EOF while reading varint")
)

// varintLen returns the number of bytes needed to encode v as a QUIC varint.
func varintLen(v uint64) int {
	if v <= 63 {
		return 1
	}
	if v <= 16383 {
		return 2
	}
	if v <= 1073741823 {
		return 4
	}
	return 8
}

// appendVarint appends a QUIC variable-length integer to b.
func appendVarint(b []byte, v uint64) []byte {
	if v <= 63 {
		return append(b, byte(v))
	}
	if v <= 16383 {
		return append(b,
			0x40|byte(v>>8),
			byte(v),
		)
	}
	if v <= 1073741823 {
		return append(b,
			0x80|byte(v>>24),
			byte(v>>16),
			byte(v>>8),
			byte(v),
		)
	}
	return append(b,
		0xc0|byte(v>>56),
		byte(v>>48),
		byte(v>>40),
		byte(v>>32),
		byte(v>>24),
		byte(v>>16),
		byte(v>>8),
		byte(v),
	)
}

// readVarint reads a QUIC variable-length integer from b.
// Returns the decoded value, the number of bytes read, and any error.
func readVarint(b []byte) (val uint64, read int, err error) {
	if len(b) < 1 {
		return 0, 0, errVarintUnexpected
	}
	first := b[0]
	prefix := first >> 6
	length := 1 << prefix
	if len(b) < length {
		return 0, 0, errVarintUnexpected
	}
	val = uint64(first & 0x3f)
	for i := 1; i < length; i++ {
		val = (val << 8) | uint64(b[i])
	}
	return val, length, nil
}
