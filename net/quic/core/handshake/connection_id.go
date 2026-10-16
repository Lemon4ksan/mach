// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package handshake

import (
	"bytes"
	"encoding/hex"
)

// MaxConnectionIDLen is the maximum length of a QUIC connection ID as defined in RFC 9000 Section 5.1.
const MaxConnectionIDLen = 20

// ConnectionID is a stack-allocated zero-heap-allocation QUIC connection ID.
type ConnectionID struct {
	len uint8
	buf [MaxConnectionIDLen]byte
}

// NewConnectionID creates a ConnectionID from a byte slice.
func NewConnectionID(b []byte) ConnectionID {
	var cid ConnectionID

	n := copy(cid.buf[:], b)
	if n > MaxConnectionIDLen {
		n = MaxConnectionIDLen
	}

	cid.len = uint8(n)

	return cid
}

// Len returns the length of the connection ID in bytes.
func (c ConnectionID) Len() int {
	return int(c.len)
}

// Bytes returns the connection ID bytes.
func (c ConnectionID) Bytes() []byte {
	return c.buf[:c.len]
}

// Equal returns whether two connection IDs are identical.
func (c ConnectionID) Equal(other ConnectionID) bool {
	if c.len != other.len {
		return false
	}

	return bytes.Equal(c.buf[:c.len], other.buf[:other.len])
}

// String returns the hex representation of the connection ID.
func (c ConnectionID) String() string {
	if c.len == 0 {
		return "(empty)"
	}

	return hex.EncodeToString(c.buf[:c.len])
}
