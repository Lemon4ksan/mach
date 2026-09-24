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
	"github.com/lemon4ksan/mach/proto/http/altsvc"
	"github.com/lemon4ksan/mach/proto/http/status"
	"github.com/lemon4ksan/mach/proto/http/zerocopy"
)

// SetByteRange sets 'Range: bytes=startPos-endPos' header (RFC 9110 Section 14.1.2).
//
//   - If startPos is negative, then 'bytes=-startPos' value is set.
//   - If endPos is negative, then 'bytes=startPos-' value is set.
//
// Thread-safe: No.
func (h *RequestHeader) SetByteRange(startPos, endPos int) {
	b := h.bufV[:0]
	b = append(b, zerocopy.StrBytes...)

	b = append(b, '=')
	if startPos >= 0 {
		b = zerocopy.AppendUint(b, startPos)
	} else {
		endPos = -startPos
	}

	b = append(b, '-')
	if endPos >= 0 {
		b = zerocopy.AppendUint(b, endPos)
	}

	h.bufV = b
	h.setNonSpecial(zerocopy.StrRange, h.bufV)
}

// ConnectionUpgrade returns true if 'Connection: Upgrade' header is set (RFC 9110 Section 7.8).
//
// Thread-safe: No.
func (h *RequestHeader) ConnectionUpgrade() bool {
	return hasHeaderValue(h.Peek(HeaderConnection), zerocopy.StrUpgrade)
}

// ContentLength returns Content-Length header value as an integer (RFC 9110 Section 8.6, RFC 9112 Section 6.2).
//
// Negative value means that Content-Length header is not set.
// -1 means that Transfer-Encoding: chunked is used.
//
// Thread-safe: No.
func (h *RequestHeader) ContentLength() int {
	if h.contentLength >= 0 {
		return h.contentLength
	}

	return h.realContentLength()
}

func (h *RequestHeader) realContentLength() int {
	if h.contentLength >= 0 {
		return h.contentLength
	}

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
	h.contentLengthBytes = zerocopy.AppendUint(h.contentLengthBytes[:0], contentLength)
}

// ContentType returns Content-Type header value (RFC 9110 Section 8.3).
//
// Thread-safe: No.
func (h *RequestHeader) ContentType() []byte {
	if h.disableSpecialHeader {
		return peekArgBytesHeaders(&h.h, []byte(HeaderContentType))
	}

	return h.contentType
}

// ContentEncoding returns Content-Encoding header value (RFC 9110 Section 8.4).
//
// Thread-safe: No.
func (h *RequestHeader) ContentEncoding() []byte {
	return peekArgBytesHeaders(&h.h, zerocopy.StrContentEncoding)
}

// SetContentEncoding sets Content-Encoding header value (RFC 9110 Section 8.4).
//
// Thread-safe: No.
func (h *RequestHeader) SetContentEncoding(contentEncoding string) {
	h.setNonSpecial(zerocopy.StrContentEncoding, bytesconv.S2B(contentEncoding))
}

// SetContentEncodingBytes sets Content-Encoding header value (RFC 9110 Section 8.4).
//
// Thread-safe: No.
func (h *RequestHeader) SetContentEncodingBytes(contentEncoding []byte) {
	h.setNonSpecial(zerocopy.StrContentEncoding, contentEncoding)
}

// SetMultipartFormBoundary sets the following Content-Type:
// 'multipart/form-data; boundary=...' (RFC 7578 Section 4.1).
//
// Thread-safe: No.
func (h *RequestHeader) SetMultipartFormBoundary(boundary string) {
	b := h.bufV[:0]
	b = append(b, zerocopy.StrMultipartFormData...)
	b = append(b, ';', ' ')
	b = append(b, zerocopy.StrBoundary...)
	b = append(b, '=')
	b = append(b, boundary...)
	h.bufV = b
	h.SetContentTypeBytes(h.bufV)
}

// SetMultipartFormBoundaryBytes sets the following Content-Type:
// 'multipart/form-data; boundary=...' (RFC 7578 Section 4.1).
//
// Thread-safe: No.
func (h *RequestHeader) SetMultipartFormBoundaryBytes(boundary []byte) {
	b := h.bufV[:0]
	b = append(b, zerocopy.StrMultipartFormData...)
	b = append(b, ';', ' ')
	b = append(b, zerocopy.StrBoundary...)
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
	if !bytes.HasPrefix(b, zerocopy.StrMultipartFormData) {
		return nil
	}

	b = b[len(zerocopy.StrMultipartFormData):]
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
		if !bytes.HasPrefix(b, zerocopy.StrBoundary) {
			if n = bytes.IndexByte(b, ';'); n < 0 {
				return nil
			}

			continue
		}

		b = b[len(zerocopy.StrBoundary):]
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
		return peekArgBytesHeaders(&h.h, []byte(HeaderHost))
	}

	return h.host
}

// SetHost sets Host header value (RFC 9112 Section 7.2, RFC 9110 Section 7.2).
//
// Thread-safe: No.
func (h *RequestHeader) SetHost(host string) {
	h.host = zerocopy.InitHeaderValueString(h.host, host)
}

// SetHostBytes sets Host header value (RFC 9112 Section 7.2, RFC 9110 Section 7.2).
//
// Thread-safe: No.
func (h *RequestHeader) SetHostBytes(host []byte) {
	h.host = zerocopy.InitHeaderValueBytes(h.host, host)
}

// UserAgent returns User-Agent header value (RFC 9110 Section 10.1.5).
//
// Thread-safe: No.
func (h *RequestHeader) UserAgent() []byte {
	if h.disableSpecialHeader {
		return peekArgBytesHeaders(&h.h, []byte(HeaderUserAgent))
	}

	return h.userAgent
}

// SetUserAgent sets User-Agent header value (RFC 9110 Section 10.1.5).
//
// Thread-safe: No.
func (h *RequestHeader) SetUserAgent(userAgent string) {
	h.userAgent = zerocopy.InitHeaderValueString(h.userAgent, userAgent)
}

// SetUserAgentBytes sets User-Agent header value (RFC 9110 Section 10.1.5).
//
// Thread-safe: No.
func (h *RequestHeader) SetUserAgentBytes(userAgent []byte) {
	h.userAgent = zerocopy.InitHeaderValueBytes(h.userAgent, userAgent)
}

// Referer returns Referer header value (RFC 9110 Section 10.1.3).
//
// Thread-safe: No.
func (h *RequestHeader) Referer() []byte {
	return peekArgBytesHeaders(&h.h, zerocopy.StrReferer)
}

// SetReferer sets Referer header value (RFC 9110 Section 10.1.3).
//
// Thread-safe: No.
func (h *RequestHeader) SetReferer(referer string) {
	h.setNonSpecial(zerocopy.StrReferer, bytesconv.S2B(referer))
}

// SetRefererBytes sets Referer header value (RFC 9110 Section 10.1.3).
//
// Thread-safe: No.
func (h *RequestHeader) SetRefererBytes(referer []byte) {
	h.setNonSpecial(zerocopy.StrReferer, referer)
}

// Method returns HTTP request method (RFC 9110 Section 9).
//
// Thread-safe: No.
func (h *RequestHeader) Method() []byte {
	if len(h.method) == 0 {
		return []byte(MethodGet)
	}

	return h.method
}

// SetMethod sets HTTP request method (RFC 9110 Section 9).
//
// Thread-safe: No.
func (h *RequestHeader) SetMethod(method string) {
	h.method = zerocopy.InitHeaderValueString(h.method, method)
}

// SetMethodBytes sets HTTP request method (RFC 9110 Section 9).
//
// Thread-safe: No.
func (h *RequestHeader) SetMethodBytes(method []byte) {
	h.method = zerocopy.InitHeaderValueBytes(h.method, method)
}

// SetProtocol sets HTTP request protocol (RFC 9112 Section 2.3).
//
// Thread-safe: No.
func (h *RequestHeader) SetProtocol(protocol string) {
	h.protocol = zerocopy.InitHeaderValueString(h.protocol, protocol)
	h.noHTTP11 = !bytes.Equal(h.protocol, zerocopy.StrHTTP11)
}

// SetProtocolBytes sets HTTP request protocol (RFC 9112 Section 2.3).
//
// Thread-safe: No.
func (h *RequestHeader) SetProtocolBytes(protocol []byte) {
	h.protocol = zerocopy.InitHeaderValueBytes(h.protocol, protocol)
	h.noHTTP11 = !bytes.Equal(h.protocol, zerocopy.StrHTTP11)
}

// RequestURI returns RequestURI from the first HTTP request line (RFC 9112 Section 3.2).
//
// Thread-safe: No.
func (h *RequestHeader) RequestURI() []byte {
	requestURI := h.requestURI
	if len(requestURI) == 0 {
		requestURI = zerocopy.StrSlash
	}

	return requestURI
}

// SetRequestURI sets RequestURI for the first HTTP request line (RFC 9112 Section 3.2).
//
// Thread-safe: No.
func (h *RequestHeader) SetRequestURI(requestURI string) {
	h.requestURI = zerocopy.InitHeaderValueString(h.requestURI, requestURI)
}

// SetRequestURIBytes sets RequestURI for the first HTTP request line (RFC 9112 Section 3.2).
//
// Thread-safe: No.
func (h *RequestHeader) SetRequestURIBytes(requestURI []byte) {
	h.requestURI = zerocopy.InitHeaderValueBytes(h.requestURI, requestURI)
}

// IsGet returns true if request method is GET (RFC 9110 Section 9.3.1).
func (h *RequestHeader) IsGet() bool {
	return string(h.Method()) == MethodGet
}

// IsPost returns true if request method is POST (RFC 9110 Section 9.3.3).
func (h *RequestHeader) IsPost() bool {
	return string(h.Method()) == MethodPost
}

// IsPut returns true if request method is PUT (RFC 9110 Section 9.3.4).
func (h *RequestHeader) IsPut() bool {
	return string(h.Method()) == MethodPut
}

// IsHead returns true if request method is HEAD (RFC 9110 Section 9.3.2).
func (h *RequestHeader) IsHead() bool {
	return string(h.Method()) == MethodHead
}

// IsDelete returns true if request method is DELETE (RFC 9110 Section 9.3.5).
func (h *RequestHeader) IsDelete() bool {
	return string(h.Method()) == MethodDelete
}

// IsConnect returns true if request method is CONNECT (RFC 9110 Section 9.3.6).
func (h *RequestHeader) IsConnect() bool {
	return string(h.Method()) == MethodConnect
}

// IsOptions returns true if request method is OPTIONS (RFC 9110 Section 9.3.7).
func (h *RequestHeader) IsOptions() bool {
	return string(h.Method()) == MethodOptions
}

// IsTrace returns true if request method is TRACE (RFC 9110 Section 9.3.8).
func (h *RequestHeader) IsTrace() bool {
	return string(h.Method()) == MethodTrace
}

// IsPatch returns true if request method is PATCH (RFC 5789 Section 2).
func (h *RequestHeader) IsPatch() bool {
	return string(h.Method()) == MethodPatch
}

// HasAcceptEncoding returns true if the header contains the given Accept-Encoding value (RFC 9110 Section 12.5.3).
//
// Thread-safe: No.
func (h *RequestHeader) HasAcceptEncoding(acceptEncoding string) bool {
	h.bufV = append(h.bufV[:0], acceptEncoding...)
	return h.HasAcceptEncodingBytes(h.bufV)
}

// HasAcceptEncodingBytes returns true if the header contains the given Accept-Encoding value (RFC 9110 Section 12.5.3).
//
// Thread-safe: No.
func (h *RequestHeader) HasAcceptEncodingBytes(acceptEncoding []byte) bool {
	ae := h.peek(zerocopy.StrAcceptEncoding)

	n := bytes.Index(ae, acceptEncoding)
	if n < 0 {
		return false
	}

	b := ae[n+len(acceptEncoding):]
	if len(b) > 0 && b[0] != ',' {
		return false
	}

	if n == 0 {
		return true
	}

	return ae[n-1] == ' '
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
		for _, e := range h.h.Entries() {
			if !yield(bytesconv.S2B(e.Key), bytesconv.S2B(e.Value)) {
				return
			}
		}

		if len(h.host) > 0 {
			if !yield(zerocopy.StrHost, h.host) {
				return
			}
		}

		if len(h.contentType) > 0 {
			if !yield(zerocopy.StrContentType, h.contentType) {
				return
			}
		}

		if len(h.userAgent) > 0 {
			if !yield(zerocopy.StrUserAgent, h.userAgent) {
				return
			}
		}

		if h.contentLength >= 0 {
			if !yield(zerocopy.StrContentLength, h.contentLengthBytes) {
				return
			}
		}

		if h.connectionClose {
			if !yield(zerocopy.StrConnection, zerocopy.StrClose) {
				return
			}
		}

		if len(h.cookies) > 0 {
			for i := range h.cookies {
				if !yield(zerocopy.StrCookie, h.cookies[i].Value) {
					return
				}
			}
		}

		if len(h.trailer) > 0 {
			if !yield(zerocopy.StrTrailer, appendTrailerBytes(nil, h.trailer, zerocopy.StrCommaSpace)) {
				return
			}
		}
	}
}

// AllInOrder returns an iterator over key-value pairs in h in the order they were parsed or added.
//
// Modifying RequestHeader during iteration causes undefined behavior.
//
// Thread-safe: No.
func (h *RequestHeader) AllInOrder() iter.Seq2[[]byte, []byte] {
	return func(yield func([]byte, []byte) bool) {
		for _, e := range h.h.Entries() {
			if !yield(bytesconv.S2B(e.Key), bytesconv.S2B(e.Value)) {
				return
			}
		}
	}
}

// Del deletes header with the given key (RFC 9110 Section 5).
//
// Thread-safe: No.
func (h *RequestHeader) Del(key string) {
	h.bufK = zerocopy.GetHeaderKeyBytes(h.bufK, key, h.disableNormalizing)
	h.del(h.bufK)
}

// DelBytes deletes header with the given key bytes (RFC 9110 Section 5).
//
// Thread-safe: No.
func (h *RequestHeader) DelBytes(key []byte) {
	h.bufK = append(h.bufK[:0], key...)
	zerocopy.NormalizeHeaderKey(h.bufK, h.disableNormalizing)
	h.del(h.bufK)
}

func (h *RequestHeader) del(key []byte) {
	switch string(key) {
	case HeaderHost:
		h.host = h.host[:0]
	case HeaderContentType:
		h.contentType = h.contentType[:0]
	case HeaderUserAgent:
		h.userAgent = h.userAgent[:0]
	case HeaderCookie:
		h.cookies = h.cookies[:0]
	case HeaderContentLength:
		h.contentLength = 0
		h.contentLengthBytes = h.contentLengthBytes[:0]
	case HeaderConnection:
		h.connectionClose = false
	case HeaderTrailer:
		h.trailer = h.trailer[:0]
	}

	h.h.Del(bytesconv.B2S(key))
}

func (h *RequestHeader) setSpecialHeader(key, value []byte) bool {
	if len(key) == 0 || h.disableSpecialHeader {
		return false
	}

	switch key[0] | 0x20 {
	case 'c':
		switch {
		case zerocopy.CaseInsensitiveCompare(zerocopy.StrContentType, key):
			h.SetContentTypeBytes(value)
			return true
		case zerocopy.CaseInsensitiveCompare(zerocopy.StrContentLength, key):
			if contentLength, err := parseContentLength(value); err == nil {
				h.contentLength = contentLength
				h.contentLengthBytes = append(h.contentLengthBytes[:0], value...)
			}

			return true

		case zerocopy.CaseInsensitiveCompare(zerocopy.StrConnection, key):
			if hasHeaderValue(value, zerocopy.StrClose) {
				h.SetConnectionClose()
			} else {
				h.ResetConnectionClose()
				h.setNonSpecial(key, value)
			}

			return true

		case zerocopy.CaseInsensitiveCompare(zerocopy.StrCookie, key):
			h.collectCookies()
			h.cookies = zerocopy.ParseRequestCookies(h.cookies, value)
			return true
		}

	case 't':
		if zerocopy.CaseInsensitiveCompare(zerocopy.StrTransferEncoding, key) {
			return true
		} else if zerocopy.CaseInsensitiveCompare(zerocopy.StrTrailer, key) {
			_ = h.SetTrailerBytes(value)
			return true
		}

	case 'h':
		if zerocopy.CaseInsensitiveCompare(zerocopy.StrHost, key) {
			h.SetHostBytes(value)
			return true
		}
	case 'u':
		if zerocopy.CaseInsensitiveCompare(zerocopy.StrUserAgent, key) {
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
	h.AddBytesKV(bytesconv.S2B(key), bytesconv.S2B(value))
}

// AddBytesK adds the given 'Key: value' header (RFC 9110 Section 5.2).
//
// Thread-safe: No.
func (h *RequestHeader) AddBytesK(key []byte, value string) {
	h.AddBytesKV(key, bytesconv.S2B(value))
}

// AddBytesV adds the given 'Key: value' header (RFC 9110 Section 5.2).
//
// Thread-safe: No.
func (h *RequestHeader) AddBytesV(key string, value []byte) {
	h.AddBytesKV(bytesconv.S2B(key), value)
}

// AddBytesKV adds the given 'Key: value' header (RFC 9110 Section 5.2).
//
// Thread-safe: No.
func (h *RequestHeader) AddBytesKV(key, value []byte) {
	h.bufK, h.bufV = zerocopy.InitHeaderKV(
		h.bufK,
		h.bufV,
		bytesconv.B2S(key),
		bytesconv.B2S(value),
		h.disableNormalizing,
	)
	if h.setSpecialHeader(h.bufK, h.bufV) {
		return
	}

	appendArgBytesHeaders(&h.h, h.bufK, h.bufV, zerocopy.ArgsHasValue)
}

// Set sets the given 'Key: value' header (RFC 9110 Section 5.1).
// Use Add for setting multiple header values under the same key.
//
// Thread-safe: No.
func (h *RequestHeader) Set(key, value string) {
	h.bufK, h.bufV = zerocopy.InitHeaderKV(h.bufK, h.bufV, key, value, h.disableNormalizing)
	h.SetCanonical(h.bufK, h.bufV)
}

// SetBytesK sets the given 'Key: value' header (RFC 9110 Section 5.1).
//
// Thread-safe: No.
func (h *RequestHeader) SetBytesK(key []byte, value string) {
	h.bufV = append(h.bufV[:0], value...)
	h.SetBytesKV(key, h.bufV)
}

// SetBytesV sets the given 'Key: value' header (RFC 9110 Section 5.1).
//
// Thread-safe: No.
func (h *RequestHeader) SetBytesV(key string, value []byte) {
	h.bufK = zerocopy.GetHeaderKeyBytes(h.bufK, key, h.disableNormalizing)
	h.SetCanonical(h.bufK, value)
}

// SetBytesKV sets the given 'Key: value' header (RFC 9110 Section 5.1).
//
// Thread-safe: No.
func (h *RequestHeader) SetBytesKV(key, value []byte) {
	h.bufK = append(h.bufK[:0], key...)
	zerocopy.NormalizeHeaderKey(h.bufK, h.disableNormalizing)
	h.SetCanonical(h.bufK, value)
}

// SetCanonical sets the given 'Key: value' header assuming that key is in canonical form (RFC 9110 Section 5.1).
//
// Thread-safe: No.
func (h *RequestHeader) SetCanonical(key, value []byte) {
	h.bufV = zerocopy.InitHeaderValueBytes(h.bufV, value)
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
	h.bufK = zerocopy.GetHeaderKeyBytes(h.bufK, key, h.disableNormalizing)
	return h.peek(h.bufK)
}

// PeekBytes returns header value for the given key bytes (RFC 9110 Section 5.1).
//
// Thread-safe: No.
func (h *RequestHeader) PeekBytes(key []byte) []byte {
	h.bufK = append(h.bufK[:0], key...)
	zerocopy.NormalizeHeaderKey(h.bufK, h.disableNormalizing)
	return h.peek(h.bufK)
}

// PeekCanonical returns header value for the given key without normalizing it (RFC 9110 Section 5.1).
//
// Thread-safe: No.
func (h *RequestHeader) PeekCanonical(key []byte) []byte {
	return h.peek(key)
}

func (h *RequestHeader) peek(key []byte) []byte {
	switch string(key) {
	case HeaderHost:
		return h.Host()
	case HeaderContentType:
		return h.ContentType()
	case HeaderUserAgent:
		return h.UserAgent()
	case HeaderConnection:
		if h.ConnectionClose() {
			return zerocopy.StrClose
		}

		return peekArgBytesHeaders(&h.h, key)

	case HeaderContentLength:
		return h.contentLengthBytes
	case HeaderCookie:
		if h.cookiesCollected {
			return zerocopy.AppendRequestCookieBytes(nil, h.cookies)
		}

		return peekArgBytesHeaders(&h.h, key)

	case HeaderTrailer:
		return appendTrailerBytes(nil, h.trailer, zerocopy.StrCommaSpace)
	default:
		return peekArgBytesHeaders(&h.h, key)
	}
}

// PeekAll returns all header values for the given key (RFC 9110 Section 5.2).
// The returned slice is valid until the request is released. Do not store references.
//
// Thread-safe: No.
func (h *RequestHeader) PeekAll(key string) [][]byte {
	h.bufK = zerocopy.GetHeaderKeyBytes(h.bufK, key, h.disableNormalizing)
	return h.peekAll(h.bufK)
}

func (h *RequestHeader) peekAll(key []byte) [][]byte {
	h.mulHeader = h.mulHeader[:0]
	switch string(key) {
	case HeaderHost:
		if host := h.Host(); len(host) > 0 {
			h.mulHeader = append(h.mulHeader, host)
		}
	case HeaderContentType:
		if contentType := h.ContentType(); len(contentType) > 0 {
			h.mulHeader = append(h.mulHeader, contentType)
		}
	case HeaderUserAgent:
		if ua := h.UserAgent(); len(ua) > 0 {
			h.mulHeader = append(h.mulHeader, ua)
		}
	case HeaderConnection:
		if h.ConnectionClose() {
			h.mulHeader = append(h.mulHeader, zerocopy.StrClose)
		} else {
			h.mulHeader = peekAllArgBytesToDstHeaders(h.mulHeader, &h.h, key)
		}

	case HeaderContentLength:
		h.mulHeader = append(h.mulHeader, h.contentLengthBytes)
	case HeaderCookie:
		if h.cookiesCollected {
			h.mulHeader = append(h.mulHeader, zerocopy.AppendRequestCookieBytes(nil, h.cookies))
		} else {
			h.mulHeader = peekAllArgBytesToDstHeaders(h.mulHeader, &h.h, key)
		}

	case HeaderTrailer:
		h.mulHeader = append(h.mulHeader, appendTrailerBytes(nil, h.trailer, zerocopy.StrCommaSpace))
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

// SetContentRange sets 'Content-Range: bytes startPos-endPos/contentLength' header (RFC 9110 Section 14.4).
//
// Thread-safe: No.
func (h *ResponseHeader) SetContentRange(startPos, endPos, contentLength int) {
	b := h.bufV[:0]
	b = append(b, zerocopy.StrBytes...)
	b = append(b, ' ')
	b = zerocopy.AppendUint(b, startPos)
	b = append(b, '-')
	b = zerocopy.AppendUint(b, endPos)
	b = append(b, '/')
	b = zerocopy.AppendUint(b, contentLength)
	h.bufV = b
	h.setNonSpecial(zerocopy.StrContentRange, h.bufV)
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
	h.statusMessage = zerocopy.InitHeaderValueBytes(h.statusMessage, statusMessage)
}

// SetProtocol sets response protocol bytes (RFC 9112 Section 2.3).
//
// Thread-safe: No.
func (h *ResponseHeader) SetProtocol(protocol []byte) {
	h.protocol = zerocopy.InitHeaderValueBytes(h.protocol, protocol)
}

// SetLastModified sets 'Last-Modified' header to the given timestamp (RFC 9110 Section 8.8.2).
//
// Thread-safe: No.
func (h *ResponseHeader) SetLastModified(t time.Time) {
	h.bufV = zerocopy.AppendHTTPDate(h.bufV[:0], t)
	h.setNonSpecial(zerocopy.StrLastModified, h.bufV)
}

// ConnectionUpgrade returns true if 'Connection: Upgrade' header is set (RFC 9110 Section 7.8).
//
// Thread-safe: No.
func (h *ResponseHeader) ConnectionUpgrade() bool {
	return hasHeaderValue(h.Peek(HeaderConnection), zerocopy.StrUpgrade)
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
	h.contentLengthBytes = zerocopy.AppendUint(h.contentLengthBytes[:0], contentLength)
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

	return bytes.HasPrefix(contentType, zerocopy.StrTextSlash) ||
		bytes.HasPrefix(contentType, zerocopy.StrApplicationSlash) ||
		bytes.HasPrefix(contentType, zerocopy.StrImageSVG) ||
		bytes.HasPrefix(contentType, zerocopy.StrImageIcon) ||
		bytes.HasPrefix(contentType, zerocopy.StrFontSlash) ||
		bytes.HasPrefix(contentType, zerocopy.StrMultipartSlash)
}

// ContentType returns Content-Type header value (RFC 9110 Section 8.3).
//
// Thread-safe: No.
func (h *ResponseHeader) ContentType() []byte {
	contentType := h.contentType
	if !h.noDefaultContentType && len(h.contentType) == 0 {
		contentType = zerocopy.DefaultContentType
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
	h.contentEncoding = zerocopy.InitHeaderValueString(h.contentEncoding, contentEncoding)
}

// SetContentEncodingBytes sets Content-Encoding header value (RFC 9110 Section 8.4).
//
// Thread-safe: No.
func (h *ResponseHeader) SetContentEncodingBytes(contentEncoding []byte) {
	h.contentEncoding = zerocopy.InitHeaderValueBytes(h.contentEncoding, contentEncoding)
}

// addVaryBytes adds value to the 'Vary' header if it's not included (RFC 9110 Section 12.5.5).
func (h *ResponseHeader) addVaryBytes(value []byte) {
	v := h.peek(zerocopy.StrVary)
	if len(v) == 0 {
		h.SetBytesV(HeaderVary, value)
	} else if !bytes.Contains(v, value) {
		h.SetBytesV(HeaderVary, append(append(v, ','), value...))
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
	altSvcHeader := h.peek([]byte("Alt-Svc"))
	if len(altSvcHeader) == 0 {
		return nil
	}

	return altsvc.Parse(bytesconv.B2S(altSvcHeader))
}

// SetServer sets Server header value (RFC 9110 Section 10.2.4).
//
// Thread-safe: No.
func (h *ResponseHeader) SetServer(server string) {
	h.server = zerocopy.InitHeaderValueString(h.server, server)
}

// SetServerBytes sets Server header value (RFC 9110 Section 10.2.4).
//
// Thread-safe: No.
func (h *ResponseHeader) SetServerBytes(server []byte) {
	h.server = zerocopy.InitHeaderValueBytes(h.server, server)
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
		for _, e := range h.h.Entries() {
			if !yield(bytesconv.S2B(e.Key), bytesconv.S2B(e.Value)) {
				return
			}
		}

		if len(h.contentType) > 0 {
			if !yield(zerocopy.StrContentType, h.contentType) {
				return
			}
		}

		if len(h.contentEncoding) > 0 {
			if !yield(zerocopy.StrContentEncoding, h.contentEncoding) {
				return
			}
		}

		if len(h.server) > 0 {
			if !yield(zerocopy.StrServer, h.server) {
				return
			}
		}

		if len(h.contentLengthBytes) > 0 {
			if !yield(zerocopy.StrContentLength, h.contentLengthBytes) {
				return
			}
		}

		if h.connectionClose {
			if !yield(zerocopy.StrConnection, zerocopy.StrClose) {
				return
			}
		}

		if len(h.cookies) > 0 {
			for i := range h.cookies {
				if !yield(zerocopy.StrSetCookie, h.cookies[i].Value) {
					return
				}
			}
		}

		if len(h.trailer) > 0 {
			if !yield(zerocopy.StrTrailer, appendTrailerBytes(nil, h.trailer, zerocopy.StrCommaSpace)) {
				return
			}
		}
	}
}

// Del deletes header with the given key (RFC 9110 Section 5).
//
// Thread-safe: No.
func (h *ResponseHeader) Del(key string) {
	h.bufK = zerocopy.GetHeaderKeyBytes(h.bufK, key, h.disableNormalizing)
	h.del(h.bufK)
}

// DelBytes deletes header with the given key bytes (RFC 9110 Section 5).
//
// Thread-safe: No.
func (h *ResponseHeader) DelBytes(key []byte) {
	h.bufK = append(h.bufK[:0], key...)
	zerocopy.NormalizeHeaderKey(h.bufK, h.disableNormalizing)
	h.del(h.bufK)
}

func (h *ResponseHeader) del(key []byte) {
	switch string(key) {
	case HeaderContentType:
		h.contentType = h.contentType[:0]
	case HeaderContentEncoding:
		h.contentEncoding = h.contentEncoding[:0]
	case HeaderServer:
		h.server = h.server[:0]
	case HeaderSetCookie:
		h.cookies = h.cookies[:0]
	case HeaderContentLength:
		h.contentLength = 0
		h.contentLengthBytes = h.contentLengthBytes[:0]
	case HeaderConnection:
		h.connectionClose = false
	case HeaderTrailer:
		h.trailer = h.trailer[:0]
	}

	h.h.Del(bytesconv.B2S(key))
}

func (h *ResponseHeader) setSpecialHeader(key, value []byte) bool {
	if len(key) == 0 {
		return false
	}

	switch key[0] | 0x20 {
	case 'c':
		switch {
		case zerocopy.CaseInsensitiveCompare(zerocopy.StrContentType, key):
			h.SetContentTypeBytes(value)
			return true
		case zerocopy.CaseInsensitiveCompare(zerocopy.StrContentLength, key):
			if contentLength, err := parseContentLength(value); err == nil {
				h.contentLength = contentLength
				h.contentLengthBytes = append(h.contentLengthBytes[:0], value...)
			}

			return true

		case zerocopy.CaseInsensitiveCompare(zerocopy.StrContentEncoding, key):
			h.SetContentEncodingBytes(value)
			return true
		case zerocopy.CaseInsensitiveCompare(zerocopy.StrConnection, key):
			if hasHeaderValue(value, zerocopy.StrClose) {
				h.SetConnectionClose()
			} else {
				h.ResetConnectionClose()
				h.setNonSpecial(key, value)
			}

			return true
		}

	case 's':
		if zerocopy.CaseInsensitiveCompare(zerocopy.StrServer, key) {
			h.SetServerBytes(value)
			return true
		} else if zerocopy.CaseInsensitiveCompare(zerocopy.StrSetCookie, key) {
			var kv *zerocopy.ArgsKV

			h.cookies, kv = zerocopy.AllocArg(h.cookies)
			kv.Key = zerocopy.GetCookieKey(kv.Key, value)
			kv.Value = append(kv.Value[:0], value...)

			return true
		}

	case 't':
		if zerocopy.CaseInsensitiveCompare(zerocopy.StrTransferEncoding, key) {
			return true
		} else if zerocopy.CaseInsensitiveCompare(zerocopy.StrTrailer, key) {
			_ = h.SetTrailerBytes(value)
			return true
		}

	case 'd':
		if zerocopy.CaseInsensitiveCompare(zerocopy.StrDate, key) {
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
	h.AddBytesKV(bytesconv.S2B(key), bytesconv.S2B(value))
}

// AddBytesK adds the given 'Key: value' header (RFC 9110 Section 5.2).
//
// Thread-safe: No.
func (h *ResponseHeader) AddBytesK(key []byte, value string) {
	h.AddBytesKV(key, bytesconv.S2B(value))
}

// AddBytesV adds the given 'Key: value' header (RFC 9110 Section 5.2).
//
// Thread-safe: No.
func (h *ResponseHeader) AddBytesV(key string, value []byte) {
	h.AddBytesKV(bytesconv.S2B(key), value)
}

// AddBytesKV adds the given 'Key: value' header (RFC 9110 Section 5.2).
//
// Thread-safe: No.
func (h *ResponseHeader) AddBytesKV(key, value []byte) {
	h.bufK, h.bufV = zerocopy.InitHeaderKV(
		h.bufK,
		h.bufV,
		bytesconv.B2S(key),
		bytesconv.B2S(value),
		h.disableNormalizing,
	)
	if h.setSpecialHeader(h.bufK, h.bufV) {
		return
	}

	appendArgBytesHeaders(&h.h, h.bufK, h.bufV, zerocopy.ArgsHasValue)
}

// Set sets the given 'Key: value' header (RFC 9110 Section 5.1).
// Use Add for setting multiple header values under the same key.
//
// Thread-safe: No.
func (h *ResponseHeader) Set(key, value string) {
	h.bufK, h.bufV = zerocopy.InitHeaderKV(h.bufK, h.bufV, key, value, h.disableNormalizing)
	h.SetCanonical(h.bufK, h.bufV)
}

// SetBytesK sets the given 'Key: value' header (RFC 9110 Section 5.1).
//
// Thread-safe: No.
func (h *ResponseHeader) SetBytesK(key []byte, value string) {
	h.bufV = append(h.bufV[:0], value...)
	h.SetBytesKV(key, h.bufV)
}

// SetBytesV sets the given 'Key: value' header (RFC 9110 Section 5.1).
//
// Thread-safe: No.
func (h *ResponseHeader) SetBytesV(key string, value []byte) {
	h.bufK = zerocopy.GetHeaderKeyBytes(h.bufK, key, h.disableNormalizing)
	h.SetCanonical(h.bufK, value)
}

// SetBytesKV sets the given 'Key: value' header (RFC 9110 Section 5.1).
//
// Thread-safe: No.
func (h *ResponseHeader) SetBytesKV(key, value []byte) {
	h.bufK = append(h.bufK[:0], key...)
	zerocopy.NormalizeHeaderKey(h.bufK, h.disableNormalizing)
	h.SetCanonical(h.bufK, value)
}

// SetCanonical sets the given 'Key: value' header assuming that key is in canonical form (RFC 9110 Section 5.1).
//
// Thread-safe: No.
func (h *ResponseHeader) SetCanonical(key, value []byte) {
	h.bufV = zerocopy.InitHeaderValueBytes(h.bufV, value)
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
	h.bufK = zerocopy.GetHeaderKeyBytes(h.bufK, key, h.disableNormalizing)
	return h.peek(h.bufK)
}

// PeekBytes returns header value for the given key bytes (RFC 9110 Section 5.1).
//
// Thread-safe: No.
func (h *ResponseHeader) PeekBytes(key []byte) []byte {
	h.bufK = append(h.bufK[:0], key...)
	zerocopy.NormalizeHeaderKey(h.bufK, h.disableNormalizing)
	return h.peek(h.bufK)
}

// PeekCanonical returns header value for the given key without normalizing it (RFC 9110 Section 5.1).
//
// Thread-safe: No.
func (h *ResponseHeader) PeekCanonical(key []byte) []byte {
	return h.peek(key)
}

func (h *ResponseHeader) peek(key []byte) []byte {
	switch string(key) {
	case HeaderContentType:
		return h.ContentType()
	case HeaderContentEncoding:
		return h.ContentEncoding()
	case HeaderServer:
		return h.Server()
	case HeaderConnection:
		if h.ConnectionClose() {
			return zerocopy.StrClose
		}

		return peekArgBytesHeaders(&h.h, key)

	case HeaderContentLength:
		return h.contentLengthBytes
	case HeaderSetCookie:
		return zerocopy.AppendResponseCookieBytes(nil, h.cookies)
	case HeaderTrailer:
		return appendTrailerBytes(nil, h.trailer, zerocopy.StrCommaSpace)
	default:
		return peekArgBytesHeaders(&h.h, key)
	}
}

// PeekAll returns all header values for the given key (RFC 9110 Section 5.2).
// The returned slice is valid until the response is released. Do not store references.
//
// Thread-safe: No.
func (h *ResponseHeader) PeekAll(key string) [][]byte {
	h.bufK = zerocopy.GetHeaderKeyBytes(h.bufK, key, h.disableNormalizing)
	return h.peekAll(h.bufK)
}

func (h *ResponseHeader) peekAll(key []byte) [][]byte {
	h.mulHeader = h.mulHeader[:0]
	switch string(key) {
	case HeaderContentType:
		if contentType := h.ContentType(); len(contentType) > 0 {
			h.mulHeader = append(h.mulHeader, contentType)
		}
	case HeaderContentEncoding:
		if contentEncoding := h.ContentEncoding(); len(contentEncoding) > 0 {
			h.mulHeader = append(h.mulHeader, contentEncoding)
		}
	case HeaderServer:
		if server := h.Server(); len(server) > 0 {
			h.mulHeader = append(h.mulHeader, server)
		}
	case HeaderConnection:
		if h.ConnectionClose() {
			h.mulHeader = append(h.mulHeader, zerocopy.StrClose)
		} else {
			h.mulHeader = peekAllArgBytesToDstHeaders(h.mulHeader, &h.h, key)
		}

	case HeaderContentLength:
		h.mulHeader = append(h.mulHeader, h.contentLengthBytes)
	case HeaderSetCookie:
		h.mulHeader = append(h.mulHeader, zerocopy.AppendResponseCookieBytes(nil, h.cookies))
	case HeaderTrailer:
		h.mulHeader = append(h.mulHeader, appendTrailerBytes(nil, h.trailer, zerocopy.StrCommaSpace))
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
	b := zerocopy.AppendHTTPDate(nil, time.Now())
	serverDate.Store(&b)
}

func appendHeaderLine(dst, key, value []byte) []byte {
	dst = append(dst, key...)
	dst = append(dst, zerocopy.StrColonSpace...)

	if bytes.IndexByte(value, '\n') < 0 && bytes.IndexByte(value, '\r') < 0 {
		dst = append(dst, value...)
	} else {
		for _, c := range value {
			if c == '\n' || c == '\r' {
				dst = append(dst, ' ')
			} else {
				dst = append(dst, c)
			}
		}
	}

	return append(dst, zerocopy.StrCRLF...)
}

func stripSpace(b []byte) []byte {
	for len(b) > 0 && b[0] == ' ' {
		b = b[1:]
	}

	for len(b) > 0 && b[len(b)-1] == ' ' {
		b = b[:len(b)-1]
	}

	return b
}

func hasHeaderValue(s, value []byte) bool {
	var vs headerValueScanner

	vs.b = s
	for vs.next() {
		if zerocopy.CaseInsensitiveCompare(vs.value, value) {
			return true
		}
	}

	return false
}

type headerValueScanner struct {
	b     []byte
	value []byte
}

func (s *headerValueScanner) next() bool {
	b := s.b
	if len(b) == 0 {
		return false
	}

	before, after, ok := bytes.Cut(b, []byte{','})
	if !ok {
		s.value = stripSpace(b)
		s.b = b[len(b):]
		return true
	}

	s.value = stripSpace(before)
	s.b = after

	return true
}

// AppendNormalizedHeaderKey appends normalized header key (name) to dst and returns the resulting dst.
//
// Normalized header key starts with uppercase letter. The first letters after dashes are also uppercased.
// All other letters are lowercased.
func AppendNormalizedHeaderKey(dst []byte, key string) []byte {
	dst = append(dst, key...)
	zerocopy.NormalizeHeaderKey(dst[len(dst)-len(key):], false)
	return dst
}

// AppendNormalizedHeaderKeyBytes appends normalized header key (name) to dst and returns the resulting dst.
//
// Normalized header key starts with uppercase letter. The first letters after dashes are also uppercased.
// All other letters are lowercased.
func AppendNormalizedHeaderKeyBytes(dst, key []byte) []byte {
	return AppendNormalizedHeaderKey(dst, bytesconv.B2S(key))
}

// VisitHeaderParams calls f for each parameter in the given header bytes (RFC 9110 Section 5.6.6).
// It stops processing when f returns false or an invalid parameter is found.
// Parameter values may be quoted, in which case \ is treated as an escape character.
//
// f must not retain references to key and/or value after returning.
func VisitHeaderParams(b []byte, f func(key, value []byte) bool) {
	for len(b) > 0 {
		idxSemi := 0
		for idxSemi < len(b) && b[idxSemi] != ';' {
			idxSemi++
		}

		if idxSemi >= len(b) {
			return
		}

		b = b[idxSemi+1:]
		for len(b) > 0 && b[0] == ' ' {
			b = b[1:]
		}

		n := 0
		if len(b) == 0 || !zerocopy.ValidHeaderFieldByte(b[n]) {
			return
		}

		n++
		for n < len(b) && zerocopy.ValidHeaderFieldByte(b[n]) {
			n++
		}

		if n >= len(b)-1 || b[n] != '=' {
			return
		}

		param := b[:n]

		n++
		switch {
		case zerocopy.ValidHeaderFieldByte(b[n]):
			m := n

			n++
			for n < len(b) && zerocopy.ValidHeaderFieldByte(b[n]) {
				n++
			}

			if !f(param, b[m:n]) {
				return
			}

		case b[n] == '"':
			foundEndQuote := false
			escaping := false
			n++

			m := n
			for ; n < len(b); n++ {
				if b[n] == '"' && !escaping {
					foundEndQuote = true
					break
				}

				escaping = (b[n] == '\\' && !escaping)
			}

			if !foundEndQuote {
				return
			}

			if !f(param, b[m:n]) {
				return
			}

			n++

		default:
			return
		}

		b = b[n:]
	}
}

// peekArgBytesHeaders returns the header value matching key from h.
func peekArgBytesHeaders(h *headkit.Headers, key []byte) []byte {
	v := h.Get(bytesconv.B2S(key))
	if v == "" {
		return nil
	}

	return bytesconv.S2B(v)
}

// setArgBytesHeaders sets the header value for key in h.
func setArgBytesHeaders(h *headkit.Headers, key, value []byte, noValue bool) {
	h.Set(string(key), string(value))
}

// appendArgBytesHeaders appends a header key-value entry to h.
func appendArgBytesHeaders(h *headkit.Headers, key, value []byte, noValue bool) {
	h.Add(string(key), string(value))
}

// copyHeaders copies all header entries from src to dst.
func copyHeaders(dst, src *headkit.Headers) {
	dst.Reset()

	for _, e := range src.Entries() {
		dst.Add(e.Key, e.Value)
	}
}

// peekAllArgBytesToDstHeaders appends all values matching key in h to dst.
func peekAllArgBytesToDstHeaders(dst [][]byte, h *headkit.Headers, key []byte) [][]byte {
	kStr := bytesconv.B2S(key)
	for _, e := range h.Entries() {
		if bytesconv.EqualFoldASCII(e.Key, kStr) {
			dst = append(dst, bytesconv.S2B(e.Value))
		}
	}

	return dst
}
