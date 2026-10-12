// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package http

import (
	"github.com/lemon4ksan/foundation/silicon/bytesconv"

	"github.com/lemon4ksan/mach/proto/headkit"
	"github.com/lemon4ksan/mach/proto/semantic/cookie"
)

const (
	rChar = '\r'
	nChar = '\n'
)

// validTokenMask defines the 128-bit bitset for RFC 9110 §5.6.2 token characters.
var validTokenTable = [256]byte{
	'!': 1, '#': 1, '$': 1, '%': 1, '&': 1, '\'': 1, '*': 1, '+': 1,
	'-': 1, '.': 1, '^': 1, '_': 1, '`': 1, '|': 1, '~': 1,
	'0': 1, '1': 1, '2': 1, '3': 1, '4': 1, '5': 1, '6': 1, '7': 1, '8': 1, '9': 1,
	'a': 1, 'b': 1, 'c': 1, 'd': 1, 'e': 1, 'f': 1, 'g': 1, 'h': 1, 'i': 1, 'j': 1,
	'k': 1, 'l': 1, 'm': 1, 'n': 1, 'o': 1, 'p': 1, 'q': 1, 'r': 1, 's': 1, 't': 1,
	'u': 1, 'v': 1, 'w': 1, 'x': 1, 'y': 1, 'z': 1,
	'A': 1, 'B': 1, 'C': 1, 'D': 1, 'E': 1, 'F': 1, 'G': 1, 'H': 1, 'I': 1, 'J': 1,
	'K': 1, 'L': 1, 'M': 1, 'N': 1, 'O': 1, 'P': 1, 'Q': 1, 'R': 1, 'S': 1, 'T': 1,
	'U': 1, 'V': 1, 'W': 1, 'X': 1, 'Y': 1, 'Z': 1,
}

// CaseInsensitiveCompare reports whether byte slices a and b are equal under ASCII case-insensitivity.
func CaseInsensitiveCompare(a, b []byte) bool {
	return bytesconv.EqualFoldASCII(bytesconv.B2S(a), bytesconv.B2S(b))
}

// ValidHeaderFieldByte reports whether c is a valid HTTP header field byte (RFC 9110 §5.1, §5.6.2).
func ValidHeaderFieldByte(c byte) bool {
	return validTokenTable[c] != 0
}

// isValidHeaderKey reports whether b is a valid non-empty HTTP header token (RFC 9110 §5.6.2).
func isValidHeaderKey(b []byte) bool {
	n := len(b)
	if n == 0 {
		return false
	}

	i := 0
	for ; i+4 <= n; i += 4 {
		if validTokenTable[b[i]] == 0 ||
			validTokenTable[b[i+1]] == 0 ||
			validTokenTable[b[i+2]] == 0 ||
			validTokenTable[b[i+3]] == 0 {
			return false
		}
	}

	for i < n {
		if validTokenTable[b[i]] == 0 {
			return false
		}

		i++
	}

	return true
}

// validHeaderValueByte reports whether c is a valid HTTP header field value byte (RFC 9110 §5.5).
func validHeaderValueByte(c byte) bool {
	return c == '\t' || (c >= ' ' && c != 0x7f)
}

// InitHeaderKV initializes and normalizes header key and value buffers from strings.
func InitHeaderKV[SK, SV ~string | ~[]byte](
	bufK, bufV []byte,
	key SK,
	value SV,
	disableNormalizing bool,
) ([]byte, []byte) {
	bufK = GetHeaderKeyBytes(bufK, key, disableNormalizing)
	bufV = InitHeaderValue(bufV, value)
	return bufK, bufV
}

// InitHeaderValue copies value ([string] or [[]byte]) into bufV and sanitizes CR and LF characters.
func InitHeaderValue[S ~string | ~[]byte](bufV []byte, value S) []byte {
	return cookie.InitHeaderValue(bufV, value)
}

// InitHeaderValueString initializes and sanitizes a header value buffer from value string.
func InitHeaderValueString(bufV []byte, value string) []byte {
	return cookie.InitHeaderValueString(bufV, value)
}

// InitHeaderValueBytes copies value into bufV and sanitizes CR and LF characters.
func InitHeaderValueBytes(bufV, value []byte) []byte {
	return cookie.InitHeaderValueBytes(bufV, value)
}

// GetHeaderKeyBytes copies and normalizes key string into bufK.
func GetHeaderKeyBytes[S ~string | ~[]byte](bufK []byte, key S, disableNormalizing bool) []byte {
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

	headkit.CanonicalKeyBytes(b)
}

// RemoveNewLines will replace `\r` and `\n` with an empty space.
func RemoveNewLines(raw []byte) []byte {
	return cookie.RemoveNewLines(raw)
}
