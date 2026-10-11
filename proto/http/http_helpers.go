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
var validTokenMask = [2]uint64{
	0x03ff6cfa00000000,
	0x57ffffffc7fffffe,
}

// CaseInsensitiveCompare reports whether byte slices a and b are equal under ASCII case-insensitivity.
func CaseInsensitiveCompare(a, b []byte) bool {
	return bytesconv.EqualFoldASCII(bytesconv.B2S(a), bytesconv.B2S(b))
}

// ValidHeaderFieldByte reports whether c is a valid HTTP header field byte (RFC 9110 §5.1, §5.6.2).
func ValidHeaderFieldByte(c byte) bool {
	return c < 128 && ((validTokenMask[c/64] & (uint64(1) << (c % 64))) != 0)
}

// isValidHeaderKey reports whether b is a valid non-empty HTTP header token (RFC 9110 §5.6.2).
func isValidHeaderKey(b []byte) bool {
	if len(b) == 0 {
		return false
	}

	for _, c := range b {
		if !ValidHeaderFieldByte(c) {
			return false
		}
	}

	return true
}

// validHeaderValueByte reports whether c is a valid HTTP header field value byte (RFC 9110 §5.5).
func validHeaderValueByte(c byte) bool {
	return c == '\t' || (c >= ' ' && c != 0x7f)
}

// InitHeaderKV initializes and normalizes header key and value buffers from strings.
func InitHeaderKV[SK, SV ~string | ~[]byte](bufK, bufV []byte, key SK, value SV, disableNormalizing bool) ([]byte, []byte) {
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
