// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cookie

import (
	"bytes"
)

const (
	rChar = '\r'
	nChar = '\n'
)

// InitHeaderValue copies value ([string] or [[]byte]) into bufV and sanitizes CR and LF characters.
func InitHeaderValue[S ~string | ~[]byte](bufV []byte, value S) []byte {
	bufV = append(bufV[:0], value...)
	return RemoveNewLines(bufV)
}

// InitHeaderValueString initializes and sanitizes a header value buffer from value string.
func InitHeaderValueString(bufV []byte, value string) []byte {
	return InitHeaderValue(bufV, value)
}

// InitHeaderValueBytes copies value into bufV and sanitizes CR and LF characters.
func InitHeaderValueBytes(bufV, value []byte) []byte {
	return InitHeaderValue(bufV, value)
}

// RemoveNewLines replaces '\r' and '\n' with spaces.
func RemoveNewLines(raw []byte) []byte {
	foundR := bytes.IndexByte(raw, rChar)
	foundN := bytes.IndexByte(raw, nChar)
	var start int

	switch {
	case foundR != -1 && foundN != -1:
		start = min(foundR, foundN)
	case foundR != -1:
		start = foundR
	case foundN != -1:
		start = foundN
	default:
		return raw
	}

	for i := start; i < len(raw); i++ {
		if raw[i] == rChar || raw[i] == nChar {
			raw[i] = ' '
		}
	}

	return raw
}
