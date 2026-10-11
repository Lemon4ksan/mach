// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package http

import (
	"bytes"
	"iter"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lemon4ksan/foundation/silicon/bytesconv"

	"github.com/lemon4ksan/mach/proto/headkit"
	"github.com/lemon4ksan/mach/proto/semantic/altsvc"
	"github.com/lemon4ksan/mach/proto/semantic/status"
	"github.com/lemon4ksan/mach/proto/semantic/cookie"
	"github.com/lemon4ksan/mach/proto/semantic/uri"
)

var (
	strMethodGet     = []byte(MethodGet)
	strMethodHead    = []byte(MethodHead)
	strMethodPost    = []byte(MethodPost)
	strMethodPut     = []byte(MethodPut)
	strMethodDelete  = []byte(MethodDelete)
	strMethodConnect = []byte(MethodConnect)
	strMethodOptions = []byte(MethodOptions)
	strMethodTrace   = []byte(MethodTrace)
	strMethodPatch   = []byte(MethodPatch)
	strAltSvc        = []byte("Alt-Svc")
)

// ConnectionUpgrade returns true if 'Connection: Upgrade' header is set (RFC 9110 Section 7.8).
//
// Thread-safe: No.
func (h *RequestHeader) ConnectionUpgrade() bool {
	return hasHeaderValue(h.Peek(headkit.Connection), StrUpgrade)
}

// ContentLength returns Content-Length header value as an integer (RFC 9110 Section 8.6, RFC 9112 Section 6.2).
//
// Negative value means that Content-Length header is not set.
// -1 means that Transfer-Encoding: chunked is used.
//
// Thread-safe: No.
func (h *RequestHeader) ContentLength() int {
	return h.contentLength
}

// SetContentLength sets Content-Length header value (RFC 9110 Section 8.6, RFC 9112 Section 6.2).
//
// Negative value removes Content-Length header.
//
// Thread-safe: No.
func (h *RequestHeader) SetContentLength(contentLength int) {
	if contentLength < 0 {
		h.contentLength = -2
		h.contentLengthBytes = h.contentLengthBytes[:0]
		return
	}

	h.contentLength = contentLength
	h.contentLengthBytes = AppendUint(h.contentLengthBytes[:0], contentLength)
}

// ContentType returns Content-Type header value (RFC 9110 Section 8.3).
//
// Thread-safe: No.
func (h *RequestHeader) ContentType() []byte {
	if h.disableSpecialHeader {
		return peekArgBytesHeaders(&h.h, StrContentType)
	}

	return h.contentType
}

// ContentEncoding returns Content-Encoding header value (RFC 9110 Section 8.4).
//
// Thread-safe: No.
func (h *RequestHeader) ContentEncoding() []byte {
	return peekArgBytesHeaders(&h.h, StrContentEncoding)
}

// SetContentEncoding sets Content-Encoding header value (RFC 9110 Section 8.4).
//
// Thread-safe: No.
func (h *RequestHeader) SetContentEncoding(contentEncoding string) {
	h.setNonSpecial(StrContentEncoding, bytesconv.S2B(contentEncoding))
}

// SetContentEncodingBytes sets Content-Encoding header value (RFC 9110 Section 8.4).
//
// Thread-safe: No.
func (h *RequestHeader) SetContentEncodingBytes(contentEncoding []byte) {
	h.setNonSpecial(StrContentEncoding, contentEncoding)
}

// SetMultipartFormBoundary sets the following Content-Type:
// 'multipart/form-data; boundary=...' (RFC 7578 Section 4.1).
//
// Thread-safe: No.
func (h *RequestHeader) SetMultipartFormBoundary(boundary string) {
	b := h.bufV[:0]
	b = append(b, StrMultipartFormData...)
	b = append(b, ';', ' ')
	b = append(b, StrBoundary...)
	b = append(b, '=')
	b = append(b, boundary...)
	h.bufV = b
	h.SetContentTypeBytes(h.bufV)
}

// MultipartFormBoundary returns boundary part from 'multipart/form-data; boundary=...' Content-Type (RFC 7578 Section 4.1).
//
// Thread-safe: No.
func (h *RequestHeader) MultipartFormBoundary() []byte {
	b := h.ContentType()
	if !bytes.HasPrefix(b, StrMultipartFormData) {
		return nil
	}

	b = b[len(StrMultipartFormData):]
	if len(b) == 0 || b[0] != ';' {
		return nil
	}

	var n int
	for len(b) > 0 {
		n++
		for len(b) > n && b[n] == ' ' {
			n++
		}

		b = b[n:]
		if !bytes.HasPrefix(b, StrBoundary) {
			if n = bytes.IndexByte(b, ';'); n < 0 {
				return nil
			}

			continue
		}

		b = b[len(StrBoundary):]
		if len(b) == 0 || b[0] != '=' {
			return nil
		}

		b = b[1:]
		if n = bytes.IndexByte(b, ';'); n >= 0 {
			b = b[:n]
		}

		if len(b) > 1 && b[0] == '"' && b[len(b)-1] == '"' {
			b = b[1 : len(b)-1]
		}

		return b
	}

	return nil
}

// Host returns Host header value (RFC 9112 Section 7.2, RFC 9110 Section 7.2).
//
// Thread-safe: No.
func (h *RequestHeader) Host() []byte {
	if h.disableSpecialHeader {
		return peekArgBytesHeaders(&h.h, StrHost)
	}

	return h.host
}

// SetHost sets Host header value (RFC 9112 Section 7.2, RFC 9110 Section 7.2).
//
// Thread-safe: No.
func (h *RequestHeader) SetHost(host string) {
	h.host = cookie.InitHeaderValueString(h.host, host)
}

// SetHostBytes sets Host header value (RFC 9112 Section 7.2, RFC 9110 Section 7.2).
//
// Thread-safe: No.
func (h *RequestHeader) SetHostBytes(host []byte) {
	h.host = cookie.InitHeaderValueBytes(h.host, host)
}

// UserAgent returns User-Agent header value (RFC 9110 Section 10.1.5).
//
// Thread-safe: No.
func (h *RequestHeader) UserAgent() []byte {
	if h.disableSpecialHeader {
		return peekArgBytesHeaders(&h.h, StrUserAgent)
	}

	return h.userAgent
}

// SetUserAgent sets User-Agent header value (RFC 9110 Section 10.1.5).
//
// Thread-safe: No.
func (h *RequestHeader) SetUserAgent(userAgent string) {
	h.userAgent = cookie.InitHeaderValueString(h.userAgent, userAgent)
}

// SetUserAgentBytes sets User-Agent header value (RFC 9110 Section 10.1.5).
//
// Thread-safe: No.
func (h *RequestHeader) SetUserAgentBytes(userAgent []byte) {
	h.userAgent = cookie.InitHeaderValueBytes(h.userAgent, userAgent)
}

// Method returns HTTP request method (RFC 9110 Section 9).
//
// Thread-safe: No.
func (h *RequestHeader) Method() []byte {
	if len(h.method) == 0 {
		return strMethodGet
	}

	return h.method
}

// SetMethod sets HTTP request method (RFC 9110 Section 9).
//
// Thread-safe: No.
func (h *RequestHeader) SetMethod(method string) {
	h.method = cookie.InitHeaderValueString(h.method, method)
}

// SetMethodBytes sets HTTP request method (RFC 9110 Section 9).
//
// Thread-safe: No.
func (h *RequestHeader) SetMethodBytes(method []byte) {
	h.method = cookie.InitHeaderValueBytes(h.method, method)
}

// SetProtocol sets HTTP request protocol (RFC 9112 Section 2.3).
//
// Thread-safe: No.
func (h *RequestHeader) SetProtocol(protocol string) {
	h.protocol = cookie.InitHeaderValueString(h.protocol, protocol)
	h.noHTTP11 = !bytes.Equal(h.protocol, StrHTTP11)
}

// RequestURI returns RequestURI from the first HTTP request line (RFC 9112 Section 3.2).
//
// Thread-safe: No.
func (h *RequestHeader) RequestURI() []byte {
	requestURI := h.requestURI
	if len(requestURI) == 0 {
		requestURI = StrSlash
	}

	return requestURI
}

// SetRequestURI sets RequestURI for the first HTTP request line (RFC 9112 Section 3.2).
//
// Thread-safe: No.
func (h *RequestHeader) SetRequestURI(requestURI string) {
	h.requestURI = cookie.InitHeaderValueString(h.requestURI, requestURI)
}

// SetRequestURIBytes sets RequestURI for the first HTTP request line (RFC 9112 Section 3.2).
//
// Thread-safe: No.
func (h *RequestHeader) SetRequestURIBytes(requestURI []byte) {
	h.requestURI = cookie.InitHeaderValueBytes(h.requestURI, requestURI)
}

// IsGet returns true if request method is GET (RFC 9110 Section 9.3.1).
func (h *RequestHeader) IsGet() bool {
	return bytes.Equal(h.Method(), strMethodGet)
}

// IsHead returns true if request method is HEAD (RFC 9110 Section 9.3.2).
func (h *RequestHeader) IsHead() bool {
	return bytes.Equal(h.Method(), strMethodHead)
}

// IsPost returns true if request method is POST (RFC 9110 Section 9.3.3).
func (h *RequestHeader) IsPost() bool {
	return bytes.Equal(h.Method(), strMethodPost)
}

// IsPut returns true if request method is PUT (RFC 9110 Section 9.3.4).
func (h *RequestHeader) IsPut() bool {
	return bytes.Equal(h.Method(), strMethodPut)
}

// IsDelete returns true if request method is DELETE (RFC 9110 Section 9.3.5).
func (h *RequestHeader) IsDelete() bool {
	return bytes.Equal(h.Method(), strMethodDelete)
}

// IsConnect returns true if request method is CONNECT (RFC 9110 Section 9.3.6).
func (h *RequestHeader) IsConnect() bool {
	return bytes.Equal(h.Method(), strMethodConnect)
}

// IsOptions returns true if request method is OPTIONS (RFC 9110 Section 9.3.7).
func (h *RequestHeader) IsOptions() bool {
	return bytes.Equal(h.Method(), strMethodOptions)
}

// IsTrace returns true if request method is TRACE (RFC 9110 Section 9.3.8).
func (h *RequestHeader) IsTrace() bool {
	return bytes.Equal(h.Method(), strMethodTrace)
}

// IsPatch returns true if request method is PATCH (RFC 5789 Section 2).
func (h *RequestHeader) IsPatch() bool {
	return bytes.Equal(h.Method(), strMethodPatch)
}

// Len returns the number of headers set.
//
// Thread-safe: No.
func (h *RequestHeader) Len() int {
	n := 0
	for range h.All() {
		n++
	}

	return n
}

// All returns an iterator over key-value pairs in h (RFC 9110 Section 5).
// The key and value may be invalid outside the iteration loop.
//
// Modifying RequestHeader during iteration causes undefined behavior.
//
// Thread-safe: No.
func (h *RequestHeader) All() iter.Seq2[[]byte, []byte] {
	return func(yield func([]byte, []byte) bool) {
		for k, v := range h.h.All() {
			if !yield(bytesconv.S2B(k), bytesconv.S2B(v)) {
				return
			}
		}

		if len(h.host) > 0 {
			if !yield(StrHost, h.host) {
				return
			}
		}

		if len(h.contentType) > 0 {
			if !yield(StrContentType, h.contentType) {
				return
			}
		}

		if len(h.userAgent) > 0 {
			if !yield(StrUserAgent, h.userAgent) {
				return
			}
		}

		if h.contentLength >= 0 {
			if !yield(StrContentLength, h.contentLengthBytes) {
				return
			}
		}

		if h.connectionClose {
			if !yield(StrConnection, StrClose) {
				return
			}
		}

		if len(h.cookies) > 0 {
			for i := range h.cookies {
				if !yield(StrCookie, h.cookies[i].Value) {
					return
				}
			}
		}

		if len(h.trailer) > 0 {
			h.bufV = appendTrailerBytes(h.bufV[:0], h.trailer, StrCommaSpace)
			if !yield(StrTrailer, h.bufV) {
				return
			}
		}
	}
}

// Del deletes header with the given key (RFC 9110 Section 5).
//
// Thread-safe: No.
func (h *RequestHeader) Del(key string) {
	switch key {
	case headkit.Host:
		h.host = h.host[:0]
	case headkit.ContentType:
		h.contentType = h.contentType[:0]
	case headkit.UserAgent:
		h.userAgent = h.userAgent[:0]
	case headkit.Cookie:
		h.cookies = h.cookies[:0]
	case headkit.ContentLength:
		h.contentLength = 0
		h.contentLengthBytes = h.contentLengthBytes[:0]
	case headkit.Connection:
		h.connectionClose = false
	case headkit.Trailer:
		h.trailer = h.trailer[:0]
	}

	h.h.Del(key)
}

// DelBytes deletes header with the given key bytes (RFC 9110 Section 5).
//
// Thread-safe: No.
func (h *RequestHeader) DelBytes(key []byte) {
	h.bufK = append(h.bufK[:0], key...)
	NormalizeHeaderKey(h.bufK, h.disableNormalizing)
	h.del(h.bufK)
}

func (h *RequestHeader) del(key []byte) {
	k := bytesconv.B2S(key)
	switch k {
	case headkit.Host:
		h.host = h.host[:0]
	case headkit.ContentType:
		h.contentType = h.contentType[:0]
	case headkit.UserAgent:
		h.userAgent = h.userAgent[:0]
	case headkit.Cookie:
		h.cookies = h.cookies[:0]
	case headkit.ContentLength:
		h.contentLength = 0
		h.contentLengthBytes = h.contentLengthBytes[:0]
	case headkit.Connection:
		h.connectionClose = false
	case headkit.Trailer:
		h.trailer = h.trailer[:0]
	}

	h.h.Del(k)
}

func (h *RequestHeader) setSpecialHeader(key, value []byte) bool {
	if len(key) == 0 || h.disableSpecialHeader {
		return false
	}

	switch key[0] | 0x20 {
	case 'c':
		switch {
		case CaseInsensitiveCompare(StrContentType, key):
			h.SetContentTypeBytes(value)
			return true
		case CaseInsensitiveCompare(StrContentLength, key):
			if contentLength, err := parseContentLength(value); err == nil {
				h.contentLength = contentLength
				h.contentLengthBytes = append(h.contentLengthBytes[:0], value...)
			}

			return true

		case CaseInsensitiveCompare(StrConnection, key):
			if hasHeaderValue(value, StrClose) {
				h.SetConnectionClose()
				return true
			}

			h.ResetConnectionClose()
			h.setNonSpecial(key, value)
			return true

		case CaseInsensitiveCompare(StrCookie, key):
			h.collectCookies()
			h.cookies = cookie.ParseRequestCookies(h.cookies, value)
			return true
		}

	case 't':
		if CaseInsensitiveCompare(StrTransferEncoding, key) {
			return true
		} else if CaseInsensitiveCompare(StrTrailer, key) {
			_ = h.SetTrailerBytes(value)
			return true
		}

	case 'h':
		if CaseInsensitiveCompare(StrHost, key) {
			h.SetHostBytes(value)
			return true
		}
	case 'u':
		if CaseInsensitiveCompare(StrUserAgent, key) {
			h.SetUserAgentBytes(value)
			return true
		}
	}

	return false
}

// Add adds the given 'Key: value' header (RFC 9110 Section 5.2).
// Multiple headers with the same key may be added with this function.
//
// Thread-safe: No.
func (h *RequestHeader) Add(key, value string) {
	h.bufK, h.bufV = InitHeaderKV(h.bufK, h.bufV, key, value, h.disableNormalizing)
	if h.setSpecialHeader(h.bufK, h.bufV) {
		return
	}

	h.h.Add(key, value)
}

// AddBytesKV adds the given 'Key: value' header (RFC 9110 Section 5.2).
//
// Thread-safe: No.
func (h *RequestHeader) AddBytesKV(key, value []byte) {
	h.bufK, h.bufV = InitHeaderKV(
		h.bufK,
		h.bufV,
		bytesconv.B2S(key),
		bytesconv.B2S(value),
		h.disableNormalizing,
	)
	if h.setSpecialHeader(h.bufK, h.bufV) {
		return
	}

	appendArgBytesHeaders(&h.h, h.bufK, h.bufV)
}

// Set sets the given 'Key: value' header (RFC 9110 Section 5.1).
// Use Add for setting multiple header values under the same key.
//
// Thread-safe: No.
func (h *RequestHeader) Set(key, value string) {
	h.bufK, h.bufV = InitHeaderKV(h.bufK, h.bufV, key, value, h.disableNormalizing)
	if h.setSpecialHeader(h.bufK, h.bufV) {
		return
	}

	h.h.Set(key, value)
}

// SetBytesKV sets the given 'Key: value' header (RFC 9110 Section 5.1).
//
// Thread-safe: No.
func (h *RequestHeader) SetBytesKV(key, value []byte) {
	h.bufK = append(h.bufK[:0], key...)
	NormalizeHeaderKey(h.bufK, h.disableNormalizing)
	h.SetCanonical(h.bufK, value)
}

// SetCanonical sets the given 'Key: value' header assuming that key is in canonical form (RFC 9110 Section 5.1).
//
// Thread-safe: No.
func (h *RequestHeader) SetCanonical(key, value []byte) {
	h.bufV = cookie.InitHeaderValueBytes(h.bufV, value)
	if h.setSpecialHeader(key, h.bufV) {
		return
	}

	h.setNonSpecial(key, h.bufV)
}

// Peek returns header value for the given key (RFC 9110 Section 5.1).
// The returned value is valid until the request is released. Do not store references.
//
// Thread-safe: No.
func (h *RequestHeader) Peek(key string) []byte {
	h.bufK = GetHeaderKeyBytes(h.bufK, key, h.disableNormalizing)
	return h.peek(h.bufK)
}

// PeekBytes returns header value for the given key bytes (RFC 9110 Section 5.1).
//
// Thread-safe: No.
func (h *RequestHeader) PeekBytes(key []byte) []byte {
	h.bufK = append(h.bufK[:0], key...)
	NormalizeHeaderKey(h.bufK, h.disableNormalizing)
	return h.peek(h.bufK)
}

func (h *RequestHeader) peek(key []byte) []byte {
	k := bytesconv.B2S(key)
	switch k {
	case headkit.Host:
		return h.Host()
	case headkit.ContentType:
		return h.ContentType()
	case headkit.UserAgent:
		return h.UserAgent()
	case headkit.Connection:
		if h.ConnectionClose() {
			return StrClose
		}

		return peekArgBytesHeaders(&h.h, key)

	case headkit.ContentLength:
		return h.contentLengthBytes
	case headkit.Cookie:
		if h.cookiesCollected {
			h.bufV = cookie.AppendRequestCookieBytes(h.bufV[:0], h.cookies)
			return h.bufV
		}

		return peekArgBytesHeaders(&h.h, key)

	case headkit.Trailer:
		h.bufV = appendTrailerBytes(h.bufV[:0], h.trailer, StrCommaSpace)
		return h.bufV
	default:
		return peekArgBytesHeaders(&h.h, key)
	}
}

// PeekAll returns all header values for the given key (RFC 9110 Section 5.2).
// The returned slice is valid until the request is released. Do not store references.
//
// Thread-safe: No.
func (h *RequestHeader) PeekAll(key string) [][]byte {
	h.bufK = GetHeaderKeyBytes(h.bufK, key, h.disableNormalizing)
	return h.peekAll(h.bufK)
}

func (h *RequestHeader) peekAll(key []byte) [][]byte {
	h.mulHeader = h.mulHeader[:0]
	switch bytesconv.B2S(key) {
	case headkit.Host:
		if host := h.Host(); len(host) > 0 {
			h.mulHeader = append(h.mulHeader, host)
		}
	case headkit.ContentType:
		if contentType := h.ContentType(); len(contentType) > 0 {
			h.mulHeader = append(h.mulHeader, contentType)
		}
	case headkit.UserAgent:
		if ua := h.UserAgent(); len(ua) > 0 {
			h.mulHeader = append(h.mulHeader, ua)
		}
	case headkit.Connection:
		if h.ConnectionClose() {
			h.mulHeader = append(h.mulHeader, StrClose)
			return h.mulHeader
		}

		h.mulHeader = peekAllArgBytesToDstHeaders(h.mulHeader, &h.h, key)

	case headkit.ContentLength:
		h.mulHeader = append(h.mulHeader, h.contentLengthBytes)
	case headkit.Cookie:
		if h.cookiesCollected {
			h.bufV = cookie.AppendRequestCookieBytes(h.bufV[:0], h.cookies)
			h.mulHeader = append(h.mulHeader, h.bufV)
			return h.mulHeader
		}

		h.mulHeader = peekAllArgBytesToDstHeaders(h.mulHeader, &h.h, key)

	case headkit.Trailer:
		h.bufV = appendTrailerBytes(h.bufV[:0], h.trailer, StrCommaSpace)
		h.mulHeader = append(h.mulHeader, h.bufV)
	default:
		h.mulHeader = peekAllArgBytesToDstHeaders(h.mulHeader, &h.h, key)
	}

	return h.mulHeader
}

// PeekKeys returns all header keys (RFC 9110 Section 5).
// The returned slice is valid until the request is released. Do not store references.
//
// Thread-safe: No.
func (h *RequestHeader) PeekKeys() [][]byte {
	h.mulHeader = h.mulHeader[:0]
	for key := range h.All() {
		h.mulHeader = append(h.mulHeader, key)
	}

	return h.mulHeader
}

// StatusCode returns response status code (RFC 9110 Section 15).
//
// Thread-safe: No.
func (h *ResponseHeader) StatusCode() int {
	if h.statusCode == 0 {
		return status.OK
	}

	return h.statusCode
}

// SetStatusCode sets response status code (RFC 9110 Section 15).
//
// Thread-safe: No.
func (h *ResponseHeader) SetStatusCode(statusCode int) {
	h.statusCode = statusCode
}

// StatusMessage returns response status reason phrase (RFC 9112 Section 3.1.2).
//
// Thread-safe: No.
func (h *ResponseHeader) StatusMessage() []byte {
	return h.statusMessage
}

// SetStatusMessage sets response status message bytes (RFC 9112 Section 3.1.2).
//
// Thread-safe: No.
func (h *ResponseHeader) SetStatusMessage(statusMessage []byte) {
	h.statusMessage = cookie.InitHeaderValueBytes(h.statusMessage, statusMessage)
}

// SetProtocol sets response protocol bytes (RFC 9112 Section 2.3).
//
// Thread-safe: No.
func (h *ResponseHeader) SetProtocol(protocol []byte) {
	h.protocol = cookie.InitHeaderValueBytes(h.protocol, protocol)
}

// SetLastModified sets 'Last-Modified' header to the given timestamp (RFC 9110 Section 8.8.2).
//
// Thread-safe: No.
func (h *ResponseHeader) SetLastModified(t time.Time) {
	h.bufV = AppendHTTPDate(h.bufV[:0], t)
	h.setNonSpecial(StrLastModified, h.bufV)
}

// ConnectionUpgrade returns true if 'Connection: Upgrade' header is set (RFC 9110 Section 7.8).
//
// Thread-safe: No.
func (h *ResponseHeader) ConnectionUpgrade() bool {
	return hasHeaderValue(h.Peek(headkit.Connection), StrUpgrade)
}

// ContentLength returns Content-Length header value as an integer (RFC 9110 Section 8.6, RFC 9112 Section 6.2).
//
// Negative value means that Content-Length header is not set.
// -1 means that Transfer-Encoding: chunked is used.
//
// Thread-safe: No.
func (h *ResponseHeader) ContentLength() int {
	return h.realContentLength()
}

// SetContentLength sets Content-Length header value (RFC 9110 Section 8.6, RFC 9112 Section 6.2).
//
// Negative value removes Content-Length header.
//
// Thread-safe: No.
func (h *ResponseHeader) SetContentLength(contentLength int) {
	if contentLength < 0 {
		h.contentLength = -2
		h.contentLengthBytes = h.contentLengthBytes[:0]
		return
	}

	h.contentLength = contentLength
	h.contentLengthBytes = AppendUint(h.contentLengthBytes[:0], contentLength)
}

func (h *ResponseHeader) realContentLength() int {
	if h.contentLength >= 0 {
		return h.contentLength
	}

	if h.mustSkipContentLength() {
		return 0
	}

	return h.contentLength
}

func (h *ResponseHeader) mustSkipContentLength() bool {
	statusCode := h.StatusCode()
	if statusCode < 100 || statusCode == status.OK {
		return false
	}

	return statusCode == status.NotModified || statusCode == status.NoContent || statusCode < 200
}

func (h *ResponseHeader) isCompressibleContentType() bool {
	contentType := h.ContentType()

	return bytes.HasPrefix(contentType, StrTextSlash) ||
		bytes.HasPrefix(contentType, StrApplicationSlash) ||
		bytes.HasPrefix(contentType, StrImageSVG) ||
		bytes.HasPrefix(contentType, StrImageIcon) ||
		bytes.HasPrefix(contentType, StrFontSlash) ||
		bytes.HasPrefix(contentType, StrMultipartSlash)
}

// ContentType returns Content-Type header value (RFC 9110 Section 8.3).
//
// Thread-safe: No.
func (h *ResponseHeader) ContentType() []byte {
	contentType := h.contentType
	if !h.noDefaultContentType && len(h.contentType) == 0 {
		contentType = DefaultContentType
	}

	return contentType
}

// ContentEncoding returns Content-Encoding header value (RFC 9110 Section 8.4).
//
// Thread-safe: No.
func (h *ResponseHeader) ContentEncoding() []byte {
	return h.contentEncoding
}

// SetContentEncoding sets Content-Encoding header value (RFC 9110 Section 8.4).
//
// Thread-safe: No.
func (h *ResponseHeader) SetContentEncoding(contentEncoding string) {
	h.contentEncoding = cookie.InitHeaderValueString(h.contentEncoding, contentEncoding)
}

// SetContentEncodingBytes sets Content-Encoding header value (RFC 9110 Section 8.4).
//
// Thread-safe: No.
func (h *ResponseHeader) SetContentEncodingBytes(contentEncoding []byte) {
	h.contentEncoding = cookie.InitHeaderValueBytes(h.contentEncoding, contentEncoding)
}

// addVaryBytes adds value to the 'Vary' header if it's not included (RFC 9110 Section 12.5.5).
func (h *ResponseHeader) addVaryBytes(value []byte) {
	v := h.peek(StrVary)
	if len(v) == 0 {
		h.SetBytesKV(StrVary, value)
		return
	}

	if !bytes.Contains(v, value) {
		h.bufV = append(h.bufV[:0], v...)
		h.bufV = append(h.bufV, ',')
		h.bufV = append(h.bufV, value...)
		h.SetBytesKV(StrVary, h.bufV)
	}
}

// Server returns Server header value (RFC 9110 Section 10.2.4).
//
// Thread-safe: No.
func (h *ResponseHeader) Server() []byte {
	return h.server
}

// AltSvc returns parsed Alt-Svc services using foundation/net/http/altsvc (RFC 7838).
//
// Thread-safe: No.
func (h *ResponseHeader) AltSvc() []altsvc.Service {
	altSvcHeader := h.peek(strAltSvc)
	if len(altSvcHeader) == 0 {
		return nil
	}

	return altsvc.Parse(altSvcHeader)
}

// SetServer sets Server header value (RFC 9110 Section 10.2.4).
//
// Thread-safe: No.
func (h *ResponseHeader) SetServer(server string) {
	h.server = cookie.InitHeaderValueString(h.server, server)
}

// SetServerBytes sets Server header value (RFC 9110 Section 10.2.4).
//
// Thread-safe: No.
func (h *ResponseHeader) SetServerBytes(server []byte) {
	h.server = cookie.InitHeaderValueBytes(h.server, server)
}

// Len returns the number of headers set.
//
// Thread-safe: No.
func (h *ResponseHeader) Len() int {
	n := 0
	for range h.All() {
		n++
	}

	return n
}

// All returns an iterator over key-value pairs in h (RFC 9110 Section 5).
// The key and value may be invalid outside the iteration loop.
//
// Modifying ResponseHeader during iteration causes undefined behavior.
//
// Thread-safe: No.
func (h *ResponseHeader) All() iter.Seq2[[]byte, []byte] {
	return func(yield func([]byte, []byte) bool) {
		for k, v := range h.h.All() {
			if !yield(bytesconv.S2B(k), bytesconv.S2B(v)) {
				return
			}
		}

		if len(h.contentType) > 0 {
			if !yield(StrContentType, h.contentType) {
				return
			}
		}

		if len(h.contentEncoding) > 0 {
			if !yield(StrContentEncoding, h.contentEncoding) {
				return
			}
		}

		if len(h.server) > 0 {
			if !yield(StrServer, h.server) {
				return
			}
		}

		if len(h.contentLengthBytes) > 0 {
			if !yield(StrContentLength, h.contentLengthBytes) {
				return
			}
		}

		if h.connectionClose {
			if !yield(StrConnection, StrClose) {
				return
			}
		}

		if len(h.cookies) > 0 {
			for i := range h.cookies {
				if !yield(StrSetCookie, h.cookies[i].Value) {
					return
				}
			}
		}

		if len(h.trailer) > 0 {
			h.bufV = appendTrailerBytes(h.bufV[:0], h.trailer, StrCommaSpace)
			if !yield(StrTrailer, h.bufV) {
				return
			}
		}
	}
}

// Del deletes header with the given key (RFC 9110 Section 5).
//
// Thread-safe: No.
func (h *ResponseHeader) Del(key string) {
	switch key {
	case headkit.ContentType:
		h.contentType = h.contentType[:0]
	case headkit.ContentEncoding:
		h.contentEncoding = h.contentEncoding[:0]
	case headkit.Server:
		h.server = h.server[:0]
	case headkit.SetCookie:
		h.cookies = h.cookies[:0]
	case headkit.ContentLength:
		h.contentLength = 0
		h.contentLengthBytes = h.contentLengthBytes[:0]
	case headkit.Connection:
		h.connectionClose = false
	case headkit.Trailer:
		h.trailer = h.trailer[:0]
	}

	h.h.Del(key)
}

// DelBytes deletes header with the given key bytes (RFC 9110 Section 5).
//
// Thread-safe: No.
func (h *ResponseHeader) DelBytes(key []byte) {
	h.bufK = append(h.bufK[:0], key...)
	NormalizeHeaderKey(h.bufK, h.disableNormalizing)
	h.del(h.bufK)
}

func (h *ResponseHeader) del(key []byte) {
	k := bytesconv.B2S(key)
	switch k {
	case headkit.ContentType:
		h.contentType = h.contentType[:0]
	case headkit.ContentEncoding:
		h.contentEncoding = h.contentEncoding[:0]
	case headkit.Server:
		h.server = h.server[:0]
	case headkit.SetCookie:
		h.cookies = h.cookies[:0]
	case headkit.ContentLength:
		h.contentLength = 0
		h.contentLengthBytes = h.contentLengthBytes[:0]
	case headkit.Connection:
		h.connectionClose = false
	case headkit.Trailer:
		h.trailer = h.trailer[:0]
	}

	h.h.Del(k)
}

func (h *ResponseHeader) setSpecialHeader(key, value []byte) bool {
	if len(key) == 0 {
		return false
	}

	switch key[0] | 0x20 {
	case 'c':
		switch {
		case CaseInsensitiveCompare(StrContentType, key):
			h.SetContentTypeBytes(value)
			return true
		case CaseInsensitiveCompare(StrContentLength, key):
			if contentLength, err := parseContentLength(value); err == nil {
				h.contentLength = contentLength
				h.contentLengthBytes = append(h.contentLengthBytes[:0], value...)
			}

			return true

		case CaseInsensitiveCompare(StrContentEncoding, key):
			h.SetContentEncodingBytes(value)
			return true
		case CaseInsensitiveCompare(StrConnection, key):
			if hasHeaderValue(value, StrClose) {
				h.SetConnectionClose()
				return true
			}

			h.ResetConnectionClose()
			h.setNonSpecial(key, value)
			return true
		}

	case 's':
		if CaseInsensitiveCompare(StrServer, key) {
			h.SetServerBytes(value)
			return true
		}
		if CaseInsensitiveCompare(StrSetCookie, key) {
			var kv *uri.ArgsKV

			h.cookies, kv = uri.AllocArg(h.cookies)
			kv.Key = cookie.GetCookieKey(kv.Key, value)
			kv.Value = append(kv.Value[:0], value...)

			return true
		}

	case 't':
		if CaseInsensitiveCompare(StrTransferEncoding, key) {
			return true
		} else if CaseInsensitiveCompare(StrTrailer, key) {
			_ = h.SetTrailerBytes(value)
			return true
		}

	case 'd':
		if CaseInsensitiveCompare(StrDate, key) {
			return true
		}
	}

	return false
}

// Add adds the given 'Key: value' header (RFC 9110 Section 5.2).
// Multiple headers with the same key may be added with this function.
//
// Thread-safe: No.
func (h *ResponseHeader) Add(key, value string) {
	h.bufK, h.bufV = InitHeaderKV(h.bufK, h.bufV, key, value, h.disableNormalizing)
	if h.setSpecialHeader(h.bufK, h.bufV) {
		return
	}

	h.h.Add(key, value)
}


// AddBytesKV adds the given 'Key: value' header (RFC 9110 Section 5.2).
//
// Thread-safe: No.
func (h *ResponseHeader) AddBytesKV(key, value []byte) {
	h.bufK, h.bufV = InitHeaderKV(
		h.bufK,
		h.bufV,
		bytesconv.B2S(key),
		bytesconv.B2S(value),
		h.disableNormalizing,
	)
	if h.setSpecialHeader(h.bufK, h.bufV) {
		return
	}

	appendArgBytesHeaders(&h.h, h.bufK, h.bufV)
}

// Set sets the given 'Key: value' header (RFC 9110 Section 5.1).
// Use Add for setting multiple header values under the same key.
//
// Thread-safe: No.
func (h *ResponseHeader) Set(key, value string) {
	h.bufK, h.bufV = InitHeaderKV(h.bufK, h.bufV, key, value, h.disableNormalizing)
	if h.setSpecialHeader(h.bufK, h.bufV) {
		return
	}

	h.h.Set(key, value)
}

// SetBytesKV sets the given 'Key: value' header (RFC 9110 Section 5.1).
//
// Thread-safe: No.
func (h *ResponseHeader) SetBytesKV(key, value []byte) {
	h.bufK = append(h.bufK[:0], key...)
	NormalizeHeaderKey(h.bufK, h.disableNormalizing)
	h.SetCanonical(h.bufK, value)
}

// SetCanonical sets the given 'Key: value' header assuming that key is in canonical form (RFC 9110 Section 5.1).
//
// Thread-safe: No.
func (h *ResponseHeader) SetCanonical(key, value []byte) {
	h.bufV = cookie.InitHeaderValueBytes(h.bufV, value)
	if h.setSpecialHeader(key, h.bufV) {
		return
	}

	h.setNonSpecial(key, h.bufV)
}

// Peek returns header value for the given key (RFC 9110 Section 5.1).
// The returned value is valid until the response is released. Do not store references.
//
// Thread-safe: No.
func (h *ResponseHeader) Peek(key string) []byte {
	h.bufK = GetHeaderKeyBytes(h.bufK, key, h.disableNormalizing)
	return h.peek(h.bufK)
}

// PeekBytes returns header value for the given key bytes (RFC 9110 Section 5.1).
//
// Thread-safe: No.
func (h *ResponseHeader) PeekBytes(key []byte) []byte {
	h.bufK = append(h.bufK[:0], key...)
	NormalizeHeaderKey(h.bufK, h.disableNormalizing)
	return h.peek(h.bufK)
}

func (h *ResponseHeader) peek(key []byte) []byte {
	k := bytesconv.B2S(key)
	switch k {
	case headkit.ContentType:
		return h.ContentType()
	case headkit.ContentEncoding:
		return h.ContentEncoding()
	case headkit.Server:
		return h.Server()
	case headkit.Connection:
		if h.ConnectionClose() {
			return StrClose
		}

		return peekArgBytesHeaders(&h.h, key)

	case headkit.ContentLength:
		return h.contentLengthBytes
	case headkit.SetCookie:
		h.bufV = cookie.AppendResponseCookieBytes(h.bufV[:0], h.cookies)
		return h.bufV
	case headkit.Trailer:
		h.bufV = appendTrailerBytes(h.bufV[:0], h.trailer, StrCommaSpace)
		return h.bufV
	default:
		return peekArgBytesHeaders(&h.h, key)
	}
}

// PeekAll returns all header values for the given key (RFC 9110 Section 5.2).
// The returned slice is valid until the response is released. Do not store references.
//
// Thread-safe: No.
func (h *ResponseHeader) PeekAll(key string) [][]byte {
	h.bufK = GetHeaderKeyBytes(h.bufK, key, h.disableNormalizing)
	return h.peekAll(h.bufK)
}

func (h *ResponseHeader) peekAll(key []byte) [][]byte {
	h.mulHeader = h.mulHeader[:0]
	switch bytesconv.B2S(key) {
	case headkit.ContentType:
		if contentType := h.ContentType(); len(contentType) > 0 {
			h.mulHeader = append(h.mulHeader, contentType)
		}
	case headkit.ContentEncoding:
		if contentEncoding := h.ContentEncoding(); len(contentEncoding) > 0 {
			h.mulHeader = append(h.mulHeader, contentEncoding)
		}
	case headkit.Server:
		if server := h.Server(); len(server) > 0 {
			h.mulHeader = append(h.mulHeader, server)
		}
	case headkit.Connection:
		if h.ConnectionClose() {
			h.mulHeader = append(h.mulHeader, StrClose)
			return h.mulHeader
		}

		h.mulHeader = peekAllArgBytesToDstHeaders(h.mulHeader, &h.h, key)

	case headkit.ContentLength:
		h.mulHeader = append(h.mulHeader, h.contentLengthBytes)
	case headkit.SetCookie:
		h.bufV = cookie.AppendResponseCookieBytes(h.bufV[:0], h.cookies)
		h.mulHeader = append(h.mulHeader, h.bufV)
	case headkit.Trailer:
		h.bufV = appendTrailerBytes(h.bufV[:0], h.trailer, StrCommaSpace)
		h.mulHeader = append(h.mulHeader, h.bufV)
	default:
		h.mulHeader = peekAllArgBytesToDstHeaders(h.mulHeader, &h.h, key)
	}

	return h.mulHeader
}

// PeekKeys returns all header keys (RFC 9110 Section 5).
// The returned slice is valid until the response is released. Do not store references.
//
// Thread-safe: No.
func (h *ResponseHeader) PeekKeys() [][]byte {
	h.mulHeader = h.mulHeader[:0]
	for key := range h.All() {
		h.mulHeader = append(h.mulHeader, key)
	}

	return h.mulHeader
}

// SetDisableNormalizing controls whether header key normalization is disabled for this ResponseHeader.
//
// Thread-safe: No.
func (h *ResponseHeader) SetDisableNormalizing(disable bool) {
	h.disableNormalizing = disable
}

func updateServerDate() {
	refreshServerDate()
	go func() {
		for {
			time.Sleep(time.Second)
			refreshServerDate()
		}
	}()
}

var (
	serverDate     atomic.Pointer[[]byte]
	serverDateOnce sync.Once
)

func refreshServerDate() {
	b := AppendHTTPDate(nil, time.Now())
	serverDate.Store(&b)
}

func appendHeaderLine(dst, key, value []byte) []byte {
	dst = append(dst, key...)
	dst = append(dst, StrColonSpace...)

	if bytes.IndexByte(value, '\n') < 0 && bytes.IndexByte(value, '\r') < 0 {
		dst = append(dst, value...)
		return append(dst, StrCRLF...)
	}

	for _, c := range value {
		if c == '\n' || c == '\r' {
			dst = append(dst, ' ')
			continue
		}
		dst = append(dst, c)
	}

	return append(dst, StrCRLF...)
}

func hasHeaderValue(s, value []byte) bool {
	for len(s) > 0 {
		var before []byte
		idx := bytes.IndexByte(s, ',')
		if idx >= 0 {
			before = s[:idx]
			s = s[idx+1:]
		} else {
			before = s
			s = nil
		}
		if CaseInsensitiveCompare(bytes.Trim(before, " \t"), value) {
			return true
		}
	}

	return false
}


// peekArgBytesHeaders returns the header value matching key from h.
func peekArgBytesHeaders(h *headkit.Headers, key []byte) []byte {
	v := h.Get(bytesconv.B2S(key))
	if v == "" {
		return nil
	}

	return bytesconv.S2B(v)
}

func bytesHeaderKVToString(key, value []byte) (string, string) {
	kStr := headkit.InternCanonicalKey(key)
	if kStr == "" {
		if ik := headkit.InternKey(key); ik != nil {
			kStr = bytesconv.B2S(ik)
		} else {
			kStr = string(key)
		}
	}

	vStr := headkit.InternValueString(value)
	if vStr == "" {
		if iv := headkit.InternValue(value); iv != nil {
			vStr = bytesconv.B2S(iv)
		} else {
			vStr = string(value)
		}
	}

	return kStr, vStr
}

// setArgBytesHeaders sets the header value for key in h.
func setArgBytesHeaders(h *headkit.Headers, key, value []byte) {
	k, v := bytesHeaderKVToString(key, value)
	h.Set(k, v)
}

// appendArgBytesHeaders appends a header key-value entry to h.
func appendArgBytesHeaders(h *headkit.Headers, key, value []byte) {
	k, v := bytesHeaderKVToString(key, value)
	h.Add(k, v)
}

// copyHeaders copies all header entries from src to dst with zero heap allocations.
func copyHeaders(dst, src *headkit.Headers) {
	dst.Reset()

	for k, v := range src.All() {
		dst.Add(k, v)
	}
}

// peekAllArgBytesToDstHeaders appends all values matching key in h to dst without allocating intermediate slices.
func peekAllArgBytesToDstHeaders(dst [][]byte, h *headkit.Headers, key []byte) [][]byte {
	kStr := bytesconv.B2S(key)
	for k, v := range h.All() {
		if bytesconv.EqualFoldASCII(k, kStr) {
			dst = append(dst, bytesconv.S2B(v))
		}
	}

	return dst
}
