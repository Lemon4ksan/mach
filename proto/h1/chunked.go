// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h1

import (
	"bufio"
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

const maxChunkLineLen = 16384 // RFC 9112 §7.1 and Chromium kMaxLineBufLen

// Reset resets the ChunkedReader with reader r, clearing internal state (RFC 9112 §7.1).
func (cr *ChunkedReader) Reset(r *bufio.Reader) {
	cr.r = r
	cr.remaining = 0
	cr.done = false
}

func (cr *ChunkedReader) readChunkLine() ([]byte, error) {
	line, err := cr.r.ReadSlice('\n')
	if err == nil {
		if len(line) > maxChunkLineLen {
			return nil, ErrInvalidChunkSize
		}
		return line, nil
	}

	if errors.Is(err, bufio.ErrBufferFull) {
		total := len(line)
		for {
			part, pErr := cr.r.ReadSlice('\n')
			total += len(part)
			if total > maxChunkLineLen {
				return nil, ErrInvalidChunkSize
			}
			if pErr == nil {
				return nil, ErrInvalidChunkSize
			}
			if !errors.Is(pErr, bufio.ErrBufferFull) {
				return nil, pErr
			}
		}
	}

	return nil, err
}

// Read reads decoded data from the chunked stream into p (RFC 9112 §7.1).
func (cr *ChunkedReader) Read(p []byte) (n int, err error) {
	if cr.done {
		return 0, io.EOF
	}

	if cr.remaining == 0 {
		line, err := cr.readChunkLine()
		if err != nil {
			return 0, err
		}

		// RFC 9112 §7.1: chunk = chunk-size [ chunk-ext ] CRLF
		if len(line) < 2 || line[len(line)-2] != '\r' || line[len(line)-1] != '\n' {
			return 0, ErrChunkBoundaryError
		}

		line = line[:len(line)-2]

		// RFC 9112 §7.1.1: Parse and validate chunk extensions (e.g., "1a;ext=val" or "1a ;ext=val")
		if idx := IndexByte(line, ';'); idx != -1 {
			extPart := line[idx:]
			line = TrimTrailingSpace(line[:idx])
			if err := validateChunkExtensions(extPart); err != nil {
				return 0, err
			}
		}

		// RFC 9112 §7.1: chunk-size = 1*HEXDIG. Leading and trailing whitespace without ';' is prohibited.
		if len(line) == 0 {
			return 0, ErrInvalidChunkSize
		}

		chunkSize, consumed, err := ParseHexUint(line)
		if err != nil || chunkSize < 0 || consumed != len(line) {
			if err != nil {
				return 0, fmt.Errorf("%w: %w", ErrInvalidChunkSize, err)
			}
			return 0, ErrInvalidChunkSize
		}

		if chunkSize == 0 {
			cr.done = true
			// RFC 9112 §7.1.2: Drain trailer section *( field-line CRLF ) terminated by empty CRLF line
			trailerCount := 0
			for {
				tLine, tErr := cr.r.ReadSlice('\n')
				if tErr != nil {
					if errors.Is(tErr, io.EOF) {
						return 0, io.ErrUnexpectedEOF
					}

					return 0, tErr
				}

				if len(tLine) < 2 || tLine[len(tLine)-2] != '\r' || tLine[len(tLine)-1] != '\n' {
					return 0, ErrChunkBoundaryError
				}

				trimmed := tLine[:len(tLine)-2]
				if len(trimmed) == 0 {
					break
				}

				trailerCount++
				if trailerCount > 64 {
					return 0, ErrInvalidTrailer
				}

				// RFC 9112 §5.2: Obsolete line folding prohibited in trailers
				if trimmed[0] == ' ' || trimmed[0] == '\t' {
					return 0, ErrInvalidTrailer
				}

				colonIdx := IndexColon(trimmed)
				if colonIdx <= 0 {
					return 0, ErrInvalidTrailer
				}

				// RFC 9112 §5.1: Whitespace before colon prohibited
				if trimmed[colonIdx-1] == ' ' || trimmed[colonIdx-1] == '\t' {
					return 0, ErrInvalidTrailer
				}

				fieldName := trimmed[:colonIdx]
				if !isValidHeaderKey(fieldName) {
					return 0, ErrInvalidTrailer
				}

				// RFC 9112 §7.1.2: Reject forbidden trailer fields (Transfer-Encoding, Content-Length, Host, Trailer)
				if isForbiddenTrailer(fieldName) {
					return 0, ErrForbiddenTrailer
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

func validateChunkExtensions(ext []byte) error {
	// ext begins with ';'
	for len(ext) > 0 {
		if ext[0] != ';' {
			return ErrInvalidChunkSize
		}
		ext = ext[1:]

		// Skip BWS
		for len(ext) > 0 && (ext[0] == ' ' || ext[0] == '\t') {
			ext = ext[1:]
		}

		if len(ext) == 0 {
			return ErrInvalidChunkSize
		}

		// Parse chunk-ext-name = token
		nameLen := 0
		for nameLen < len(ext) && ext[nameLen] != ';' && ext[nameLen] != '=' && ext[nameLen] != ' ' && ext[nameLen] != '\t' {
			c := ext[nameLen]
			if c >= 128 || validTokenTable[c] == 0 {
				return ErrInvalidChunkSize
			}
			nameLen++
		}
		if nameLen == 0 {
			return ErrInvalidChunkSize
		}
		ext = ext[nameLen:]

		// Skip BWS
		for len(ext) > 0 && (ext[0] == ' ' || ext[0] == '\t') {
			ext = ext[1:]
		}

		if len(ext) == 0 {
			break
		}

		if ext[0] == ';' {
			continue
		}

		if ext[0] != '=' {
			return ErrInvalidChunkSize
		}
		ext = ext[1:]

		// Skip BWS
		for len(ext) > 0 && (ext[0] == ' ' || ext[0] == '\t') {
			ext = ext[1:]
		}

		if len(ext) == 0 {
			return ErrInvalidChunkSize
		}

		// chunk-ext-val = token / quoted-string
		if ext[0] == '"' {
			ext = ext[1:]
			closed := false
			for len(ext) > 0 {
				c := ext[0]
				if c == '"' {
					ext = ext[1:]
					closed = true
					break
				}
				if c == '\\' {
					if len(ext) < 2 {
						return ErrInvalidChunkSize
					}
					ext = ext[2:]
					continue
				}
				if c < 0x20 && c != '\t' {
					return ErrInvalidChunkSize
				}
				ext = ext[1:]
			}
			if !closed {
				return ErrInvalidChunkSize
			}
		} else {
			valLen := 0
			for valLen < len(ext) && ext[valLen] != ';' && ext[valLen] != ' ' && ext[valLen] != '\t' {
				c := ext[valLen]
				if c >= 128 || validTokenTable[c] == 0 {
					return ErrInvalidChunkSize
				}
				valLen++
			}
			if valLen == 0 {
				return ErrInvalidChunkSize
			}
			ext = ext[valLen:]
		}

		// Skip BWS
		for len(ext) > 0 && (ext[0] == ' ' || ext[0] == '\t') {
			ext = ext[1:]
		}
	}

	return nil
}

func isForbiddenTrailer(name []byte) bool {
	switch len(name) {
	case 4:
		return bytesconv.EqualFoldASCII(bytesconv.B2S(name), "Host")
	case 7:
		return bytesconv.EqualFoldASCII(bytesconv.B2S(name), "Trailer")
	case 14:
		return bytesconv.EqualFoldASCII(bytesconv.B2S(name), "Content-Length")
	case 17:
		return bytesconv.EqualFoldASCII(bytesconv.B2S(name), "Transfer-Encoding")
	}

	return false
}
