// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h1

import (
	"errors"

	"github.com/lemon4ksan/foundation/silicon/hexkit"
	"github.com/lemon4ksan/foundation/silicon/simd"
)

var (
	// ErrEmptyHexNum is returned when parsing an empty hex string.
	ErrEmptyHexNum = hexkit.ErrEmptyHexNum

	// ErrInvalidChunkSize is returned when a chunk-size line cannot be parsed as valid hexadecimal (RFC 9112 §7.1).
	ErrInvalidChunkSize = errors.New("h1: invalid chunk size in chunked encoding")

	// ErrChunkBoundaryError is returned when chunk data is not followed by CRLF (RFC 9112 §7.1).
	ErrChunkBoundaryError = errors.New("h1: missing CRLF at chunk boundary")

	// ErrForbiddenTrailer is returned when a trailer field contains a forbidden framing or routing field (RFC 9112 §7.1.2).
	ErrForbiddenTrailer = errors.New("h1: forbidden trailer field in chunked encoding (RFC 9112 §7.1.2)")

	// ErrInvalidTrailer is returned when the trailer section fails RFC 9112 syntax rules.
	ErrInvalidTrailer = errors.New("h1: invalid trailer section in chunked encoding (RFC 9112 §7.1.2)")
)

// TrimSpace strips both leading and trailing ASCII spaces (' ') and tabs ('\t') from b
// using 64-bit SWAR vector processing.
//
//go:inline
func TrimSpace(b []byte) []byte {
	return simd.TrimSpaceSWAR(b)
}

// TrimLeadingSpace strips leading ASCII spaces (' ') and tabs ('\t') from b
// using 64-bit SWAR vector processing.
//
//go:inline
func TrimLeadingSpace(b []byte) []byte {
	return simd.TrimLeadingSpaceSWAR(b)
}

// TrimTrailingSpace strips trailing ASCII spaces (' ') and tabs ('\t') from b
// using 64-bit SWAR vector processing.
//
//go:inline
func TrimTrailingSpace(b []byte) []byte {
	return simd.TrimTrailingSpaceSWAR(b)
}

// IndexColon locates the first occurrence of ':' in b using SWAR vector routines.
// Returns -1 if not found.
//
//go:inline
func IndexColon(b []byte) int {
	return simd.IndexColonSWAR(b)
}

// IndexByte locates the first occurrence of target in b using SIMD/SWAR vector routines.
// Returns -1 if target is not found.
//
//go:inline
func IndexByte(b []byte, target byte) int {
	return simd.IndexByteVector(b, target)
}

// IndexTwoBytes locates the first occurrence of either b1 or b2 in b using SIMD/SWAR vector routines.
// Returns -1 if neither byte is found.
//
//go:inline
func IndexTwoBytes(b []byte, b1, b2 byte) int {
	return simd.IndexTwoBytesVector(b, b1, b2)
}

// IndexCRLF searches for the first occurrence of "\r\n" in b.
//
//go:inline
func IndexCRLF(b []byte) int {
	return simd.IndexCRLF(b)
}

// IndexCRLFCRLF searches for the position immediately after "\r\n\r\n" in b.
//
//go:inline
func IndexCRLFCRLF(b []byte) int {
	return simd.IndexCRLFCRLF(b)
}

// FormatHexUint writes the lowercase hexadecimal representation of val into buf
// without heap allocations, returning the number of bytes written (RFC 9112 §7.1).
// FormatHexUint panics if val is negative.
//
//go:inline
func FormatHexUint(buf *[16]byte, val int) int {
	return hexkit.FormatHexUint(buf, val)
}

// ParseHexUint parses a hex-encoded uint from src for chunk-size decoding (RFC 9112 §7.1).
// Prevents parsing errors due to integer overflow.
// Returns the parsed integer, number of bytes consumed, or an error.
//
//go:inline
func ParseHexUint(src []byte) (int, int, error) {
	val, n, err := hexkit.ParseHexUint(src)
	if err != nil {
		if errors.Is(err, hexkit.ErrHexOverflow) {
			return 0, 0, ErrInvalidChunkSize
		}

		return 0, 0, err
	}

	return val, n, nil
}

// DecodeArgAppend decodes URL-encoded argument bytes from src into dst, replacing '+' with space (RFC 3986 §2.1).
// Accelerated with SIMD/SWAR 2-byte vector scanning for '%' and '+' and foundation hexkit.
func DecodeArgAppend(dst, src []byte) []byte {
	n := len(src)
	if n == 0 {
		return dst
	}

	idx := simd.IndexTwoBytesVector(src, '%', '+')
	if idx < 0 {
		return append(dst, src...)
	}

	dst = append(dst, src[:idx]...)
	i := idx

	for i < n {
		rem := src[i:]

		next := simd.IndexTwoBytesVector(rem, '%', '+')
		if next < 0 {
			return append(dst, rem...)
		}

		if next > 0 {
			dst = append(dst, rem[:next]...)
			i += next
		}

		c := src[i]
		if c == '+' {
			dst = append(dst, ' ')
			i++
			continue
		}

		// Percent-decoding '%XX'
		if i+2 >= n {
			// Incomplete escape sequence at the end of input
			return append(dst, src[i:]...)
		}

		h1, ok1 := hexkit.FromHexChar(src[i+1])

		h2, ok2 := hexkit.FromHexChar(src[i+2])
		if ok1 && ok2 {
			dst = append(dst, (h1<<4)|h2)
			i += 3
		} else {
			dst = append(dst, '%')
			i++
		}
	}

	return dst
}

// DecodeArgAppendNoPlus decodes percent-encoded bytes from src into dst without translating '+' into space (RFC 3986 §2.1).
// Accelerated with SIMD/SWAR vector scanning for '%' and foundation hexkit.
func DecodeArgAppendNoPlus(dst, src []byte) []byte {
	n := len(src)
	if n == 0 {
		return dst
	}

	idx := simd.IndexByteVector(src, '%')
	if idx < 0 {
		return append(dst, src...)
	}

	dst = append(dst, src[:idx]...)
	i := idx

	for i < n {
		rem := src[i:]

		next := simd.IndexByteVector(rem, '%')
		if next < 0 {
			return append(dst, rem...)
		}

		if next > 0 {
			dst = append(dst, rem[:next]...)
			i += next
		}

		// Percent-decoding '%XX'
		if i+2 >= n {
			return append(dst, src[i:]...)
		}

		h1, ok1 := hexkit.FromHexChar(src[i+1])

		h2, ok2 := hexkit.FromHexChar(src[i+2])
		if ok1 && ok2 {
			dst = append(dst, (h1<<4)|h2)
			i += 3
		} else {
			dst = append(dst, '%')
			i++
		}
	}

	return dst
}
