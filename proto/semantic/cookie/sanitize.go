// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cookie

import (
	"bytes"

	"github.com/lemon4ksan/foundation/silicon/bytesconv"
)

const (
	rChar = '\r'
	nChar = '\n'
)

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

// RemoveNewLines replaces '\r' and '\n' with spaces.
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
		}
	}

	return raw
}
