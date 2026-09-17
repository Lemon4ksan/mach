// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package http

import (
	"bufio"

	"github.com/lemon4ksan/mach/proto/bytesutil"
)

// ParseHexUint parses a hex-encoded uint from src.
// Returns the parsed integer, number of bytes consumed, and error if malformed.
func ParseHexUint(src []byte) (int, int, error) {
	return parseHexUintFallback(src)
}

// ReadBodyChunked decodes an HTTP/1.1 chunked stream from r into dst (RFC 9112 §7.1).
func ReadBodyChunked(r *bufio.Reader, maxBodySize int, dst []byte) ([]byte, error) {
	return readBodyChunked(r, maxBodySize, dst)
}

// FormatChunkHeader writes the hex chunk header with \r\n trailer into buf.
// Returns the number of bytes written.
func FormatChunkHeader(buf *[24]byte, val int) int {
	n := bytesutil.FormatHexUint((*[16]byte)(buf[:16]), val)
	buf[n] = '\r'
	buf[n+1] = '\n'

	return n + 2
}

func parseHexUintFallback(src []byte) (int, int, error) {
	if len(src) == 0 {
		return 0, 0, bytesutil.ErrEmptyHexNum
	}

	var n, i int
	for i = 0; i < len(src); i++ {
		c := src[i]

		k := int(bytesutil.Hex2intTable[c])
		if k == 16 {
			if i == 0 {
				return 0, 0, bytesutil.ErrEmptyHexNum
			}

			return n, i, nil
		}

		if i >= 16 {
			return n, i, bytesutil.ErrTooLargeHexNum
		}

		n = (n << 4) | k
	}

	if i == 0 {
		return 0, 0, bytesutil.ErrEmptyHexNum
	}

	return n, i, nil
}
