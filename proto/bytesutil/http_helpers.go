// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bytesutil

import (
	"bytes"
	"encoding/binary"
)

const (
	rChar = '\r'
	nChar = '\n'
)

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

// ValidHeaderFieldByte returns true if c valid header field byte
// as defined by RFC 7230.
func ValidHeaderFieldByte(c byte) bool {
	return c < 128 && ValidHeaderFieldByteTable[c] == 1
}

func InitHeaderKV(bufK, bufV []byte, key, value string, disableNormalizing bool) ([]byte, []byte) {
	bufK = GetHeaderKeyBytes(bufK, key, disableNormalizing)
	bufV = InitHeaderValueString(bufV, value)
	return bufK, bufV
}

func InitHeaderValueString(bufV []byte, value string) []byte {
	return InitHeaderValueBytes(bufV, S2B(value))
}

func InitHeaderValueBytes(bufV, value []byte) []byte {

	bufV = append(bufV[:0], value...)
	bufV = RemoveNewLines(bufV)
	return bufV
}

func GetHeaderKeyBytes(bufK []byte, key string, disableNormalizing bool) []byte {
	bufK = append(bufK[:0], key...)
	NormalizeHeaderKey(bufK, disableNormalizing)
	return bufK
}

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
