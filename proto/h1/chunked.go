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
	// ErrBodyTooLarge is returned when the decoded chunked body exceeds the limit.
	ErrBodyTooLarge = errors.New("h1: request entity too large")

	hdrCRLF = []byte("\r\n")
)

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
		line, err := cr.r.ReadBytes('\n')
		if err != nil {
			return 0, err
		}

		// RFC 9112 §7.1: chunk = chunk-size [ chunk-ext ] CRLF
		if len(line) < 2 || line[len(line)-2] != '\r' || line[len(line)-1] != '\n' {
			return 0, ErrChunkBoundaryError
		}

		line = line[:len(line)-2]

		// RFC 9112 §7.1.1: Strip chunk extensions (e.g., "1a;ext=val" or "1a ;ext=val")
		if idx := IndexByte(line, ';'); idx != -1 {
			line = line[:idx]
		}

		// Strip trailing BWS (RFC 9112 §7.1.1: chunk-size [ BWS ";" ... ])
		line = TrimTrailingSpace(line)

		// RFC 9112 §7.1: chunk-size = 1*HEXDIG. Leading whitespace is prohibited.
		if len(line) == 0 {
			return 0, ErrInvalidChunkSize
		}

		chunkSize, consumed, err := ParseHexUint(line)
		if err != nil || chunkSize < 0 || consumed != len(line) {
			return 0, fmt.Errorf("%w: %w", ErrInvalidChunkSize, err)
		}

		if chunkSize == 0 {
			cr.done = true
			// RFC 9112 §7.1.2: Drain trailer section *( field-line CRLF ) terminated by empty CRLF line
			for {
				tLine, tErr := cr.r.ReadBytes('\n')
				if tErr != nil {
					if errors.Is(tErr, io.EOF) {
						return 0, io.EOF
					}

					return 0, tErr
				}

				trimmed := bytes.TrimRight(tLine, "\r\n")
				if len(trimmed) == 0 {
					break
				}
			}

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
