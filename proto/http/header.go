// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package http

import (
	"errors"

	"github.com/lemon4ksan/foundation/net/headkit"
	"github.com/lemon4ksan/foundation/net/http/zerocopy"
)

const (
	rChar = byte('\r')
	nChar = byte('\n')
)

var (
	// ErrBadTrailer is returned when a message contains a forbidden trailer field (RFC 9112 Section 7.1.2).
	ErrBadTrailer = errors.New("mach: contain forbidden trailer")

	// ErrReadingResponseHeaders is returned when an error occurs while reading response headers (RFC 9112 Section 2.1).
	ErrReadingResponseHeaders = errors.New("mach: error when reading response headers")

	// ErrReadingResponseTrailer is returned when an error occurs while reading response trailers (RFC 9112 Section 7.1.2).
	ErrReadingResponseTrailer = errors.New("mach: error when reading response trailer")

	// ErrResponseFirstLineMissingSpace is returned when whitespace is missing in the status line (RFC 9112 Section 3.1).
	ErrResponseFirstLineMissingSpace = errors.New("mach: cannot find whitespace in the first line of response")

	// ErrUnexpectedStatusCodeChar is returned when a status code contains non-digit characters (RFC 9112 Section 3.1.2).
	ErrUnexpectedStatusCodeChar = errors.New("mach: unexpected char at the end of status code")

	// ErrMissingRequestMethod is returned when an HTTP request line is missing the method token (RFC 9112 Section 3.1.1).
	ErrMissingRequestMethod = errors.New("mach: cannot find http request method")

	// ErrUnsupportedRequestMethod is returned when an unsupported method token is encountered (RFC 9112 Section 3.1.1).
	ErrUnsupportedRequestMethod = errors.New("mach: unsupported http request method")

	// ErrExtraWhitespaceInRequestLine is returned when redundant whitespace appears in the request line (RFC 9112 Section 3).
	ErrExtraWhitespaceInRequestLine = errors.New("mach: extra whitespace in request line")

	// ErrEmptyRequestURI is returned when the target request-URI is empty (RFC 9112 Section 3.2).
	ErrEmptyRequestURI = errors.New("mach: requesturi cannot be empty")

	// ErrDuplicateContentLength is returned when duplicate conflicting Content-Length headers are received (RFC 9112 Section 6.2).
	ErrDuplicateContentLength = errors.New("mach: duplicate content-length header")

	// ErrUnsupportedTransferEncoding is returned when an unsupported Transfer-Encoding coding is encountered (RFC 9112 Section 6.1).
	ErrUnsupportedTransferEncoding = errors.New("mach: unsupported transfer-encoding")

	// ErrNonNumericChars is returned when Content-Length contains non-numeric characters (RFC 9110 Section 8.6).
	ErrNonNumericChars = errors.New("mach: non-numeric chars found")

	// ErrNeedMore indicates that more data is required to complete header parsing.
	ErrNeedMore = errors.New("mach: need more data: cannot find trailing lf")

	// ErrSmallReadBuffer is returned when the read buffer is too small to contain the headers.
	ErrSmallReadBuffer = errors.New("mach: small read buffer. increase readbuffersize")
)

// ErrNothingRead is returned when a keep-alive connection is closed,
// either because the remote closed it or because of a read timeout.
type ErrNothingRead struct{ error }

// Unwrap returns the underlying error.
func (e ErrNothingRead) Unwrap() error { return e.error }

// ErrSmallBuffer is returned when the provided buffer size is too small
// for reading request and/or response headers.
//
// ReadBufferSize value from Server or clients should reduce the number
// of such errors.
type ErrSmallBuffer struct{ error }

// Unwrap returns the underlying error.
func (e *ErrSmallBuffer) Unwrap() error { return e.error }

// header represents common HTTP header state shared between RequestHeader and ResponseHeader.
type header struct {
	h                     headkit.Headers
	cookies               []zerocopy.ArgsKV
	bufK                  []byte
	bufV                  []byte
	contentLengthBytes    []byte
	contentType           []byte
	protocol              []byte
	mulHeader             [][]byte
	trailer               [][]byte
	contentLength         int
	disableNormalizing    bool
	SecureErrorLogMessage bool
	noHTTP11              bool
	connectionClose       bool
	noDefaultContentType  bool
}

// ConnectionClose returns true if 'Connection: close' header is set (RFC 9110 Section 7.6.1).
func (h *header) ConnectionClose() bool {
	return h.connectionClose
}

// SetConnectionClose sets 'Connection: close' header (RFC 9110 Section 7.6.1).
func (h *header) SetConnectionClose() {
	h.connectionClose = true
}

// ResetConnectionClose clears 'Connection: close' header if it exists (RFC 9110 Section 7.6.1).
func (h *header) ResetConnectionClose() {
	if h.connectionClose {
		h.connectionClose = false
		h.h.Del(HeaderConnection)
	}
}

// SetContentType sets Content-Type header value (RFC 9110 Section 8.3).
func (h *header) SetContentType(contentType string) {
	h.contentType = zerocopy.InitHeaderValueString(h.contentType, contentType)
}

// SetContentTypeBytes sets Content-Type header value (RFC 9110 Section 8.3).
func (h *header) SetContentTypeBytes(contentType []byte) {
	h.contentType = zerocopy.InitHeaderValueBytes(h.contentType, contentType)
}

// Protocol returns HTTP protocol version bytes (RFC 9112 Section 2.3).
func (h *header) Protocol() []byte {
	if len(h.protocol) == 0 {
		return zerocopy.StrHTTP11
	}

	return h.protocol
}

// IsHTTP11 returns true if the protocol is HTTP/1.1 (RFC 9112 Section 2.3).
func (h *header) IsHTTP11() bool {
	return !h.noHTTP11
}

// DisableNormalizing disables header names' normalization.
//
// By default all header names are normalized by uppercasing the first letter
// and all first letters following dashes, while lowercasing all other letters.
// The previous setting is returned.
func (h *header) DisableNormalizing() bool {
	orig := h.disableNormalizing
	h.disableNormalizing = true
	return orig
}

// EnableNormalizing enables header names' normalization.
//
// Header names are normalized by uppercasing the first letter and
// all first letters following dashes, while lowercasing all other letters.
// The previous setting is returned.
func (h *header) EnableNormalizing() bool {
	orig := h.disableNormalizing
	h.disableNormalizing = false
	return orig
}

// SetNoDefaultContentType controls whether a default Content-Type header will be omitted.
func (h *header) SetNoDefaultContentType(noDefaultContentType bool) {
	h.noDefaultContentType = noDefaultContentType
}

func (h *header) copyTo(dst *header) {
	dst.disableNormalizing = h.disableNormalizing
	dst.noHTTP11 = h.noHTTP11
	dst.connectionClose = h.connectionClose
	dst.noDefaultContentType = h.noDefaultContentType
	dst.contentLength = h.contentLength
	dst.contentLengthBytes = append(dst.contentLengthBytes, h.contentLengthBytes...)
	dst.protocol = append(dst.protocol, h.protocol...)
	dst.contentType = append(dst.contentType, h.contentType...)
	dst.trailer = copyTrailer(dst.trailer, h.trailer)
	dst.cookies = zerocopy.CopyArgs(dst.cookies, h.cookies)
	copyHeaders(&dst.h, &h.h)
}

func (h *header) setNonSpecial(key, value []byte) {
	setArgBytesHeaders(&h.h, key, value, zerocopy.ArgsHasValue)
}

// RequestHeader represents an HTTP request header block (RFC 9112 Section 2.1).
//
// It is forbidden copying RequestHeader instances by value.
// Create new instances instead and use CopyTo.
//
// RequestHeader instance MUST NOT be used from concurrently running goroutines.
type RequestHeader struct {
	header
	noCopy zerocopy.NoCopy
	method []byte

	requestURI []byte
	host       []byte
	userAgent  []byte
	rawHeaders []byte // stores an immutable copy of headers as they were received from the wire.

	disableSpecialHeader bool
	cookiesCollected     bool
}

// Reset clears request header.
//
// Thread-safe: No.
func (h *RequestHeader) Reset() {
	h.disableSpecialHeader = false
	h.disableNormalizing = false
	h.SetNoDefaultContentType(false)
	h.resetSkipNormalize()
}

func (h *RequestHeader) resetSkipNormalize() {
	h.noHTTP11 = false
	h.connectionClose = false
	h.contentLength = 0
	h.contentLengthBytes = h.contentLengthBytes[:0]
	h.method = h.method[:0]
	h.protocol = h.protocol[:0]
	h.requestURI = h.requestURI[:0]
	h.host = h.host[:0]
	h.contentType = h.contentType[:0]
	h.userAgent = h.userAgent[:0]
	h.trailer = h.trailer[:0]
	h.mulHeader = h.mulHeader[:0]
	h.h.Reset()
	h.cookies = h.cookies[:0]
	h.cookiesCollected = false
	h.rawHeaders = h.rawHeaders[:0]
}

// CopyTo copies all headers from h to dst.
//
// Thread-safe: No.
func (h *RequestHeader) CopyTo(dst *RequestHeader) {
	dst.Reset()
	h.copyTo(&dst.header)
	dst.method = append(dst.method, h.method...)
	dst.requestURI = append(dst.requestURI, h.requestURI...)
	dst.host = append(dst.host, h.host...)
	dst.userAgent = append(dst.userAgent, h.userAgent...)
	dst.cookiesCollected = h.cookiesCollected
	dst.rawHeaders = append(dst.rawHeaders, h.rawHeaders...)
}

// DisableSpecialHeader disables special header processing.
//
// Special headers are: Content-Type, Content-Length, Connection, Transfer-Encoding, Host, User-Agent.
// The previous setting is returned.
func (h *RequestHeader) DisableSpecialHeader() bool {
	orig := h.disableSpecialHeader
	h.disableSpecialHeader = true
	return orig
}

// EnableSpecialHeader enables special header processing.
//
// Special headers are: Content-Type, Content-Length, Connection, Transfer-Encoding, Host, User-Agent.
// The previous setting is returned.
func (h *RequestHeader) EnableSpecialHeader() bool {
	orig := h.disableSpecialHeader
	h.disableSpecialHeader = false
	return orig
}

// ResponseHeader represents an HTTP response header block (RFC 9112 Section 2.1).
//
// It is forbidden copying ResponseHeader instances by value.
// Create new instances instead and use CopyTo.
//
// ResponseHeader instance MUST NOT be used from concurrently running goroutines.
type ResponseHeader struct {
	header
	noCopy        zerocopy.NoCopy
	statusMessage []byte

	contentEncoding []byte
	server          []byte
	statusCode      int
	noDefaultDate   bool
}

// Reset clears response header.
//
// Thread-safe: No.
func (h *ResponseHeader) Reset() {
	h.disableNormalizing = false
	h.SetNoDefaultContentType(false)
	h.noDefaultDate = false
	h.resetSkipNormalize()
}

func (h *ResponseHeader) resetSkipNormalize() {
	h.noHTTP11 = false
	h.connectionClose = false
	h.statusCode = 0
	h.statusMessage = h.statusMessage[:0]
	h.protocol = h.protocol[:0]
	h.contentLength = 0
	h.contentLengthBytes = h.contentLengthBytes[:0]
	h.contentType = h.contentType[:0]
	h.contentEncoding = h.contentEncoding[:0]
	h.server = h.server[:0]
	h.h.Reset()
	h.cookies = h.cookies[:0]
	h.trailer = h.trailer[:0]
	h.mulHeader = h.mulHeader[:0]
}

// CopyTo copies all headers from h to dst.
//
// Thread-safe: No.
func (h *ResponseHeader) CopyTo(dst *ResponseHeader) {
	dst.Reset()
	h.copyTo(&dst.header)
	dst.noDefaultDate = h.noDefaultDate
	dst.statusCode = h.statusCode
	dst.statusMessage = append(dst.statusMessage, h.statusMessage...)
	dst.contentEncoding = append(dst.contentEncoding, h.contentEncoding...)
	dst.server = append(dst.server, h.server...)
}
