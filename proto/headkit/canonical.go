// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package headkit

import (
	"net/textproto"
)

// CanonicalKey returns the canonical MIME header format of s (e.g., "accept-encoding" -> "Accept-Encoding").
// Employs a zero-allocation fast path if s is already canonicalized.
func CanonicalKey(s string) string {
	if IsCanonical(s) {
		return s
	}

	return textproto.CanonicalMIMEHeaderKey(s)
}

// IsCanonical reports whether s is already in canonical MIME header format (RFC 9110 §5.1).
func IsCanonical(s string) bool {
	n := len(s)
	if n == 0 {
		return true
	}

	_ = s[n-1]

	upper := true
	for i := range n {
		c := s[i]
		if upper {
			if 'a' <= c && c <= 'z' {
				return false
			}
		} else {
			if 'A' <= c && c <= 'Z' {
				return false
			}
		}

		upper = (c == '-')
	}

	return true
}

// CanonicalKeyBytes formats in-place or returns the canonical MIME header byte representation of b.
func CanonicalKeyBytes(b []byte) []byte {
	n := len(b)
	if n == 0 {
		return b
	}

	_ = b[n-1]

	upper := true
	for i := range n {
		c := b[i]
		if upper {
			if 'a' <= c && c <= 'z' {
				b[i] -= 'a' - 'A'
			}
		} else {
			if 'A' <= c && c <= 'Z' {
				b[i] += 'a' - 'A'
			}
		}

		upper = (c == '-')
	}

	return b
}
