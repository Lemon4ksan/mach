// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package http

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"

	"github.com/lemon4ksan/mach/proto/http/zerocopy"
)

var errChunkedStream = errors.New("chunked stream")

// ErrBrokenChunk is returned when a chunked transfer coding stream contains malformed
// framing, missing CRLF delimiters, invalid characters, or an unreadable chunk size
// per RFC 9112 Section 7.1.
type ErrBrokenChunk struct{ error }

// ParseHexUint parses a hex-encoded uint from src per RFC 9112 Section 7.1.1.
//
// It returns the parsed integer value, the number of bytes consumed from src,
// and an error if the hex sequence is malformed, signed, or overflows 15 hex digits.
//
// Concurrency: Thread-safe; operates strictly on the provided byte slice without allocations.
func ParseHexUint(src []byte) (int, int, error) {
	return parseHexUintFallback(src)
}

// ReadBodyChunked decodes an HTTP/1.1 chunked transfer coding stream from r into dst
// in accordance with RFC 9112 Section 7.1.
//
// It reads chunk-size headers, ignores valid chunk extensions (RFC 9112 Section 7.1.1),
// and appends chunk payload data to dst until the terminating 0-size chunk is encountered.
// If maxBodySize is greater than zero and the decoded content exceeds maxBodySize,
// ErrBodyTooLarge is returned.
//
// Concurrency: Not thread-safe; caller must own r and dst exclusively during execution.
func ReadBodyChunked(r *bufio.Reader, maxBodySize int, dst []byte) ([]byte, error) {
	return readBodyChunked(r, maxBodySize, dst)
}

// FormatChunkHeader formats a hex-encoded chunk size header followed by CRLF (\r\n)
// into buf per RFC 9112 Section 7.1.
//
// buf must have a capacity of at least 24 bytes. Returns the total number of bytes written.
//
// Concurrency: Thread-safe; writes strictly into caller-provided stack or heap buffer.
func FormatChunkHeader(buf *[24]byte, val int) int {
	n := zerocopy.FormatHexUint((*[16]byte)(buf[:16]), val)
	buf[n] = '\r'
	buf[n+1] = '\n'

	return n + 2
}

func parseHexUintFallback(src []byte) (int, int, error) {
	if len(src) == 0 {
		return 0, 0, zerocopy.ErrEmptyHexNum
	}

	var n, i int
	for i = 0; i < len(src); i++ {
		c := src[i]

		k := int(zerocopy.Hex2intTable[c])
		if k == 16 {
			if i == 0 {
				return 0, 0, zerocopy.ErrEmptyHexNum
			}

			return n, i, nil
		}

		if i >= 15 {
			return n, i, zerocopy.ErrTooLargeHexNum
		}

		n = (n << 4) | k
	}

	if i == 0 {
		return 0, 0, zerocopy.ErrEmptyHexNum
	}

	return n, i, nil
}

type chunkedBodyWriter struct {
	w   *bufio.Writer
	err error
}

func (cw *chunkedBodyWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}

	if err := writeChunk(cw.w, p); err != nil {
		cw.err = err
		return 0, err
	}

	return len(p), nil
}

func writeBodyChunked(w *bufio.Writer, r io.Reader) error {
	var wt io.WriterTo
	// Frame WriteTo output directly, skipping zerocopy.CopyBufPool, for bodies whose
	// WriteTo is known to match reading.
	switch v := r.(type) {
	case *bytes.Reader:
		wt = v
	case *bytes.Buffer:
		wt = v
	default:
		if bwt, ok := r.(BodyWriterTo); ok && bwt.SupportsBodyWriteTo() {
			wt = bwt
		}
	}

	if wt != nil {
		cw := chunkedBodyWriter{w: w}
		if _, err := wt.WriteTo(&cw); err != nil {
			return err
		}

		if cw.err != nil {
			return cw.err
		}

		return writeChunk(w, nil)
	}

	vbuf := zerocopy.CopyBufPool.Get()
	buf := vbuf.([]byte)

	var (
		err error
		n   int
	)
	for {
		n, err = r.Read(buf)
		if n == 0 {
			if err == nil {
				continue
			}

			if errors.Is(err, io.EOF) {
				if err = writeChunk(w, buf[:0]); err != nil {
					break
				}

				err = nil
			}

			break
		}

		if err = writeChunk(w, buf[:n]); err != nil {
			break
		}
	}

	zerocopy.CopyBufPool.Put(vbuf)

	return err
}

func writeChunk(w *bufio.Writer, b []byte) error {
	n := len(b)
	if err := zerocopy.WriteHexInt(w, n); err != nil {
		return err
	}

	if _, err := w.Write(zerocopy.StrCRLF); err != nil {
		return err
	}

	if _, err := w.Write(b); err != nil {
		return err
	}

	if n > 0 {
		if _, err := w.Write(zerocopy.StrCRLF); err != nil {
			return err
		}
	}

	return w.Flush()
}

// readBodyChunked parses a chunked transfer coding body (RFC 9112 Section 7.1).
// It repeatedly reads chunk sizes and data until the terminating 0-size chunk is found.
func readBodyChunked(r *bufio.Reader, maxBodySize int, dst []byte) ([]byte, error) {
	if len(dst) > 0 {
		panic("BUG: expected zero-length buffer")
	}

	for {
		chunkSize, err := parseChunkSize(r)
		if err != nil {
			return dst, err
		}

		if chunkSize == 0 {
			return dst, err
		}

		if maxBodySize > 0 && len(dst)+chunkSize > maxBodySize {
			return dst, ErrBodyTooLarge
		}

		dst, err = appendBodyFixedSize(r, dst, chunkSize+len(zerocopy.StrCRLF))
		if err != nil {
			return dst, err
		}

		if !bytes.Equal(dst[len(dst)-len(zerocopy.StrCRLF):], zerocopy.StrCRLF) {
			return dst, ErrBrokenChunk{error: errors.New("cannot find crlf at the end of chunk")}
		}

		dst = dst[:len(dst)-len(zerocopy.StrCRLF)]
	}
}

// parseChunkSize decodes the chunk size hex value and skips optional chunk extensions (RFC 9112 Section 7.1.1).
func parseChunkSize(r *bufio.Reader) (int, error) {
	n, err := zerocopy.ReadHexInt(r)
	if err != nil {
		return -1, err
	}

	inExt := false
	for {
		c, err := r.ReadByte()
		if err != nil {
			return -1, ErrBrokenChunk{error: fmt.Errorf("cannot read '\\r' char at the end of chunk size: %w", err)}
		}

		if c == '\r' {
			if err := r.UnreadByte(); err != nil {
				return -1, ErrBrokenChunk{
					error: fmt.Errorf("cannot unread '\\r' char at the end of chunk size: %w", err),
				}
			}

			break
		}

		if c == '\n' {
			return -1, ErrBrokenChunk{error: errors.New("invalid character '\\n' after chunk size")}
		}

		if inExt {
			continue
		}

		switch c {
		case ' ', '\t':
			continue
		case ';':
			inExt = true
			continue
		default:
			return -1, ErrBrokenChunk{error: fmt.Errorf("invalid character %q after chunk size", c)}
		}
	}

	err = readCrLf(r)
	if err != nil {
		return -1, err
	}

	return n, nil
}

func readCrLf(r *bufio.Reader) error {
	for _, exp := range []byte{'\r', '\n'} {
		c, err := r.ReadByte()
		if err != nil {
			return ErrBrokenChunk{error: fmt.Errorf("cannot read %q char at the end of chunk size: %w", exp, err)}
		}

		if c != exp {
			return ErrBrokenChunk{error: fmt.Errorf("unexpected char %q at the end of chunk size: expected %q", c, exp)}
		}
	}

	return nil
}
