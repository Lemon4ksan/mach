// Code automatically split by refactoring script

package http

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"iter"

	"github.com/lemon4ksan/foundation/borrow"
	"github.com/lemon4ksan/foundation/net/http/zerocopy"
	"github.com/lemon4ksan/foundation/silicon/bytesconv"
)

type RequestHeader struct {
	header
	noCopy zerocopy.NoCopy
	method []byte // RequestHeader represents HTTP request header.
	//
	// It is forbidden copying RequestHeader instances.
	// Create new instances instead and use CopyTo.
	//
	// RequestHeader instance MUST NOT be used from concurrently running
	// goroutines.

	requestURI []byte
	host       []byte
	userAgent  []byte
	rawHeaders []byte // stores an immutable copy of headers as they were received from the
	// wire.

	disableSpecialHeader bool
	cookiesCollected     bool
}

func (h *RequestHeader) SetByteRange(startPos, endPos int) {
	b := h.bufV[: // SetByteRange sets 'Range: bytes=startPos-endPos' header.
	//
	//   - If startPos is negative, then 'bytes=-startPos' value is set.
	//   - If endPos is negative, then 'bytes=startPos-' value is set.
	0]
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

func (h *RequestHeader) ConnectionUpgrade() bool {
	return hasHeaderValue(h.Peek(HeaderConnection), zerocopy.StrUpgrade)
} // ConnectionUpgrade returns true if 'Connection: Upgrade' header is set.

func (h *RequestHeader) ContentLength() int {
	if h.disableSpecialHeader {
		te := peekArgBytesHeaders(&h.h, zerocopy.StrTransferEncoding)
		if zerocopy.CaseInsensitiveCompare(te, zerocopy.StrChunked) {
			return -1
		}
		v := peekArgBytesHeaders(&h.h, zerocopy.StrContentLength)
		if len(v) == 0 {
			return -2
		}
		n, err := parseContentLength(v)
		if err != nil {
			return -2
		}
		return n
	}
	return h.contentLength
} // ContentLength returns Content-Length header value.
//
// It may be negative:
// -1 means Transfer-Encoding: chunked.
// -2 means Transfer-Encoding: identity.

func (h *RequestHeader) SetContentLength(contentLength int) {
	h.contentLength = contentLength
	if contentLength >= 0 {
		h.contentLengthBytes = zerocopy.AppendUint(h.contentLengthBytes[: // SetContentLength sets Content-Length header value.
		//
		// Negative content-length sets 'Transfer-Encoding: chunked' header.
		0], contentLength)
		h.h.Del(HeaderTransferEncoding)
	} else {
		h.contentLengthBytes = h.contentLengthBytes[:0]
		setArgBytesHeaders(&h.h, zerocopy.StrTransferEncoding, zerocopy.StrChunked, zerocopy.ArgsHasValue)
	}
}

func (h *RequestHeader) ContentType() []byte { // ContentType returns Content-Type header value.

	if h.disableSpecialHeader {
		return peekArgBytesHeaders(&h.h, []byte(HeaderContentType))
	}
	return h.contentType
}

func (h *RequestHeader) ContentEncoding() []byte { // ContentEncoding returns Content-Encoding header value.

	return peekArgBytesHeaders(&h.h, zerocopy.StrContentEncoding)
}

func (h *RequestHeader) SetContentEncoding(contentEncoding string) {
	h.SetBytesK(zerocopy.StrContentEncoding, contentEncoding)
} // SetContentEncoding sets Content-Encoding header value.

func (h *RequestHeader) SetContentEncodingBytes(contentEncoding []byte) { // SetContentEncodingBytes sets Content-Encoding header value.

	h.bufV = zerocopy.InitHeaderValueBytes(h.bufV, contentEncoding)
	h.setNonSpecial(zerocopy.StrContentEncoding, h.bufV)
}

func (h *RequestHeader) SetMultipartFormBoundary(boundary string) {
	b := h.bufV[: // SetMultipartFormBoundary sets the following Content-Type:
	// 'multipart/form-data; boundary=...'
	// where ... is substituted by the given boundary.
	0]
	b = append(b, zerocopy.StrMultipartFormData...)
	b = append(b, ';', ' ')
	b = append(b, zerocopy.StrBoundary...)
	b = append(b, '=')
	b = append(b, boundary...)
	h.bufV = b
	h.SetContentTypeBytes(h.bufV)
}

func (h *RequestHeader) SetMultipartFormBoundaryBytes(boundary []byte) { // SetMultipartFormBoundaryBytes sets the following Content-Type:
	// 'multipart/form-data; boundary=...'
	// where ... is substituted by the given boundary.

	b := h.bufV[:0]
	b = append(b, zerocopy.StrMultipartFormData...)
	b = append(b, ';', ' ')
	b = append(b, zerocopy.StrBoundary...)
	b = append(b, '=')
	b = append(b, boundary...)
	h.bufV = b
	h.SetContentTypeBytes(h.bufV)
}

func (h *RequestHeader) MultipartFormBoundary() []byte { // MultipartFormBoundary returns boundary part
	// from 'multipart/form-data; boundary=...' Content-Type.

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

func (h *RequestHeader) Host() []byte { // Host returns Host header value.

	if h.disableSpecialHeader {
		return peekArgBytesHeaders(&h.h, []byte(HeaderHost))
	}
	return h.host
}

func (h *RequestHeader) SetHost(host string) {
	h.host = zerocopy.InitHeaderValueString(h.host, host)
} // SetHost sets Host header value.

func (h *RequestHeader) SetHostBytes(host []byte) { // SetHostBytes sets Host header value.

	h.host = zerocopy.InitHeaderValueBytes(h.host, host)
}

func (h *RequestHeader) UserAgent() []byte { // UserAgent returns User-Agent header value.

	if h.disableSpecialHeader {
		return peekArgBytesHeaders(&h.h, []byte(HeaderUserAgent))
	}
	return h.userAgent
}

func (h *RequestHeader) SetUserAgent(userAgent string) {
	h.userAgent = zerocopy.InitHeaderValueString(h.userAgent, userAgent)
} // SetUserAgent sets User-Agent header value.

func (h *RequestHeader) SetUserAgentBytes(userAgent []byte) { // SetUserAgentBytes sets User-Agent header value.

	h.userAgent = zerocopy.InitHeaderValueBytes(h.userAgent, userAgent)
}

func (h *RequestHeader) Referer() []byte { // Referer returns Referer header value.

	return peekArgBytesHeaders(&h.h, zerocopy.StrReferer)
}

func (h *RequestHeader) SetReferer(referer string) {
	h.SetBytesK(zerocopy.StrReferer, referer)
} // SetReferer sets Referer header value.

func (h *RequestHeader) SetRefererBytes(referer []byte) { // SetRefererBytes sets Referer header value.

	h.bufV = zerocopy.InitHeaderValueBytes(h.bufV, referer)
	h.setNonSpecial(zerocopy.StrReferer, h.bufV)
}

func (h *RequestHeader) Method() []byte { // Method returns HTTP request method.

	if len(h.method) == 0 {
		return []byte(MethodGet)
	}
	return h.method
}

func (h *RequestHeader) SetMethod(method string) {
	h.method = zerocopy.InitHeaderValueString(h.method, method)
} // SetMethod sets HTTP request method.

func (h *RequestHeader) SetMethodBytes(method []byte) { // SetMethodBytes sets HTTP request method.

	h.method = zerocopy.InitHeaderValueBytes(h.method, method)
}

func (h *RequestHeader) SetProtocol(protocol string) {
	h.protocol = zerocopy.InitHeaderValueString(h.protocol, protocol)
	h.noHTTP11 = !bytes.Equal(h.protocol, zerocopy.StrHTTP11)
} // SetProtocol sets HTTP request protocol.

func (h *RequestHeader) SetProtocolBytes(protocol []byte) { // SetProtocolBytes sets HTTP request protocol.

	h.protocol = zerocopy.InitHeaderValueBytes(h.protocol, protocol)
	h.noHTTP11 = !bytes.Equal(h.protocol, zerocopy.StrHTTP11)
}

func (h *RequestHeader) RequestURI() []byte { // RequestURI returns RequestURI from the first HTTP request line.

	requestURI := h.requestURI
	if len(requestURI) == 0 {
		requestURI = zerocopy.StrSlash
	}
	return requestURI
}

func (h *RequestHeader) SetRequestURI(requestURI string) {
	h.requestURI = zerocopy.InitHeaderValueString(h.requestURI, requestURI)
} // SetRequestURI sets RequestURI for the first HTTP request line.
// RequestURI must be properly encoded.
// Use zerocopy.URI.RequestURI for constructing proper RequestURI if unsure.

func (h *RequestHeader) SetRequestURIBytes(requestURI []byte) { // SetRequestURIBytes sets RequestURI for the first HTTP request line.
	// RequestURI must be properly encoded.
	// Use zerocopy.URI.RequestURI for constructing proper RequestURI if unsure.

	h.requestURI = zerocopy.InitHeaderValueBytes(h.requestURI, requestURI)
}

func (h *RequestHeader) IsGet() bool {
	return string(h.Method()) == MethodGet
} // IsGet returns true if request method is GET.

func (h *RequestHeader) IsPost() bool {
	return string(h.Method()) == MethodPost
} // IsPost returns true if request method is POST.

func (h *RequestHeader) IsPut() bool {
	return string(h.Method()) == MethodPut
} // IsPut returns true if request method is PUT.

func (h *RequestHeader) IsHead() bool {
	return string(h.Method()) == MethodHead
} // IsHead returns true if request method is HEAD.

func (h *RequestHeader) IsDelete() bool {
	return string(h.Method()) == MethodDelete
} // IsDelete returns true if request method is DELETE.

func (h *RequestHeader) IsConnect() bool {
	return string(h.Method()) == MethodConnect
} // IsConnect returns true if request method is CONNECT.

func (h *RequestHeader) IsOptions() bool {
	return string(h.Method()) == MethodOptions
} // IsOptions returns true if request method is OPTIONS.

func (h *RequestHeader) IsTrace() bool {
	return string(h.Method()) == MethodTrace
} // IsTrace returns true if request method is TRACE.

func (h *RequestHeader) IsPatch() bool {
	return string(h.Method()) == MethodPatch
} // IsPatch returns true if request method is PATCH.

func (h *RequestHeader) HasAcceptEncoding(acceptEncoding string) bool {
	h.bufV = append(h.bufV[: // HasAcceptEncoding returns true if the header contains
	// the given Accept-Encoding value.
	0], acceptEncoding...)
	return h.HasAcceptEncodingBytes(h.bufV)
}

func (h *RequestHeader) HasAcceptEncodingBytes(acceptEncoding []byte) bool { // HasAcceptEncodingBytes returns true if the header contains
	// the given Accept-Encoding value.

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

func (h *RequestHeader) Len() int {
	n := 0
	for range // Len returns the number of headers set,
	// i.e. the number of times f is called in VisitAll.
	h.All() {
		n++
	}
	return n
}

func (h *RequestHeader) DisableSpecialHeader() bool {
	orig := h.disableSpecialHeader
	h.disableSpecialHeader = true
	return orig
} // DisableSpecialHeader disables special header processing.
// fasthttp will not set any special headers for you, such as Host, Content-Type, User-Agent, etc.
// You must set everything yourself.
// If RequestHeader.Read() is called, special headers will be ignored.
// This can be used to control case and order of special headers.
// This is generally not recommended.
// The previous setting is returned.

func (h *RequestHeader) EnableSpecialHeader() bool {
	orig := h.disableSpecialHeader
	h.disableSpecialHeader = false
	return orig
} // EnableSpecialHeader enables special header processing.
// fasthttp will send Host, Content-Type, User-Agent, etc headers for you.
// This is suggested and enabled by default.
// The previous setting is returned.

func (h *RequestHeader) Reset() {
	h.disableSpecialHeader = false
	h.disableNormalizing = false
	h.SetNoDefaultContentType(false)
	h.resetSkipNormalize()
} // Reset clears request header.

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

func (h *RequestHeader) CopyTo(dst *RequestHeader) {
	dst.Reset()
	h.copyTo(&dst.header)
	dst.method = append(dst.method, h.method...)
	dst.requestURI = append(dst.requestURI, h.requestURI...)
	dst.host = append(dst.host, h.host...)
	dst.userAgent = append(dst.userAgent, h.userAgent...)
	dst.cookiesCollected = h.cookiesCollected
	dst.rawHeaders = append(dst.rawHeaders, h.rawHeaders...)
} // CopyTo copies all the headers to dst.

func (h *RequestHeader) Cookies() iter.Seq2[[]byte, // Cookies returns an iterator over key-value pairs request cookie in h.
	// The key and value may invalid outside the iteration loop.
	// Copy key and/or value contents for each iteration if you need retaining
	// them.
	//
	// Making modifications to the RequestHeader during the iteration loop leads to undefined
	// behavior and can cause panics.
	[]byte] {
	return func(yield func([]byte, []byte) bool) {
		h.collectCookies()
		for i := range h.cookies {
			if !yield(h.cookies[i].Key, h.cookies[i].Value) {
				break
			}
		}
	}
}

func (h *RequestHeader) All() iter.Seq2[[]byte, // All returns an iterator over key-value pairs in h.
	// The key and value may invalid outside the iteration loop.
	// Copy key and/or value contents for each iteration if you need retaining
	// them.
	//
	// To get the headers in order they were received use AllInOrder.
	//
	// Making modifications to the RequestHeader during the iteration loop leads to undefined
	// behavior and can cause panics.
	[]byte] {
	return func(yield func([]byte, []byte) bool) {
		if host := h.Host(); len(host) > 0 && !yield(zerocopy.StrHost, host) {
			return
		}
		if len(h.contentLengthBytes) > 0 && !yield(zerocopy.StrContentLength, h.contentLengthBytes) {
			return
		}
		if contentType := h.ContentType(); len(contentType) > 0 && !yield(zerocopy.StrContentType, contentType) {
			return
		}
		if userAgent := h.UserAgent(); len(userAgent) > 0 && !yield(zerocopy.StrUserAgent, userAgent) {
			return
		}
		if len(h.trailer) > 0 && !yield(zerocopy.StrTrailer, appendTrailerBytes(nil, h.trailer, zerocopy.StrCommaSpace)) {
			return
		}
		h.collectCookies()
		if len(h.cookies) > 0 {
			h.bufV = zerocopy.AppendRequestCookieBytes(h.bufV[:0], h.cookies)
			if !yield(zerocopy.StrCookie, h.bufV) {
				return
			}
		}
		for _, e := range h.h.Entries() {
			if !yield(bytesconv.S2B(e.Key), bytesconv.S2B(e.Value)) {
				return
			}
		}
		if h.ConnectionClose() && !yield(zerocopy.StrConnection, zerocopy.StrClose) {
			return
		}
	}
}

func (h *RequestHeader) AllInOrder() iter.Seq2[[]byte, // AllInOrder returns an iterator over key-value pairs in h in the order they
	// were received.
	//
	// The key and value may invalid outside the iteration loop.
	// Copy key and/or value contents for each iteration if you need retaining
	// them.
	//
	// The returned iterator is slightly slower than All because it has to reparse
	// the raw headers to get the order.
	//
	// Making modifications to the RequestHeader during the iteration loop leads to undefined
	// behavior and can cause panics.
	[]byte] {
	return func(yield func([]byte, []byte) bool) {
		var s headerScanner
		s.b = h.rawHeaders
		s.blockEnd = len(h.rawHeaders)
		for s.next() {
			s.key = trimTrailingSpace(s.key)
			zerocopy.NormalizeHeaderKey(s.key, h.disableNormalizing)
			if len(s.key) > 0 {
				if !yield(s.key, s.value) {
					break
				}
			}
		}
	}
}

func (h *RequestHeader) Del(key string) {
	h.bufK = zerocopy.GetHeaderKeyBytes(h.bufK, key, h.disableNormalizing)
	h.del(h.bufK)
} // Del deletes header with the given key.

func (h *RequestHeader) DelBytes(key []byte) { // DelBytes deletes header with the given key.

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

func (h *RequestHeader) setSpecialHeader(key, value []byte) bool { // setSpecialHeader handles special headers and return true when a header is processed.

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
			if bytes.Equal(zerocopy.StrClose, value) {
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

func (h *RequestHeader) SetCookie(key, value string) {
	h.collectCookies()
	h.bufK = zerocopy.InitHeaderValueString(h.bufK, key)
	h.bufV = zerocopy.InitHeaderValueString(h.bufV, value)
	h.cookies = zerocopy.SetArgBytes(h.cookies, h.bufK, h.bufV, zerocopy.ArgsHasValue)
} // SetCookie sets 'Key: value' cookies.

func (h *RequestHeader) SetCookieBytesK(key []byte, // SetCookieBytesK sets 'Key: value' cookies.
	value string) {
	h.SetCookie(bytesconv.B2S(key), value)
}

func (h *RequestHeader) SetCookieBytesKV(key, value []byte) { // SetCookieBytesKV sets 'Key: value' cookies.

	h.SetCookie(bytesconv.B2S(key), bytesconv.B2S(value))
}

func (h *RequestHeader) DelCookie(key string) {
	h.collectCookies()
	h.cookies = zerocopy.DelAllArgs(h.cookies, key)
} // DelCookie removes cookie under the given key.

func (h *RequestHeader) DelCookieBytes(key []byte) { // DelCookieBytes removes cookie under the given key.

	h.DelCookie(bytesconv.B2S(key))
}

func (h *RequestHeader) DelAllCookies() {
	h.collectCookies()
	h.cookies = h.cookies[: // DelAllCookies removes all the cookies from request headers.
	0]
}

func (h *RequestHeader) Add(key, value string) {
	h.AddBytesKV(bytesconv.S2B(key), bytesconv.S2B(value))
} // Add adds the given 'Key: value' header.
//
// Multiple headers with the same key may be added with this function.
// Use Set for setting a single header for the given key.
//
// If the header is set as a Trailer (forbidden trailers will not be set, see AddTrailer for more details),
// it will be sent after the chunked request body.

func (h *RequestHeader) AddBytesK(key []byte, // AddBytesK adds the given 'Key: value' header.
	//
	// Multiple headers with the same key may be added with this function.
	// Use SetBytesK for setting a single header for the given key.
	//
	// If the header is set as a Trailer (forbidden trailers will not be set, see AddTrailer for more details),
	// it will be sent after the chunked request body.
	value string) {
	h.AddBytesKV(key, bytesconv.S2B(value))
}

func (h *RequestHeader) AddBytesV(key string, value []byte) { // AddBytesV adds the given 'Key: value' header.
	//
	// Multiple headers with the same key may be added with this function.
	// Use SetBytesV for setting a single header for the given key.
	//
	// If the header is set as a Trailer (forbidden trailers will not be set, see AddTrailer for more details),
	// it will be sent after the chunked request body.

	h.AddBytesKV(bytesconv.S2B(key), value)
}

func (h *RequestHeader) AddBytesKV(key, value []byte) { // AddBytesKV adds the given 'Key: value' header.
	//
	// Multiple headers with the same key may be added with this function.
	// Use SetBytesKV for setting a single header for the given key.
	//
	// the Content-Type, Content-Length, Connection, Transfer-Encoding,
	// Host and User-Agent headers can only be set once and will overwrite
	// the previous value, while the zerocopy.Cookie header will not clear previous cookies.
	//
	// If the header is set as a Trailer (forbidden trailers will not be set, see AddTrailer for more details),
	// it will be sent after the chunked request body.

	h.bufK, h.bufV = zerocopy.InitHeaderKV(h.bufK, h.bufV, bytesconv.B2S(key), bytesconv.B2S(value), h.disableNormalizing)
	if h.setSpecialHeader(h.bufK, h.bufV) {
		return
	}
	appendArgBytesHeaders(&h.h, h.bufK, h.bufV, zerocopy.ArgsHasValue)
}

func (h *RequestHeader) Set(key, value string) {
	h.bufK, h.bufV = zerocopy.InitHeaderKV(h.bufK, h.bufV, key, value, h.disableNormalizing)
	h.SetCanonical(h.bufK, h.bufV)
} // Set sets the given 'Key: value' header.
//
// Please note that the zerocopy.Cookie header will not clear previous cookies,
// delete cookies before calling in order to reset cookies.
//
// If the header is set as a Trailer (forbidden trailers will not be set, see SetTrailer for more details),
// it will be sent after the chunked request body.
//
// Use Add for setting multiple header values under the same key.

func (h *RequestHeader) SetBytesK(key []byte, // SetBytesK sets the given 'Key: value' header.
	//
	// Please note that the zerocopy.Cookie header will not clear previous cookies,
	// delete cookies before calling in order to reset cookies.
	//
	// If the header is set as a Trailer (forbidden trailers will not be set, see SetTrailer for more details),
	// it will be sent after the chunked request body.
	//
	// Use AddBytesK for setting multiple header values under the same key.
	value string) {
	h.bufV = append(h.bufV[:0], value...)
	h.SetBytesKV(key, h.bufV)
}

func (h *RequestHeader) SetBytesV(key string, value []byte) { // SetBytesV sets the given 'Key: value' header.
	//
	// Please note that the zerocopy.Cookie header will not clear previous cookies,
	// delete cookies before calling in order to reset cookies.
	//
	// If the header is set as a Trailer (forbidden trailers will not be set, see SetTrailer for more details),
	// it will be sent after the chunked request body.
	//
	// Use AddBytesV for setting multiple header values under the same key.

	h.bufK = zerocopy.GetHeaderKeyBytes(h.bufK, key, h.disableNormalizing)
	h.SetCanonical(h.bufK, value)
}

func (h *RequestHeader) SetBytesKV(key, value []byte) { // SetBytesKV sets the given 'Key: value' header.
	//
	// Please note that the zerocopy.Cookie header will not clear previous cookies,
	// delete cookies before calling in order to reset cookies.
	//
	// If the header is set as a Trailer (forbidden trailers will not be set, see SetTrailer for more details),
	// it will be sent after the chunked request body.
	//
	// Use AddBytesKV for setting multiple header values under the same key.

	h.bufK = append(h.bufK[:0], key...)
	zerocopy.NormalizeHeaderKey(h.bufK, h.disableNormalizing)
	h.SetCanonical(h.bufK, value)
}

func (h *RequestHeader) SetCanonical(key, value []byte) { // SetCanonical sets the given 'Key: value' header assuming that
	// key is in canonical form.
	//
	// Please note that the zerocopy.Cookie header will not clear previous cookies,
	// delete cookies before calling in order to reset cookies.
	//
	// If the header is set as a Trailer (forbidden trailers will not be set, see SetTrailer for more details),
	// it will be sent after the chunked request body.

	h.bufV = zerocopy.InitHeaderValueBytes(h.bufV, value)
	if h.setSpecialHeader(key, h.bufV) {
		return
	}
	h.setNonSpecial(key, h.bufV)
}

func (h *RequestHeader) Peek(key string) []byte { // Peek returns header value for the given key.
	//
	// The returned value is valid until the request is released,
	// either though ReleaseRequest or your request handler returning.
	// Do not store references to returned value. Make copies instead.

	h.bufK = zerocopy.GetHeaderKeyBytes(h.bufK, key, h.disableNormalizing)
	return h.peek(h.bufK)
}

func (h *RequestHeader) PeekBytes(key []byte) []byte { // PeekBytes returns header value for the given key.
	//
	// The returned value is valid until the request is released,
	// either though ReleaseRequest or your request handler returning.
	// Do not store references to returned value. Make copies instead.

	h.bufK = append(h.bufK[:0], key...)
	zerocopy.NormalizeHeaderKey(h.bufK, h.disableNormalizing)
	return h.peek(h.bufK)
}

func (h *RequestHeader) PeekCanonical(key []byte) []byte { // PeekCanonical returns header value for the given key without normalizing it.
	// The key must match the canonical form used with SetCanonical.
	//
	// The returned value is valid until the request is released,
	// either though ReleaseRequest or your request handler returning.
	// Do not store references to the returned value. Make copies instead.

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

func (h *RequestHeader) PeekAll(key string) [][]byte { // PeekAll returns all header value for the given key.
	//
	// The returned value is valid until the request is released,
	// either though ReleaseRequest or your request handler returning.
	// Any future calls to the Peek* will modify the returned value.
	// Do not store references to returned value. Make copies instead.

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

func (h *RequestHeader) PeekKeys() [][]byte { // PeekKeys return all header keys.
	//
	// The returned value is valid until the request is released,
	// either though ReleaseRequest or your request handler returning.
	// Any future calls to the Peek* will modify the returned value.
	// Do not store references to returned value. Make copies instead.

	h.mulHeader = h.mulHeader[:0]
	for key := range h.All() {
		h.mulHeader = append(h.mulHeader, key)
	}
	return h.mulHeader
}

func (h *RequestHeader) Cookie(key string) []byte { // zerocopy.Cookie returns cookie for the given key.

	h.collectCookies()
	return zerocopy.PeekArgStr(h.cookies, key)
}

func (h *RequestHeader) CookieBytes(key []byte) []byte { // CookieBytes returns cookie for the given key.

	h.collectCookies()
	return zerocopy.PeekArgBytes(h.cookies, key)
}

func (h *RequestHeader) Read(r *bufio.Reader) error {
	return h.readLoop(r, true)
} // Read reads request header from r.
//
// io.EOF is returned if r is closed before reading the first header byte.

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
} // readLoop reads request header from r optionally loops until it has enough data.
//
// io.EOF is returned if r is closed before reading the first header byte.

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
			return &ErrSmallBuffer{error: fmt.Errorf("error when reading request headers: %w (n=%d, reader buffered=%d)", ErrSmallReadBuffer, n, r.Buffered())}
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

func (h *RequestHeader) Write(w *bufio.Writer) error {
	_, err := w.Write(h.Header())
	return err
} // Write writes request header to w.

func (h *RequestHeader) WriteTo(w io.Writer) (int64, error) {
	n, err := w.Write(h.Header())
	return int64(n), err
} // WriteTo writes request header to w.
//
// WriteTo implements io.WriterTo interface.

func (h *RequestHeader) Header() []byte { // Header returns request header representation.
	//
	// Headers that set as Trailer will not represent. Use TrailerHeader for trailers.
	//
	// The returned value is valid until the request is released,
	// either though ReleaseRequest or your request handler returning.
	// Do not store references to returned value. Make copies instead.

	h.bufV = h.AppendBytes(h.bufV[:0])
	return h.bufV
}

func (h *RequestHeader) writeTrailer(w *bufio.Writer) error {
	_, err := w.Write(h.TrailerHeader())
	return err
} // writeTrailer writes request trailer to w.

func (h *RequestHeader) TrailerHeader() []byte { // TrailerHeader returns request trailer header representation.
	//
	// Trailers will only be received with chunked transfer.
	//
	// The returned value is valid until the request is released,
	// either though ReleaseRequest or your request handler returning.
	// Do not store references to returned value. Make copies instead.

	h.bufV = h.bufV[:0]
	for _, t := range h.trailer {
		value := h.peek(t)
		h.bufV = appendHeaderLine(h.bufV, t, value)
	}
	h.bufV = append(h.bufV, zerocopy.StrCRLF...)
	return h.bufV
}

func (h *RequestHeader) RawHeaders() []byte { // RawHeaders returns raw header key/value bytes.
	//
	// Depending on server configuration, header keys may be normalized to
	// capital-case in place.
	//
	// This copy is set aside during parsing, so empty slice is returned for all
	// cases where parsing did not happen. Similarly, request line is not stored
	// during parsing and can not be returned.
	//
	// The slice is not safe to use after the handler returns.

	return h.rawHeaders
}

func (h *RequestHeader) String() string {
	return string(h.Header())
} // String returns request header representation.

func (h *RequestHeader) AppendBytes(dst []byte) []byte { // AppendBytes appends request header representation to dst and returns
	// the extended dst.

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
	n = bytes.IndexByte(b, ' ')
	if n < 0 {
		return 0, fmt.Errorf("cannot find whitespace in the first line of request %q", buf)
	}
	protoStr := b[n+1:]
	if !isHTTPVersion(protoStr) {
		if h.SecureErrorLogMessage {
			return 0, fmt.Errorf("unsupported http version %q", protoStr)
		}
		return 0, fmt.Errorf("unsupported http version %q in %q", protoStr, buf)
	}
	if n == 0 {
		if h.SecureErrorLogMessage {
			return 0, ErrEmptyRequestURI
		}
		return 0, fmt.Errorf("request uri cannot be empty in %q", buf)
	}
	if err := validateRequestURI(h.method, b[:n]); err != nil {
		if h.SecureErrorLogMessage {
			return 0, fmt.Errorf("invalid request uri %q", b[:n])
		}
		return 0, fmt.Errorf("invalid request uri %q in %q: %w", b[:n], buf, err)
	}
	h.noHTTP11 = !bytes.Equal(protoStr, zerocopy.StrHTTP11)
	h.protocol = append(h.protocol[:0], protoStr...)
	h.requestURI = append(h.requestURI[:0], b[:n]...)
	return len(buf) - len(bNext), nil
}

func (h *RequestHeader) parseHeaders(buf []byte, blockEnd int) (int, error) {
	h.contentLength = -2
	contentLengthSeen := false
	transferEncodingSeen := false
	hostSeen := false
	var s headerScanner
	s.b = buf
	s.blockEnd = blockEnd
	for s.next() {
		key := s.key
		s.key = trimTrailingSpace(s.key)
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
					h.connectionClose = true
					return 0, ErrDuplicateContentLength
				}
				contentLengthSeen = true
				var err error
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
				if bytes.Equal(s.value, zerocopy.StrClose) {
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

func (h *RequestHeader) collectCookies() {
	if h.cookiesCollected {
		return
	}
	for _, e := range h.h.Entries() {
		if zerocopy.CaseInsensitiveCompare(bytesconv.S2B(e.Key), zerocopy.StrCookie) {
			h.cookies = zerocopy.ParseRequestCookies(h.cookies, bytesconv.S2B(e.Value))
		}
	}
	h.h.Del(HeaderCookie)
	h.cookiesCollected = true
}

func (h *RequestHeader) PeekScoped(s *borrow.Scope, key string) borrow.Bytes {
	b := h.Peek(key)
	if len(b) == 0 {
		return borrow.Bytes{}
	}
	return borrow.NewBytes(b, nil)
} // Borrow methods moved from borrow.go
// PeekScoped borrows the header value associated with key into the given borrow scope.

func (h *RequestHeader) CookieScoped(s *borrow.Scope, key string) borrow.Bytes {
	b := h.Cookie(key)
	if len(b) == 0 {
		return borrow.Bytes{}
	}
	return borrow.NewBytes(b, nil)
} // CookieScoped borrows the cookie value associated with key into the given borrow scope.

func (h *RequestHeader) PeekAllScoped(s *borrow.Scope, key string) []borrow. // PeekAllScoped borrows all header values associated with key into a slice of borrowed bytes.
										Bytes {
	values := h.PeekAll(key)
	if len(values) == 0 {
		return nil
	}
	res := make([]borrow.Bytes, len(values))
	for i, v := range values {
		res[i] = borrow.NewBytes(v, nil)
	}
	return res
}

func (h *RequestHeader) TrailerScoped(s *borrow.Scope, key string) borrow.Bytes {
	b := h.Peek(key)
	if len(b) == 0 {
		return borrow.Bytes{}
	}
	return borrow.NewBytes(b, nil)
} // TrailerScoped borrows the trailer value associated with key into the given borrow scope.
