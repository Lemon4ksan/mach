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

	protoh1 "github.com/lemon4ksan/mach/proto/h1"
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
// Accelerated via proto/h1 vectorized hexkit implementation.
//
// Concurrency: Thread-safe; operates strictly on the provided byte slice without allocations.
func ParseHexUint(src []byte) (int, int, error) {
	return protoh1.ParseHexUint(src)
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
// Accelerated via proto/h1 vectorized hexkit implementation.
//
// Concurrency: Thread-safe; writes strictly into caller-provided stack or heap buffer.
func FormatChunkHeader(buf *[24]byte, val int) int {
	n := protoh1.FormatHexUint((*[16]byte)(buf[:16]), val)
	buf[n] = '\r'
	buf[n+1] = '\n'

	return n + 2
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
	// Frame WriteTo output directly, skipping CopyBufPool, for bodies whose
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

	vbuf := CopyBufPool.Get()
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

	CopyBufPool.Put(vbuf)

	return err
}

func writeChunk(w *bufio.Writer, b []byte) error {
	n := len(b)
	var hexBuf [16]byte
	hexLen := protoh1.FormatHexUint(&hexBuf, n)

	if _, err := w.Write(hexBuf[:hexLen]); err != nil {
		return err
	}

	if _, err := w.Write(StrCRLF); err != nil {
		return err
	}

	if _, err := w.Write(b); err != nil {
		return err
	}

	if n > 0 {
		if _, err := w.Write(StrCRLF); err != nil {
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

		dst, err = appendBodyFixedSize(r, dst, chunkSize+len(StrCRLF))
		if err != nil {
			return dst, err
		}

		if !bytes.Equal(dst[len(dst)-len(StrCRLF):], StrCRLF) {
			return dst, ErrBrokenChunk{error: errors.New("cannot find crlf at the end of chunk")}
		}

		dst = dst[:len(dst)-len(StrCRLF)]
	}
}

// parseChunkSize decodes the chunk size hex value and skips optional chunk extensions (RFC 9112 Section 7.1.1).
func parseChunkSize(r *bufio.Reader) (int, error) {
	line, err := r.ReadSlice('\n')
	if err != nil {
		if errors.Is(err, io.EOF) {
			return -1, io.EOF
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			fullLine, errFull := r.ReadBytes('\n')
			if errFull != nil {
				return -1, ErrBrokenChunk{error: fmt.Errorf("cannot read chunk size line: %w", errFull)}
			}
			line = fullLine
		} else {
			return -1, ErrBrokenChunk{error: fmt.Errorf("cannot read chunk size line: %w", err)}
		}
	}

	if len(line) < 2 || line[len(line)-2] != '\r' || line[len(line)-1] != '\n' {
		return -1, ErrBrokenChunk{error: errors.New("cannot find crlf at the end of chunk size")}
	}

	line = line[:len(line)-2]

	// Strip chunk extensions (RFC 9112 §7.1.1)
	if idx := protoh1.IndexByte(line, ';'); idx != -1 {
		line = line[:idx]
	}

	// Strip trailing BWS (RFC 9112 §7.1.1)
	line = protoh1.TrimTrailingSpace(line)

	if len(line) == 0 {
		return -1, ErrBrokenChunk{error: errors.New("empty chunk size")}
	}

	chunkSize, consumed, err := protoh1.ParseHexUint(line)
	if err != nil || chunkSize < 0 || consumed != len(line) {
		return -1, ErrBrokenChunk{error: fmt.Errorf("invalid chunk size: %w", err)}
	}

	return chunkSize, nil
}

func readCrLf(r *bufio.Reader) error {
	b1, err1 := r.ReadByte()
	b2, err2 := r.ReadByte()
	if err1 != nil || err2 != nil || b1 != '\r' || b2 != '\n' {
		return ErrBrokenChunk{error: errors.New("cannot find crlf at the end of chunk")}
	}

	return nil
}
