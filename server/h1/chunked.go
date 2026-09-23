// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h1

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"

	"github.com/lemon4ksan/foundation/silicon/bytesconv"
)

var (
	// ErrInvalidChunkSize is returned when a chunk-size line cannot be parsed as valid hexadecimal (RFC 9112 §7.1).
	ErrInvalidChunkSize = errors.New("h1: invalid chunk size in chunked encoding")

	// ErrChunkBoundaryError is returned when chunk data is not followed by CRLF (RFC 9112 §7.1).
	ErrChunkBoundaryError = errors.New("h1: missing CRLF at chunk boundary")

	errEmptyHexNum = errors.New("h1: empty hex number")
)

// ParseHexUint parses a hex-encoded uint from src for chunk-size decoding (RFC 9112 §7.1).
// Returns the parsed integer, number of bytes consumed, or an error.
func ParseHexUint(src []byte) (int, int, error) {
	if len(src) == 0 {
		return 0, 0, errEmptyHexNum
	}

	var (
		val int
		i   int
	)
	for i = 0; i < len(src); i++ {
		c := src[i]

		var d int
		switch {
		case c >= '0' && c <= '9':
			d = int(c - '0')
		case c >= 'a' && c <= 'f':
			d = int(c - 'a' + 10)
		case c >= 'A' && c <= 'F':
			d = int(c - 'A' + 10)
		default:
			if i == 0 {
				return 0, 0, errEmptyHexNum
			}

			return val, i, nil
		}

		val = (val << 4) | d
	}

	return val, i, nil
}

// FormatHexUint writes the hex representation of val into buf for chunk-size encoding (RFC 9112 §7.1).
// Operates with zero heap allocations.
func FormatHexUint(buf *[16]byte, val int) int {
	if val == 0 {
		buf[0] = '0'
		return 1
	}

	idx := 15
	for val > 0 {
		nib := byte(val & 0x0F)
		if nib < 10 {
			buf[idx] = '0' + nib
		} else {
			buf[idx] = 'a' + (nib - 10)
		}

		idx--
		val >>= 4
	}

	count := 15 - idx
	copy(buf[:count], buf[idx+1:16])

	return count
}

// ChunkedReader decodes an HTTP/1.1 chunked transfer-encoded byte stream (RFC 9112 §7.1).
// Not safe for concurrent use across multiple goroutines.
type ChunkedReader struct {
	r         *bufio.Reader
	remaining int64
	done      bool
}

// NewChunkedReader creates a ChunkedReader wrapping the provided bufio.Reader (RFC 9112 §7.1).
func NewChunkedReader(r *bufio.Reader) *ChunkedReader {
	return &ChunkedReader{r: r}
}

// Read reads decoded data from the chunked stream into p (RFC 9112 §7.1).
func (cr *ChunkedReader) Read(p []byte) (n int, err error) {
	if cr.done {
		return 0, io.EOF
	}

	if cr.remaining == 0 {
		// Read next chunk header
		line, err := cr.r.ReadBytes('\n')
		if err != nil {
			return 0, err
		}

		line = bytes.TrimRight(line, "\r\n")
		// Strip chunk extensions if present (e.g., "1a;ext=val")
		if idx := bytes.IndexByte(line, ';'); idx != -1 {
			line = line[:idx]
		}

		line = bytes.TrimSpace(line)

		if len(line) == 0 {
			return 0, ErrInvalidChunkSize
		}

		chunkSize, _, err := ParseHexUint(line)
		if err != nil || chunkSize < 0 {
			return 0, fmt.Errorf("%w: %w", ErrInvalidChunkSize, err)
		}

		if chunkSize == 0 {
			cr.done = true
			// Drain trailing CRLF or trailer headers
			_, _ = cr.r.ReadBytes('\n')

			return 0, io.EOF
		}

		cr.remaining = int64(chunkSize)
	}

	toRead := min(int64(len(p)), cr.remaining)
	n, err = cr.r.Read(p[:toRead])
	cr.remaining -= int64(n)

	if cr.remaining == 0 && err == nil {
		// Expect CRLF after chunk data
		b1, err1 := cr.r.ReadByte()
		b2, err2 := cr.r.ReadByte()

		if err1 != nil || err2 != nil || b1 != '\r' || b2 != '\n' {
			return n, ErrChunkBoundaryError
		}
	}

	return n, err
}

// ReadAllChunked drains all chunked content into a preallocated byte slice up to maxBodySize (RFC 9112 §7.1, RFC 9110 §8.6).
func ReadAllChunked(r *bufio.Reader, maxBodySize int64) ([]byte, error) {
	cr := NewChunkedReader(r)

	buf := bytesconv.AcquireByteBuffer()
	defer bytesconv.ReleaseByteBuffer(buf)

	lr := io.LimitReader(cr, maxBodySize+1)

	_, err := buf.ReadFrom(lr)
	if err != nil {
		return nil, err
	}

	if int64(len(buf.B)) > maxBodySize {
		return nil, ErrBodyTooLarge
	}

	res := make([]byte, len(buf.B))
	copy(res, buf.B)

	return res, nil
}

// ChunkedWriter writes data using HTTP/1.1 chunked transfer coding (RFC 9112 §7.1).
// Not safe for concurrent use across multiple goroutines.
type ChunkedWriter struct {
	w *bytesconv.ByteBuffer
}

// NewChunkedWriter creates a new ChunkedWriter wrapping w (RFC 9112 §7.1).
func NewChunkedWriter(w *bytesconv.ByteBuffer) *ChunkedWriter {
	return &ChunkedWriter{w: w}
}

// Write frames p as a chunk: "<hex-length>\r\n<data>\r\n" and flushes (RFC 9112 §7.1).
func (cw *ChunkedWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}

	var hexBuf [16]byte

	n := FormatHexUint(&hexBuf, len(p))

	_, _ = cw.w.Write(hexBuf[:n])
	_, _ = cw.w.Write(hdrCRLF)
	_, _ = cw.w.Write(p)
	_, _ = cw.w.Write(hdrCRLF)

	return len(p), cw.w.Flush()
}

// Close writes the terminal chunk "0\r\n\r\n" and flushes (RFC 9112 §7.1).
func (cw *ChunkedWriter) Close() error {
	_, _ = cw.w.WriteString("0\r\n\r\n")
	return cw.w.Flush()
}
