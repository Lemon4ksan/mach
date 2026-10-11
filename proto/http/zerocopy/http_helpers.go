// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package zerocopy

import (
	"bytes"
	"encoding/binary"

	"github.com/lemon4ksan/foundation/silicon/bytesconv"
)

const (
	rChar = '\r'
	nChar = '\n'
)

// CaseInsensitiveCompare reports whether byte slices a and b are equal under ASCII case-insensitivity.
func CaseInsensitiveCompare(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}

	i := 0
	for ; i+8 <= len(a); i += 8 {
		va := binary.LittleEndian.Uint64(a[i:])

		vb := binary.LittleEndian.Uint64(b[i:])
		if (va | 0x2020202020202020) != (vb | 0x2020202020202020) {
			return false
		}
	}

	for ; i < len(a); i++ {
		if a[i]|0x20 != b[i]|0x20 {
			return false
		}
	}

	return true
}

// ValidHeaderFieldByte reports whether c is a valid HTTP header field byte (RFC 9110 §5.1).
func ValidHeaderFieldByte(c byte) bool {
	return c < 128 && ValidHeaderFieldByteTable[c] == 1
}

// InitHeaderKV initializes and normalizes header key and value buffers from strings.
func InitHeaderKV(bufK, bufV []byte, key, value string, disableNormalizing bool) ([]byte, []byte) {
	bufK = GetHeaderKeyBytes(bufK, key, disableNormalizing)
	bufV = InitHeaderValueString(bufV, value)
	return bufK, bufV
}

// InitHeaderValueString initializes and sanitizes a header value buffer from value string.
func InitHeaderValueString(bufV []byte, value string) []byte {
	return InitHeaderValueBytes(bufV, bytesconv.S2B(value))
}

// InitHeaderValueBytes copies value into bufV and sanitizes CR and LF characters.
func InitHeaderValueBytes(bufV, value []byte) []byte {
	bufV = append(bufV[:0], value...)
	bufV = RemoveNewLines(bufV)
	return bufV
}

// GetHeaderKeyBytes copies and normalizes key string into bufK.
func GetHeaderKeyBytes(bufK []byte, key string, disableNormalizing bool) []byte {
	bufK = append(bufK[:0], key...)
	NormalizeHeaderKey(bufK, disableNormalizing)
	return bufK
}

// NormalizeHeaderKey canonicalizes header key bytes in b unless disableNormalizing is true.
func NormalizeHeaderKey(b []byte, disableNormalizing bool) {
	b = RemoveNewLines(b)

	if disableNormalizing {
		return
	}

	n := len(b)
	if n == 0 {
		return
	}

	for _, c := range b {
		if !ValidHeaderFieldByte(c) {
			return
		}
	}

	NormalizeHeaderKeyValidated(b, false)
}

// NormalizeHeaderKeyValidated applies casing normalization to valid header key bytes in b.
func NormalizeHeaderKeyValidated(b []byte, disableNormalizing bool) {
	if disableNormalizing {
		return
	}

	n := len(b)
	if n == 0 {
		return
	}

	upper := true
	for i, c := range b {
		if upper {
			c = ToUpperTable[c]
		} else {
			c = ToLowerTable[c]
		}

		upper = c == '-'
		b[i] = c
	}
}

// RemoveNewLines will replace `\r` and `\n` with an empty space.
func RemoveNewLines(raw []byte) []byte {
	foundR := bytes.IndexByte(raw, rChar)
	foundN := bytes.IndexByte(raw, nChar)
	start := 0

	switch {
	case foundN != -1:
		if foundR > foundN {
			start = foundN
		} else if foundR != -1 {
			start = foundR
		}

	case foundR != -1:
		start = foundR
	default:
		return raw
	}

	for i := start; i < len(raw); i++ {
		switch raw[i] {
		case rChar, nChar:
			raw[i] = ' '
		default:
			continue
		}
	}

	return raw
}
