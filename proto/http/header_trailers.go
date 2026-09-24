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
	"iter"

	"github.com/lemon4ksan/foundation/silicon/bytesconv"

	"github.com/lemon4ksan/mach/proto/headkit"
	"github.com/lemon4ksan/mach/proto/http/zerocopy"
)

// SetTrailer specifies a trailer header field to be sent after a chunked message body (RFC 9112 Section 7.1.2).
//
// In accordance with RFC 9112 Section 7.1.2 and RFC 9110 Section 6.5.1, the following headers are forbidden as trailers:
// 1. Message framing fields: Transfer-Encoding, Content-Length.
// 2. Routing fields: Host.
// 3. Request modifiers and controls: Cache-Control, Max-Forwards, TE, Expect.
// 4. Authentication fields: Authorization, Proxy-Authorization.
// 5. Response control data: Location, Set-Cookie.
// 6. Payload representation metadata: Content-Encoding, Content-Type, Content-Range, Trailer.
//
// Returns ErrBadTrailer if trailer contains forbidden fields.
func (h *header) SetTrailer(trailer string) error {
	return h.SetTrailerBytes(bytesconv.S2B(trailer))
}

// SetTrailerBytes sets Trailer header value for chunked transfer coding (RFC 9112 Section 7.1.2).
// Returns ErrBadTrailer if any forbidden trailer field is specified.
func (h *header) SetTrailerBytes(trailer []byte) error {
	h.trailer = h.trailer[:0]
	return h.AddTrailerBytes(trailer)
}

// AddTrailer appends trailer field names to the Trailer header for chunked transfer (RFC 9112 Section 7.1.2).
// Returns ErrBadTrailer if any forbidden trailer field is encountered.
func (h *header) AddTrailer(trailer string) error {
	return h.AddTrailerBytes(bytesconv.S2B(trailer))
}

// AddTrailerBytes appends trailer field names from bytes to the Trailer header for chunked transfer (RFC 9112 Section 7.1.2).
// Returns ErrBadTrailer if any forbidden trailer field is encountered.
func (h *header) AddTrailerBytes(trailer []byte) (err error) {
	for i := -1; i+1 < len(trailer); {
		trailer = trailer[i+1:]

		i = bytes.IndexByte(trailer, ',')
		if i < 0 {
			i = len(trailer)
		}

		key := trim(trailer[:i])
		if !isValidTrailerKey(key) || isBadTrailer(key) {
			err = ErrBadTrailer
			continue
		}

		h.bufK = append(h.bufK[:0], key...)
		zerocopy.NormalizeHeaderKeyValidated(h.bufK, h.disableNormalizing)

		if cap(h.trailer) > len(h.trailer) {
			h.trailer = h.trailer[:len(h.trailer)+1]
			h.trailer[len(h.trailer)-1] = append(h.trailer[len(h.trailer)-1][:0], h.bufK...)
		} else {
			key = make([]byte, len(h.bufK))
			copy(key, h.bufK)
			h.trailer = append(h.trailer, key)
		}
	}

	return err
}

// Trailers returns an iterator over registered trailer field names (RFC 9112 Section 7.1.2).
func (h *header) Trailers() iter.Seq[[]byte] {
	return func(yield func([]byte) bool) {
		for i := range h.trailer {
			if !yield(h.trailer[i]) {
				break
			}
		}
	}
}

// PeekTrailerKeys returns all registered trailer keys.
// The returned slice is valid until the header is reset or released.
func (h *header) PeekTrailerKeys() [][]byte {
	return h.trailer
}

// ReadTrailer reads trailing header fields from r following the final zero-length chunk of a chunked body (RFC 9112 Section 7.1.2).
func (h *header) ReadTrailer(r *bufio.Reader) error {
	n := 1
	for {
		err := h.tryReadTrailer(r, n)
		if err == nil {
			return nil
		}

		if !errors.Is(err, ErrNeedMore) {
			return err
		}

		n = r.Buffered() + 1
	}
}

func (h *header) tryReadTrailer(r *bufio.Reader, n int) error {
	b, err := r.Peek(n)
	if len(b) == 0 {
		if x, ok := err.(interface{ Timeout() bool }); ok && x.Timeout() {
			return ErrTimeout
		}

		if n == 1 || err == io.EOF {
			return io.EOF
		}

		if errors.Is(err, bufio.ErrBufferFull) {
			if h.SecureErrorLogMessage {
				return &ErrSmallBuffer{error: ErrReadingResponseTrailer}
			}

			return &ErrSmallBuffer{error: fmt.Errorf("error when reading response trailer: %w", ErrSmallReadBuffer)}
		}

		return fmt.Errorf("error when reading response trailer: %w", err)
	}

	b = mustPeekBuffered(r)

	headersLen, errParse := parseTrailerHeaders(b, &h.h, h.disableNormalizing)
	if errParse != nil {
		if err == io.EOF {
			return err
		}

		return headerError("response", err, errParse, b, h.SecureErrorLogMessage)
	}

	mustDiscard(r, headersLen)

	return nil
}

// TrailerHeader returns the serialized wire representation of request trailers (RFC 9112 Section 7.1.2).
func (h *RequestHeader) TrailerHeader() []byte {
	h.bufV = h.bufV[:0]
	for _, t := range h.trailer {
		value := h.peek(t)
		h.bufV = appendHeaderLine(h.bufV, t, value)
	}

	h.bufV = append(h.bufV, zerocopy.StrCRLF...)

	return h.bufV
}

func (h *RequestHeader) writeTrailer(w *bufio.Writer) error {
	_, err := w.Write(h.TrailerHeader())
	return err
}

// TrailerHeader returns the serialized wire representation of response trailers (RFC 9112 Section 7.1.2).
func (h *ResponseHeader) TrailerHeader() []byte {
	h.bufV = h.bufV[:0]
	for _, t := range h.trailer {
		value := h.peek(t)
		h.bufV = appendHeaderLine(h.bufV, t, value)
	}

	h.bufV = append(h.bufV, zerocopy.StrCRLF...)

	return h.bufV
}

func (h *ResponseHeader) writeTrailer(w *bufio.Writer) error {
	_, err := w.Write(h.TrailerHeader())
	return err
}

func isValidTrailerKey(key []byte) bool {
	if len(key) == 0 {
		return false
	}

	for _, c := range key {
		if !zerocopy.ValidHeaderFieldByte(c) {
			return false
		}
	}

	return true
}

func validHeaderValueByte(c byte) bool {
	return zerocopy.ValidHeaderValueByteTable[c] == 1
}

func isValidHeaderKey(a []byte) (valid, innerSpace bool) {
	if len(a) == 0 {
		return false, false
	}

	for _, c := range a {
		if !zerocopy.ValidHeaderFieldByte(c) {
			return false, false
		}
	}

	return true, false
}

func parseTrailerHeaders(src []byte, dest *headkit.Headers, disableNormalizing bool) (int, error) {
	var err error

	n := 0
	for len(src) > 0 {
		idxSemi := bytes.IndexByte(src, '\n')
		if idxSemi < 0 {
			break
		}

		line := src[:idxSemi]
		src = src[idxSemi+1:]
		n += idxSemi + 1

		if len(line) > 0 && line[len(line)-1] == '\r' {
			line = line[:len(line)-1]
		}

		if len(line) == 0 {
			break
		}

		idxColon := bytes.IndexByte(line, ':')
		if idxColon > 0 {
			k := line[:idxColon]
			v := bytes.TrimSpace(line[idxColon+1:])
			dest.Add(string(k), string(v))
		}
	}

	return n, err
}

func isBadTrailer(key []byte) bool {
	if len(key) == 0 {
		return true
	}

	switch key[0] | 0x20 {
	case 'a':
		return zerocopy.CaseInsensitiveCompare(key, zerocopy.StrAuthorization)
	case 'c':
		if len(key) >= len(HeaderContentType) && zerocopy.CaseInsensitiveCompare(key[:8], zerocopy.StrContentType[:8]) {
			return zerocopy.CaseInsensitiveCompare(key[8:], zerocopy.StrContentEncoding[8:]) ||
				zerocopy.CaseInsensitiveCompare(key[8:], zerocopy.StrContentLength[8:]) ||
				zerocopy.CaseInsensitiveCompare(key[8:], zerocopy.StrContentType[8:]) ||
				zerocopy.CaseInsensitiveCompare(key[8:], zerocopy.StrContentRange[8:])
		}

		return zerocopy.CaseInsensitiveCompare(key, zerocopy.StrConnection) ||
			zerocopy.CaseInsensitiveCompare(key, zerocopy.StrCookie)

	case 'e':
		return zerocopy.CaseInsensitiveCompare(key, zerocopy.StrExpect)
	case 'h':
		return zerocopy.CaseInsensitiveCompare(key, zerocopy.StrHost)
	case 'k':
		return zerocopy.CaseInsensitiveCompare(key, zerocopy.StrKeepAlive)
	case 'l':
		return zerocopy.CaseInsensitiveCompare(key, zerocopy.StrLocation)
	case 'm':
		return zerocopy.CaseInsensitiveCompare(key, zerocopy.StrMaxForwards)
	case 'p':
		if len(key) >= len(HeaderProxyConnection) &&
			zerocopy.CaseInsensitiveCompare(key[:6], zerocopy.StrProxyConnection[:6]) {
			return zerocopy.CaseInsensitiveCompare(key[6:], zerocopy.StrProxyConnection[6:]) ||
				zerocopy.CaseInsensitiveCompare(key[6:], zerocopy.StrProxyAuthenticate[6:]) ||
				zerocopy.CaseInsensitiveCompare(key[6:], zerocopy.StrProxyAuthorization[6:])
		}

	case 'r':
		return zerocopy.CaseInsensitiveCompare(key, zerocopy.StrRange)
	case 's':
		return zerocopy.CaseInsensitiveCompare(key, zerocopy.StrSetCookie)
	case 't':
		return zerocopy.CaseInsensitiveCompare(key, zerocopy.StrTE) ||
			zerocopy.CaseInsensitiveCompare(key, zerocopy.StrTrailer) ||
			zerocopy.CaseInsensitiveCompare(key, zerocopy.StrTransferEncoding)
	case 'w':
		return zerocopy.CaseInsensitiveCompare(key, zerocopy.StrWWWAuthenticate)
	case 'x':
		return (len(key) >= 11 && zerocopy.CaseInsensitiveCompare(key[:11], []byte("x-forwarded"))) ||
			(len(key) >= 9 && zerocopy.CaseInsensitiveCompare(key[:9], []byte("x-real-ip")))
	}

	return false
}

func appendTrailerBytes(dst []byte, trailer [][]byte, sep []byte) []byte {
	for i, n := 0, len(trailer); i < n; i++ {
		dst = append(dst, trailer[i]...)
		if i+1 < n {
			dst = append(dst, sep...)
		}
	}

	return dst
}

func copyTrailer(dst, src [][]byte) [][]byte {
	if cap(dst) >= len(src) {
		dst = dst[:len(src)]
	} else {
		dst = make([][]byte, len(src))
	}

	for i := range dst {
		l := len(src[i])
		if cap(dst[i]) >= l {
			dst[i] = dst[i][:l]
		} else {
			dst[i] = make([]byte, l)
		}

		copy(dst[i], src[i])
	}

	return dst
}
