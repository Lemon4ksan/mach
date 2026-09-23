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

	"github.com/lemon4ksan/foundation/net/http/status"
	"github.com/lemon4ksan/foundation/net/http/zerocopy"
	"github.com/lemon4ksan/foundation/silicon/bytesconv"
	"github.com/lemon4ksan/foundation/silicon/simd"
)

// Read reads an HTTP request header block from r up to and including the CRLFCRLF terminator (RFC 9112 Section 2.1).
// It returns io.EOF if the stream is closed prior to reading the first byte.
//
// Concurrency: Not goroutine-safe; single-threaded execution required.
func (h *RequestHeader) Read(r *bufio.Reader) error {
	return h.readLoop(r, true)
}

func (h *RequestHeader) readLoop(r *bufio.Reader, waitForMore bool) error {
	n := 1
	for {
		err := h.tryRead(r, n)
		if err == nil {
			return nil
		}

		if !waitForMore || !errors.Is(err, ErrNeedMore) {
			h.resetSkipNormalize()
			return err
		}

		n = r.Buffered() + 1
	}
}

func (h *RequestHeader) tryRead(r *bufio.Reader, n int) error {
	h.resetSkipNormalize()

	b, err := r.Peek(n)
	if len(b) == 0 {
		if err == io.EOF {
			return err
		}

		if err == nil {
			panic("bufio.Reader.Peek() returned nil, nil")
		}

		if errors.Is(err, bufio.ErrBufferFull) {
			return &ErrSmallBuffer{
				error: fmt.Errorf(
					"error when reading request headers: %w (n=%d, reader buffered=%d)",
					ErrSmallReadBuffer,
					n,
					r.Buffered(),
				),
			}
		}

		if n == 1 {
			return ErrNothingRead{error: err}
		}

		return fmt.Errorf("error when reading request headers: %w", err)
	}

	b = mustPeekBuffered(r)

	headersLen, errParse := h.parse(b)
	if errParse != nil {
		return headerError("request", err, errParse, b, h.SecureErrorLogMessage)
	}

	if errValidate := h.validate(); errValidate != nil {
		return headerError("request", err, errValidate, b, h.SecureErrorLogMessage)
	}

	mustDiscard(r, headersLen)

	return nil
}

func (h *RequestHeader) validate() error {
	if h.IsHTTP11() && len(h.Host()) == 0 {
		h.connectionClose = true
		return errRequestHostRequired
	}

	return nil
}

func (h *RequestHeader) ignoreBody() bool {
	return h.IsGet() || h.IsHead()
}

func (h *RequestHeader) parse(buf []byte) (int, error) {
	m, err := h.parseFirstLine(buf)
	if err != nil {
		return 0, err
	}

	var rawEnd int

	h.rawHeaders, rawEnd, err = readRawHeaders(h.rawHeaders[:0], buf[m:])
	if err != nil {
		return 0, err
	}

	n, err := h.parseHeaders(buf[m:], rawEnd)
	if err != nil {
		return 0, err
	}

	return m + n, nil
}

func (h *RequestHeader) parseFirstLine(buf []byte) (int, error) {
	bNext := buf

	var (
		b   []byte
		err error
	)
	for len(b) == 0 {
		if b, bNext, err = nextLine(bNext); err != nil {
			return 0, err
		}
	}

	n := bytes.IndexByte(b, ' ')
	if n <= 0 {
		if h.SecureErrorLogMessage {
			return 0, ErrMissingRequestMethod
		}

		return 0, fmt.Errorf("cannot find http request method in %q", buf)
	}

	h.method = append(h.method[:0], b[:n]...)
	if !isValidMethod(h.method) {
		if h.SecureErrorLogMessage {
			return 0, ErrUnsupportedRequestMethod
		}

		return 0, fmt.Errorf("unsupported http request method %q in %q", h.method, buf)
	}

	b = b[n+1:]
	for len(b) > 0 && b[0] == ' ' {
		b = b[1:]
	}

	n = bytes.LastIndexByte(b, ' ')
	if n < 0 {
		return 0, fmt.Errorf("cannot find whitespace in the first line of request %q", buf)
	}

	protoStr := b[n+1:]
	if !isHTTPVersion(protoStr) {
		return 0, fmt.Errorf("unsupported HTTP version %q", protoStr)
	}

	h.noHTTP11 = !bytes.Equal(protoStr, zerocopy.StrHTTP11)

	uriStr := trimTrailingSpace(b[:n])
	if len(uriStr) == 0 {
		if h.SecureErrorLogMessage {
			return 0, ErrEmptyRequestURI
		}

		return 0, fmt.Errorf("request uri cannot be empty in %q", buf)
	}

	if err := validateRequestURI(h.method, uriStr); err != nil {
		if h.SecureErrorLogMessage {
			return 0, fmt.Errorf("invalid request uri %q", uriStr)
		}

		return 0, fmt.Errorf("invalid request uri %q in %q: %w", uriStr, buf, err)
	}

	h.noHTTP11 = !bytes.Equal(protoStr, zerocopy.StrHTTP11)
	h.protocol = append(h.protocol[:0], protoStr...)
	h.requestURI = append(h.requestURI[:0], uriStr...)

	return len(buf) - len(bNext), nil
}

func (h *RequestHeader) parseHeaders(buf []byte, blockEnd int) (int, error) {
	h.contentLength = -2

	var (
		s                    headerScanner
		err                  error
		contentLengthSeen    bool
		transferEncodingSeen bool
		hostSeen             bool
	)

	s.b = buf
	s.blockEnd = blockEnd

	for s.next() {
		key := s.key
		s.key = trimTrailingSpace(s.key)
		s.value = trimTrailingSpace(s.value)

		if len(s.key) != len(key) {
			h.connectionClose = true
			return 0, fmt.Errorf("invalid header key %q", key)
		}

		if len(s.key) == 0 {
			h.connectionClose = true
			return 0, fmt.Errorf("invalid header key %q", s.key)
		}

		zerocopy.NormalizeHeaderKeyValidated(s.key, h.disableNormalizing || s.keyHasSpace)

		for _, ch := range s.value {
			if !validHeaderValueByte(ch) {
				h.connectionClose = true
				return 0, fmt.Errorf("invalid header value %q", s.value)
			}
		}

		isContentLength := false
		isTransferEncoding := false

		contentLength := 0
		switch s.key[0] | 0x20 {
		case 'c':
			if zerocopy.CaseInsensitiveCompare(s.key, zerocopy.StrContentLength) {
				isContentLength = true
				if contentLengthSeen {
					parsed, err := parseContentLength(s.value)
					if err != nil || parsed != h.contentLength {
						h.connectionClose = true
						return 0, ErrDuplicateContentLength
					}

					isContentLength = false

					continue
				}

				contentLengthSeen = true

				contentLength, err = parseContentLength(s.value)
				if err != nil {
					h.contentLength = -2
					h.connectionClose = true
					return 0, err
				}
			}

		case 't':
			if zerocopy.CaseInsensitiveCompare(s.key, zerocopy.StrTransferEncoding) {
				isTransferEncoding = true

				if h.noHTTP11 {
					h.connectionClose = true
					return 0, ErrUnsupportedTransferEncoding
				}

				if transferEncodingSeen {
					h.connectionClose = true
					if h.SecureErrorLogMessage {
						return 0, ErrUnsupportedTransferEncoding
					}

					return 0, errors.New("too many transfer-encoding headers")
				}

				transferEncodingSeen = true
			}
		}

		if h.disableSpecialHeader {
			appendArgBytesHeaders(&h.h, s.key, s.value, zerocopy.ArgsHasValue)
			continue
		}

		switch s.key[0] | 0x20 {
		case 'h':
			if zerocopy.CaseInsensitiveCompare(s.key, zerocopy.StrHost) {
				if hostSeen {
					h.connectionClose = true
					return 0, errors.New("too many host headers")
				}

				hostSeen = true

				h.host = append(h.host[:0], s.value...)

				continue
			}

		case 'u':
			if zerocopy.CaseInsensitiveCompare(s.key, zerocopy.StrUserAgent) {
				h.userAgent = append(h.userAgent[:0], s.value...)
				continue
			}
		case 'c':
			if zerocopy.CaseInsensitiveCompare(s.key, zerocopy.StrContentType) {
				h.contentType = append(h.contentType[:0], s.value...)
				continue
			}

			if isContentLength {
				if h.contentLength != -1 {
					h.contentLength = contentLength
					h.contentLengthBytes = append(h.contentLengthBytes[:0], s.value...)
				}

				continue
			}

			if zerocopy.CaseInsensitiveCompare(s.key, zerocopy.StrConnection) {
				if hasHeaderValue(s.value, zerocopy.StrClose) {
					h.connectionClose = true
				} else {
					h.connectionClose = false
					appendArgBytesHeaders(&h.h, s.key, s.value, zerocopy.ArgsHasValue)
				}

				continue
			}

		case 't':
			if isTransferEncoding {
				isIdentity := zerocopy.CaseInsensitiveCompare(s.value, zerocopy.StrIdentity)
				isChunked := zerocopy.CaseInsensitiveCompare(s.value, zerocopy.StrChunked)

				if !isIdentity && !isChunked {
					h.connectionClose = true
					if h.SecureErrorLogMessage {
						return 0, ErrUnsupportedTransferEncoding
					}

					return 0, fmt.Errorf("unsupported transfer-encoding: %q", s.value)
				}

				if isChunked {
					h.contentLength = -1
					setArgBytesHeaders(&h.h, zerocopy.StrTransferEncoding, zerocopy.StrChunked, zerocopy.ArgsHasValue)
				}

				continue
			}

			if zerocopy.CaseInsensitiveCompare(s.key, zerocopy.StrTrailer) {
				err := h.SetTrailerBytes(s.value)
				if err != nil {
					h.connectionClose = true
					return 0, err
				}

				continue
			}
		}

		appendArgBytesHeaders(&h.h, s.key, s.value, zerocopy.ArgsHasValue)
	}

	if s.err != nil {
		h.connectionClose = true
		return 0, s.err
	}

	if contentLengthSeen && transferEncodingSeen {
		h.connectionClose = true
		return 0, errors.New("both Content-Length and Transfer-Encoding are present (RFC 9112 Section 6.1)")
	}

	if h.contentLength < 0 {
		h.contentLengthBytes = h.contentLengthBytes[:0]
	}

	if h.noHTTP11 && !h.connectionClose {
		v := peekArgBytesHeaders(&h.h, zerocopy.StrConnection)
		h.connectionClose = !hasHeaderValue(v, zerocopy.StrKeepAlive)
	}

	return s.r, nil
}

// Write writes the serialized wire representation of the HTTP request header to w (RFC 9112 Section 2.1).
//
// Concurrency: Not goroutine-safe.
func (h *RequestHeader) Write(w *bufio.Writer) error {
	_, err := w.Write(h.Header())
	return err
}

// WriteTo writes the serialized wire representation of the request header to w, implementing io.WriterTo (RFC 9112 Section 2.1).
func (h *RequestHeader) WriteTo(w io.Writer) (int64, error) {
	n, err := w.Write(h.Header())
	return int64(n), err
}

// Header returns the byte slice representation of the serialized request header (RFC 9112 Section 2.1).
// The returned slice is valid until the request is reset or released. Do not retain references.
func (h *RequestHeader) Header() []byte {
	h.bufV = h.AppendBytes(h.bufV[:0])
	return h.bufV
}

// RawHeaders returns the unmodified wire byte representation of headers as received from the transport (RFC 9112 Section 2.1).
// The slice is valid only during handler invocation and must not be retained.
func (h *RequestHeader) RawHeaders() []byte {
	return h.rawHeaders
}

// String returns the diagnostic string representation of the request header.
func (h *RequestHeader) String() string {
	return string(h.Header())
}

// AppendBytes appends the serialized wire representation of the request header to dst (RFC 9112 Section 2.1).
func (h *RequestHeader) AppendBytes(dst []byte) []byte {
	dst = append(dst, h.Method()...)
	dst = append(dst, ' ')
	dst = append(dst, h.RequestURI()...)
	dst = append(dst, ' ')
	dst = append(dst, h.Protocol()...)
	dst = append(dst, zerocopy.StrCRLF...)

	userAgent := h.UserAgent()
	if len(userAgent) > 0 && !h.disableSpecialHeader {
		dst = appendHeaderLine(dst, zerocopy.StrUserAgent, userAgent)
	}

	host := h.Host()
	if len(host) > 0 && !h.disableSpecialHeader {
		dst = appendHeaderLine(dst, zerocopy.StrHost, host)
	}

	contentType := h.ContentType()
	if !h.noDefaultContentType && len(contentType) == 0 && h.ContentLength() > 0 {
		contentType = zerocopy.StrDefaultContentType
	}

	if len(contentType) > 0 && !h.disableSpecialHeader {
		dst = appendHeaderLine(dst, zerocopy.StrContentType, contentType)
	}

	if len(h.contentLengthBytes) > 0 && !h.disableSpecialHeader {
		dst = appendHeaderLine(dst, zerocopy.StrContentLength, h.contentLengthBytes)
	}

	for _, _e := range h.h.Entries() {
		kv := &zerocopy.ArgsKV{Key: bytesconv.S2B(_e.Key), Value: bytesconv.S2B(_e.Value)}

		exclude := false
		for _, t := range h.trailer {
			if bytes.Equal(kv.Key, t) {
				exclude = true
				break
			}
		}

		if !exclude {
			dst = appendHeaderLine(dst, kv.Key, kv.Value)
		}
	}

	if len(h.trailer) > 0 {
		dst = appendHeaderLine(dst, zerocopy.StrTrailer, appendTrailerBytes(nil, h.trailer, zerocopy.StrCommaSpace))
	}

	n := len(h.cookies)
	if n > 0 && !h.disableSpecialHeader {
		dst = append(dst, zerocopy.StrCookie...)
		dst = append(dst, zerocopy.StrColonSpace...)
		dst = zerocopy.AppendRequestCookieBytes(dst, h.cookies)
		dst = append(dst, zerocopy.StrCRLF...)
	}

	if h.ConnectionClose() && !h.disableSpecialHeader {
		dst = appendHeaderLine(dst, zerocopy.StrConnection, zerocopy.StrClose)
	}

	return append(dst, zerocopy.StrCRLF...)
}

// Read reads an HTTP response header block from r up to and including the CRLFCRLF terminator (RFC 9112 Section 2.1).
func (h *ResponseHeader) Read(r *bufio.Reader) error {
	n := 1
	for {
		err := h.tryRead(r, n)
		if err == nil {
			return nil
		}

		if !errors.Is(err, ErrNeedMore) {
			h.resetSkipNormalize()
			return err
		}

		n = r.Buffered() + 1
	}
}

func (h *ResponseHeader) tryRead(r *bufio.Reader, n int) error {
	h.resetSkipNormalize()

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
				return &ErrSmallBuffer{error: ErrReadingResponseHeaders}
			}

			return &ErrSmallBuffer{error: fmt.Errorf("error when reading response headers: %w", ErrSmallReadBuffer)}
		}

		return fmt.Errorf("error when reading response headers: %w", err)
	}

	b = mustPeekBuffered(r)

	headersLen, errParse := h.parse(b)
	if errParse != nil {
		return headerError("response", err, errParse, b, h.SecureErrorLogMessage)
	}

	mustDiscard(r, headersLen)

	return nil
}

func (h *ResponseHeader) parse(buf []byte) (int, error) {
	m, err := h.parseFirstLine(buf)
	if err != nil {
		return 0, err
	}

	n, err := h.parseHeaders(buf[m:])
	if err != nil {
		return 0, err
	}

	return m + n, nil
}

func (h *ResponseHeader) parseFirstLine(buf []byte) (int, error) {
	bNext := buf

	var (
		b   []byte
		err error
	)
	for len(b) == 0 {
		if b, bNext, err = nextLine(bNext); err != nil {
			return 0, err
		}
	}

	n := bytes.IndexByte(b, ' ')
	if n < 0 {
		if h.SecureErrorLogMessage {
			return 0, ErrResponseFirstLineMissingSpace
		}

		return 0, fmt.Errorf("cannot find whitespace in the first line of response %q", buf)
	}

	protoStr := b[:n]

	b = b[n+1:]
	for len(b) > 0 && b[0] == ' ' {
		b = b[1:]
	}

	statusCode := b

	statusMessage := []byte(nil)
	if n = bytes.IndexByte(b, ' '); n >= 0 {
		statusCode = b[:n]
		statusMessage = b[n+1:]
	}

	if len(statusCode) != 3 {
		if h.SecureErrorLogMessage {
			return 0, ErrUnexpectedStatusCodeChar
		}

		return 0, fmt.Errorf("invalid response status code %q: response %q", statusCode, buf)
	}

	h.statusCode, n, err = zerocopy.ParseUintBuf(statusCode)
	if err != nil || n != 3 {
		if h.SecureErrorLogMessage {
			return 0, ErrUnexpectedStatusCodeChar
		}

		return 0, fmt.Errorf("invalid response status code %q: response %q", statusCode, buf)
	}

	if !isHTTPVersion(protoStr) {
		if h.SecureErrorLogMessage {
			return 0, fmt.Errorf("unsupported http version %q", protoStr)
		}

		return 0, fmt.Errorf("unsupported http version %q in %q", protoStr, buf)
	}

	h.noHTTP11 = !bytes.Equal(protoStr, zerocopy.StrHTTP11)

	h.protocol = append(h.protocol[:0], protoStr...)
	if len(statusMessage) > 0 {
		h.SetStatusMessage(statusMessage)
	}

	return len(buf) - len(bNext), nil
}

func (h *ResponseHeader) parseHeaders(buf []byte) (int, error) {
	h.contentLength = -2

	var (
		s                    headerScanner
		kv                   *zerocopy.ArgsKV
		transferEncodingSeen bool
		contentLengthSeen    bool
	)

	s.b = buf

	for s.next() {
		s.key = trimTrailingSpace(s.key)
		s.value = trimTrailingSpace(s.value)

		if len(s.key) == 0 {
			h.connectionClose = true
			return 0, fmt.Errorf("invalid header key %q", s.key)
		}

		disableNormalizing := h.disableNormalizing
		if s.keyHasSpace {
			h.connectionClose = true
			disableNormalizing = true
		}

		zerocopy.NormalizeHeaderKeyValidated(s.key, disableNormalizing)

		for _, ch := range s.value {
			if !validHeaderValueByte(ch) {
				h.connectionClose = true
				return 0, fmt.Errorf("invalid header value %q", s.value)
			}
		}

		switch s.key[0] | 0x20 {
		case 'c':
			if zerocopy.CaseInsensitiveCompare(s.key, zerocopy.StrContentType) {
				h.contentType = append(h.contentType[:0], s.value...)
				continue
			}

			if zerocopy.CaseInsensitiveCompare(s.key, zerocopy.StrContentEncoding) {
				h.contentEncoding = append(h.contentEncoding[:0], s.value...)
				continue
			}

			if zerocopy.CaseInsensitiveCompare(s.key, zerocopy.StrContentLength) {
				if contentLengthSeen {
					parsed, err := parseContentLength(s.value)
					if err != nil || parsed != h.contentLength {
						h.connectionClose = true
						return 0, ErrDuplicateContentLength
					}

					continue
				}

				contentLengthSeen = true

				contentLength, err := parseContentLength(s.value)
				if err != nil {
					h.contentLength = -2
					h.connectionClose = true
					return 0, err
				}

				if h.contentLength != -1 {
					h.contentLength = contentLength
					h.contentLengthBytes = append(h.contentLengthBytes[:0], s.value...)
				}

				continue
			}

			if zerocopy.CaseInsensitiveCompare(s.key, zerocopy.StrConnection) {
				if hasHeaderValue(s.value, zerocopy.StrClose) {
					h.connectionClose = true
				} else {
					h.connectionClose = false
					appendArgBytesHeaders(&h.h, s.key, s.value, zerocopy.ArgsHasValue)
				}

				continue
			}

		case 's':
			if zerocopy.CaseInsensitiveCompare(s.key, zerocopy.StrServer) {
				h.server = append(h.server[:0], s.value...)
				continue
			}

			if zerocopy.CaseInsensitiveCompare(s.key, zerocopy.StrSetCookie) {
				h.cookies, kv = zerocopy.AllocArg(h.cookies)
				kv.Key = zerocopy.GetCookieKey(kv.Key, s.value)
				kv.Value = append(kv.Value[:0], s.value...)

				continue
			}

		case 't':
			if zerocopy.CaseInsensitiveCompare(s.key, zerocopy.StrTransferEncoding) {
				if h.noHTTP11 {
					continue
				}

				if transferEncodingSeen {
					h.connectionClose = true
					if h.SecureErrorLogMessage {
						return 0, ErrUnsupportedTransferEncoding
					}

					return 0, errors.New("too many transfer-encoding headers")
				}

				transferEncodingSeen = true

				if !zerocopy.CaseInsensitiveCompare(s.value, zerocopy.StrChunked) {
					h.connectionClose = true
					if h.SecureErrorLogMessage {
						return 0, ErrUnsupportedTransferEncoding
					}

					return 0, fmt.Errorf("unsupported transfer-encoding: %q", s.value)
				}

				h.contentLength = -1
				setArgBytesHeaders(&h.h, zerocopy.StrTransferEncoding, zerocopy.StrChunked, zerocopy.ArgsHasValue)

				continue
			}

			if zerocopy.CaseInsensitiveCompare(s.key, zerocopy.StrTrailer) {
				err := h.SetTrailerBytes(s.value)
				if err != nil {
					h.connectionClose = true
					return 0, err
				}

				continue
			}
		}

		appendArgBytesHeaders(&h.h, s.key, s.value, zerocopy.ArgsHasValue)
	}

	if s.err != nil {
		h.connectionClose = true
		return 0, s.err
	}

	if contentLengthSeen && transferEncodingSeen {
		h.connectionClose = true
		return 0, errors.New("both Content-Length and Transfer-Encoding are present (RFC 9112 Section 6.1)")
	}

	if h.contentLength < 0 {
		h.contentLengthBytes = h.contentLengthBytes[:0]
	}

	if h.contentLength == -2 && !h.ConnectionUpgrade() && !h.mustSkipContentLength() {
		h.connectionClose = true
	}

	if h.mustSkipContentLength() && (h.contentLength > 0 || h.contentLength == -1) {
		h.connectionClose = true
	}

	if h.noHTTP11 && !h.connectionClose {
		v := peekArgBytesHeaders(&h.h, zerocopy.StrConnection)
		h.connectionClose = !hasHeaderValue(v, zerocopy.StrKeepAlive)
	}

	return s.r, nil
}

// Write writes the serialized wire representation of the HTTP response header to w (RFC 9112 Section 2.1).
func (h *ResponseHeader) Write(w *bufio.Writer) error {
	_, err := w.Write(h.Header())
	return err
}

// WriteTo writes the serialized wire representation of the response header to w, implementing io.WriterTo (RFC 9112 Section 2.1).
func (h *ResponseHeader) WriteTo(w io.Writer) (int64, error) {
	n, err := w.Write(h.Header())
	return int64(n), err
}

// Header returns the byte slice representation of the serialized response header (RFC 9112 Section 2.1).
func (h *ResponseHeader) Header() []byte {
	h.bufV = h.AppendBytes(h.bufV[:0])
	return h.bufV
}

// String returns response header representation.
func (h *ResponseHeader) String() string {
	return string(h.Header())
}

func (h *ResponseHeader) appendStatusLine(dst []byte) []byte {
	statusCode := h.StatusCode()
	if statusCode < 0 {
		statusCode = status.OK
	}

	return status.FormatLine(dst, h.Protocol(), statusCode, h.StatusMessage())
}

// AppendBytes appends response header representation to dst and returns the extended dst.
func (h *ResponseHeader) AppendBytes(dst []byte) []byte {
	dst = h.appendStatusLine(dst[:0])

	server := h.Server()
	if len(server) != 0 {
		dst = appendHeaderLine(dst, zerocopy.StrServer, server)
	}

	if !h.noDefaultDate {
		serverDateOnce.Do(updateServerDate)

		dst = appendHeaderLine(dst, zerocopy.StrDate, *serverDate.Load())
	}

	if h.ContentLength() != 0 || len(h.contentType) > 0 {
		contentType := h.ContentType()
		if len(contentType) > 0 {
			dst = appendHeaderLine(dst, zerocopy.StrContentType, contentType)
		}
	}

	contentEncoding := h.ContentEncoding()
	if len(contentEncoding) > 0 {
		dst = appendHeaderLine(dst, zerocopy.StrContentEncoding, contentEncoding)
	}

	if len(h.contentLengthBytes) > 0 {
		dst = appendHeaderLine(dst, zerocopy.StrContentLength, h.contentLengthBytes)
	}

	for _, _e := range h.h.Entries() {
		kv := &zerocopy.ArgsKV{Key: bytesconv.S2B(_e.Key), Value: bytesconv.S2B(_e.Value)}

		exclude := false
		for _, t := range h.trailer {
			if bytes.Equal(kv.Key, t) {
				exclude = true
				break
			}
		}

		if !exclude && (h.noDefaultDate || !bytes.Equal(kv.Key, zerocopy.StrDate)) {
			dst = appendHeaderLine(dst, kv.Key, kv.Value)
		}
	}

	if len(h.trailer) > 0 {
		dst = appendHeaderLine(dst, zerocopy.StrTrailer, appendTrailerBytes(nil, h.trailer, zerocopy.StrCommaSpace))
	}

	n := len(h.cookies)
	if n > 0 {
		for i := range n {
			kv := &h.cookies[i]
			dst = appendHeaderLine(dst, zerocopy.StrSetCookie, kv.Value)
		}
	}

	if h.ConnectionClose() {
		dst = appendHeaderLine(dst, zerocopy.StrConnection, zerocopy.StrClose)
	}

	return append(dst, zerocopy.StrCRLF...)
}

func readRawHeaders(dst, buf []byte) ([]byte, int, error) {
	if len(buf) == 0 {
		return dst[:0], 0, ErrNeedMore
	}

	if simd.MatchCRLF(buf) {
		return dst, 2, nil
	}

	if buf[0] == nChar {
		return dst, 1, nil
	}

	idx := simd.IndexCRLFCRLF(buf)
	if idx < 0 {
		return dst, 0, ErrNeedMore
	}

	dst = append(dst, buf[:idx]...)

	return dst, idx, nil
}

func parseContentLength(b []byte) (int, error) {
	v, n, err := zerocopy.ParseUintBuf(b)
	if err != nil {
		return -1, fmt.Errorf("cannot parse content-length: %w", err)
	}

	if n != len(b) {
		return -1, fmt.Errorf("cannot parse content-length: %w", ErrNonNumericChars)
	}

	return v, nil
}

func nextLine(b []byte) ([]byte, []byte, error) {
	nNext := simd.IndexByteVector(b, nChar)
	if nNext < 0 {
		return nil, nil, ErrNeedMore
	}

	n := nNext
	if n > 0 && b[n-1] == rChar {
		n--
	}

	return b[:n], b[nNext+1:], nil
}

func isHTTPVersion(proto []byte) bool {
	return len(proto) == len(zerocopy.StrHTTP11) && bytes.HasPrefix(proto, zerocopy.StrHTTP11[:5]) && proto[6] == '.' &&
		proto[5] >= '0' &&
		proto[5] <= '9' &&
		proto[7] >= '0' &&
		proto[7] <= '9'
}

func isValidMethod(method []byte) bool {
	for _, ch := range method {
		if zerocopy.ValidMethodValueByteTable[ch] == 0 {
			return false
		}
	}

	return true
}

func validateRequestURI(method, requestURI []byte) error {
	if zerocopy.StringContainsCTLByte(requestURI) {
		return zerocopy.ErrorInvalidURI
	}

	if bytes.IndexByte(requestURI, ' ') >= 0 {
		return zerocopy.ErrorInvalidURI
	}

	if len(requestURI) == 1 && requestURI[0] == '*' {
		return nil
	}

	if len(requestURI) > 0 && requestURI[0] == '/' {
		return nil
	}

	if before, _, ok := bytes.Cut(requestURI, zerocopy.StrColonSlashSlash); ok {
		if !zerocopy.IsValidScheme(before) {
			return zerocopy.ErrorInvalidURI
		}

		return nil
	}

	if bytes.Equal(method, zerocopy.StrConnect) {
		return nil
	}

	return zerocopy.ErrorInvalidURI
}

func headerError(typ string, err, errParse error, b []byte, secureErrorLogMessage bool) error {
	if !errors.Is(errParse, ErrNeedMore) {
		return headerErrorMsg(typ, errParse, b, secureErrorLogMessage)
	}

	if err == nil {
		return ErrNeedMore
	}

	if isOnlyCRLF(b) {
		return io.EOF
	}

	if !errors.Is(err, bufio.ErrBufferFull) {
		return headerErrorMsg(typ, err, b, secureErrorLogMessage)
	}

	return &ErrSmallBuffer{error: headerErrorMsg(typ, ErrSmallReadBuffer, b, secureErrorLogMessage)}
}

func headerErrorMsg(typ string, err error, b []byte, secureErrorLogMessage bool) error {
	if secureErrorLogMessage {
		return fmt.Errorf("error when reading %s headers: %w: buffer size=%d", typ, err, len(b))
	}

	return fmt.Errorf(
		"error when reading %s headers: %w: buffer size=%d, contents: %s",
		typ,
		err,
		len(b),
		bufferSnippet(b),
	)
}

func bufferSnippet(b []byte) string {
	n := len(b)
	start := 200

	end := n - start
	if start >= end {
		start = n
		end = n
	}

	bStart, bEnd := b[:start], b[end:]
	if len(bEnd) == 0 {
		return fmt.Sprintf("%q", b)
	}

	return fmt.Sprintf("%q...%q", bStart, bEnd)
}

func isOnlyCRLF(b []byte) bool {
	for _, ch := range b {
		if ch != rChar && ch != nChar {
			return false
		}
	}

	return true
}

func mustPeekBuffered(r *bufio.Reader) []byte {
	buf, err := r.Peek(r.Buffered())
	if len(buf) == 0 || err != nil {
		panic(fmt.Sprintf("bufio.Reader.Peek() returned unexpected data (%q, %v)", buf, err))
	}

	return buf
}

func mustDiscard(r *bufio.Reader, n int) {
	if _, err := r.Discard(n); err != nil {
		panic(fmt.Sprintf("bufio.Reader.Discard(%d) failed: %v", n, err))
	}
}
