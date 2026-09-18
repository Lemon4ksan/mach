// Code automatically split by refactoring script

package http

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"iter"
	"time"

	"github.com/lemon4ksan/foundation/borrow"
	"github.com/lemon4ksan/foundation/net/http/zerocopy"
	"github.com/lemon4ksan/foundation/silicon/bytesconv"
)

type ResponseHeader struct {
	header
	noCopy        zerocopy.NoCopy
	statusMessage []byte // ResponseHeader represents HTTP response header.
	//
	// It is forbidden copying ResponseHeader instances.
	// Create new instances instead and use CopyTo.
	//
	// ResponseHeader instance MUST NOT be used from concurrently running
	// goroutines.

	contentEncoding []byte
	server          []byte
	statusCode      int
	noDefaultDate   bool
}

func (h *ResponseHeader) SetContentRange(startPos, endPos, contentLength int) {
	b := h.bufV[: // SetContentRange sets 'Content-Range: bytes startPos-endPos/contentLength'
	// header.
	0]
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

func (h *ResponseHeader) StatusCode() int {
	if h.statusCode == 0 {
		return StatusOK
	}
	return h.statusCode
} // StatusCode returns response status code.

func (h *ResponseHeader) SetStatusCode(statusCode int) {
	h.statusCode = statusCode
} // SetStatusCode sets response status code.

func (h *ResponseHeader) StatusMessage() []byte { // StatusMessage returns response status message.

	return h.statusMessage
}

func (h *ResponseHeader) SetStatusMessage(statusMessage []byte) { // SetStatusMessage sets response status message bytes.

	h.statusMessage = zerocopy.InitHeaderValueBytes(h.statusMessage, statusMessage)
}

func (h *ResponseHeader) SetProtocol(protocol []byte) { // SetProtocol sets response protocol bytes.

	h.protocol = zerocopy.InitHeaderValueBytes(h.protocol, protocol)
}

func (h *ResponseHeader) SetLastModified(t time.Time) {
	h.bufV = zerocopy.AppendHTTPDate(h.bufV[: // SetLastModified sets 'Last-Modified' header to the given value.
	0], t)
	h.setNonSpecial(zerocopy.StrLastModified, h.bufV)
}

func (h *ResponseHeader) ConnectionUpgrade() bool {
	return hasHeaderValue(h.Peek(HeaderConnection), zerocopy.StrUpgrade)
} // ConnectionUpgrade returns true if 'Connection: Upgrade' header is set.

func (h *ResponseHeader) PeekCookie(key string) []byte { // PeekCookie is able to returns cookie by a given key from response.

	return zerocopy.PeekArgStr(h.cookies, key)
}

func (h *ResponseHeader) ContentLength() int {
	return h.contentLength
} // ContentLength returns Content-Length header value.
//
// It may be negative:
// -1 means Transfer-Encoding: chunked.
// -2 means Transfer-Encoding: identity.

func (h *ResponseHeader) SetContentLength(contentLength int) {
	if h.mustSkipContentLength() {
		return
	}
	h.contentLength = contentLength
	if contentLength >= 0 {
		h.contentLengthBytes = zerocopy.AppendUint(h.contentLengthBytes[: // SetContentLength sets Content-Length header value.
		//
		// Content-Length may be negative:
		// -1 means Transfer-Encoding: chunked.
		// -2 means Transfer-Encoding: identity.
		0], contentLength)
		h.h.Del(HeaderTransferEncoding)
		return
	} else if contentLength == -1 {
		h.contentLengthBytes = h.contentLengthBytes[:0]
		setArgBytesHeaders(&h.h, zerocopy.StrTransferEncoding, zerocopy.StrChunked, zerocopy.ArgsHasValue)
		return
	}
	h.SetConnectionClose()
}

func (h *ResponseHeader) mustSkipContentLength() bool {
	statusCode := h.StatusCode()
	if statusCode < 100 || statusCode == StatusOK {
		return false
	}
	return statusCode == StatusNotModified || statusCode == StatusNoContent || statusCode < 200
}

func (h *ResponseHeader) isCompressibleContentType() bool {
	contentType := h.ContentType()
	return bytes.HasPrefix(contentType, zerocopy.StrTextSlash) || bytes.HasPrefix(contentType, zerocopy.StrApplicationSlash) || bytes.HasPrefix(contentType, zerocopy.StrImageSVG) || bytes.HasPrefix(contentType, zerocopy.StrImageIcon) || bytes.HasPrefix(contentType, zerocopy.StrFontSlash) || bytes.HasPrefix(contentType, zerocopy.StrMultipartSlash)
}

func (h *ResponseHeader) ContentType() []byte { // ContentType returns Content-Type header value.

	contentType := h.contentType
	if !h.noDefaultContentType && len(h.contentType) == 0 {
		contentType = zerocopy.DefaultContentType
	}
	return contentType
}

func (h *ResponseHeader) ContentEncoding() []byte { // ContentEncoding returns Content-Encoding header value.

	return h.contentEncoding
}

func (h *ResponseHeader) SetContentEncoding(contentEncoding string) {
	h.contentEncoding = zerocopy.InitHeaderValueString(h.contentEncoding, contentEncoding)
} // SetContentEncoding sets Content-Encoding header value.

func (h *ResponseHeader) SetContentEncodingBytes(contentEncoding []byte) { // SetContentEncodingBytes sets Content-Encoding header value.

	h.contentEncoding = zerocopy.InitHeaderValueBytes(h.contentEncoding, contentEncoding)
}

func (h *ResponseHeader) addVaryBytes(value []byte) { // addVaryBytes add value to the 'Vary' header if it's not included.

	v := h.peek(zerocopy.StrVary)
	if len(v) == 0 {
		h.SetBytesV(HeaderVary, value)
	} else if !bytes.Contains(v, value) {
		h.SetBytesV(HeaderVary, append(append(v, ','), value...))
	}
}

func (h *ResponseHeader) Server() []byte { // Server returns Server header value.

	return h.server
}

func (h *ResponseHeader) SetServer(server string) {
	h.server = zerocopy.InitHeaderValueString(h.server, server)
} // SetServer sets Server header value.

func (h *ResponseHeader) SetServerBytes(server []byte) { // SetServerBytes sets Server header value.

	h.server = zerocopy.InitHeaderValueBytes(h.server, server)
}

func (h *ResponseHeader) Len() int {
	n := 0
	for range // Len returns the number of headers set,
	// i.e. the number of times f is called in VisitAll.
	h.All() {
		n++
	}
	return n
}

func (h *ResponseHeader) Reset() {
	h.disableNormalizing = false
	h.SetNoDefaultContentType(false)
	h.noDefaultDate = false
	h.resetSkipNormalize()
} // Reset clears response header.

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

func (h *ResponseHeader) CopyTo(dst *ResponseHeader) {
	dst.Reset()
	h.copyTo(&dst.header)
	dst.noDefaultDate = h.noDefaultDate
	dst.statusCode = h.statusCode
	dst.statusMessage = append(dst.statusMessage, h.statusMessage...)
	dst.contentEncoding = append(dst.contentEncoding, h.contentEncoding...)
	dst.server = append(dst.server, h.server...)
} // CopyTo copies all the headers to dst.

func (h *ResponseHeader) All() iter.Seq2[[]byte, // All returns an iterator over key-value pairs in h.
	// The key and value may invalid outside the iteration loop.
	// Copy key and/or value contents for each iteration if you need retaining
	// them.
	//
	// Making modifications to the ResponseHeader during the iteration loop leads to undefined
	// behavior and can cause panics.
	[]byte] {
	return func(yield func([]byte, []byte) bool) {
		if len(h.contentLengthBytes) > 0 && !yield(zerocopy.StrContentLength, h.contentLengthBytes) {
			return
		}
		if contentType := h.ContentType(); len(contentType) > 0 && !yield(zerocopy.StrContentType, contentType) {
			return
		}
		if contentEncoding := h.ContentEncoding(); len(contentEncoding) > 0 && !yield(zerocopy.StrContentEncoding, contentEncoding) {
			return
		}
		if server := h.Server(); len(server) > 0 && !yield(zerocopy.StrServer, server) {
			return
		}
		for i := range h.cookies {
			if !yield(zerocopy.StrSetCookie, h.cookies[i].Value) {
				return
			}
		}
		if len(h.trailer) > 0 && !yield(zerocopy.StrTrailer, appendTrailerBytes(nil, h.trailer, zerocopy.StrCommaSpace)) {
			return
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

func (h *ResponseHeader) Cookies() iter.Seq2[[]byte, // Cookies returns an iterator over key-value paired response cookie in h.
	// zerocopy.Cookie name is passed in key and the whole Set-zerocopy.Cookie header value
	// is passed in value for each iteration. Value may be parsed with
	// zerocopy.Cookie.ParseBytes().
	//
	// The key and value may invalid outside the iteration loop.
	// Copy key and/or value contents for each iteration if you need retaining
	// them.
	//
	// Making modifications to the ResponseHeader during the iteration loop leads to undefined
	// behavior and can cause panics.
	[]byte] {
	return func(yield func([]byte, []byte) bool) {
		for i := range h.cookies {
			if !yield(h.cookies[i].Key, h.cookies[i].Value) {
				break
			}
		}
	}
}

func (h *ResponseHeader) Del(key string) {
	h.bufK = zerocopy.GetHeaderKeyBytes(h.bufK, key, h.disableNormalizing)
	h.del(h.bufK)
} // Del deletes header with the given key.

func (h *ResponseHeader) DelBytes(key []byte) { // DelBytes deletes header with the given key.

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

func (h *ResponseHeader) setSpecialHeader(key, value []byte) bool { // setSpecialHeader handles special headers and return true when a header is processed.

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
			if bytes.Equal(zerocopy.StrClose, value) {
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

func (h *ResponseHeader) Add(key, value string) {
	h.AddBytesKV(bytesconv.S2B(key), bytesconv.S2B(value))
} // Add adds the given 'Key: value' header.
//
// Multiple headers with the same key may be added with this function.
// Use Set for setting a single header for the given key.
//
// the Content-Type, Content-Length, Connection, Server, Transfer-Encoding
// and Date headers can only be set once and will overwrite the previous value,
// while Set-zerocopy.Cookie will not clear previous cookies.
//
// If the header is set as a Trailer (forbidden trailers will not be set, see AddTrailer for more details),
// it will be sent after the chunked response body.

func (h *ResponseHeader) AddBytesK(key []byte, // AddBytesK adds the given 'Key: value' header.
	//
	// Multiple headers with the same key may be added with this function.
	// Use SetBytesK for setting a single header for the given key.
	//
	// the Content-Type, Content-Length, Connection, Server, Transfer-Encoding
	// and Date headers can only be set once and will overwrite the previous value,
	// while Set-zerocopy.Cookie will not clear previous cookies.
	//
	// If the header is set as a Trailer (forbidden trailers will not be set, see AddTrailer for more details),
	// it will be sent after the chunked response body.
	value string) {
	h.AddBytesKV(key, bytesconv.S2B(value))
}

func (h *ResponseHeader) AddBytesV(key string, value []byte) { // AddBytesV adds the given 'Key: value' header.
	//
	// Multiple headers with the same key may be added with this function.
	// Use SetBytesV for setting a single header for the given key.
	//
	// the Content-Type, Content-Length, Connection, Server, Transfer-Encoding
	// and Date headers can only be set once and will overwrite the previous value,
	// while Set-zerocopy.Cookie will not clear previous cookies.
	//
	// If the header is set as a Trailer (forbidden trailers will not be set, see AddTrailer for more details),
	// it will be sent after the chunked response body.

	h.AddBytesKV(bytesconv.S2B(key), value)
}

func (h *ResponseHeader) AddBytesKV(key, value []byte) { // AddBytesKV adds the given 'Key: value' header.
	//
	// Multiple headers with the same key may be added with this function.
	// Use SetBytesKV for setting a single header for the given key.
	//
	// the Content-Type, Content-Length, Connection, Server, Transfer-Encoding
	// and Date headers can only be set once and will overwrite the previous value,
	// while the Set-zerocopy.Cookie header will not clear previous cookies.
	//
	// If the header is set as a Trailer (forbidden trailers will not be set, see AddTrailer for more details),
	// it will be sent after the chunked response body.

	h.bufK, h.bufV = zerocopy.InitHeaderKV(h.bufK, h.bufV, bytesconv.B2S(key), bytesconv.B2S(value), h.disableNormalizing)
	if h.setSpecialHeader(h.bufK, h.bufV) {
		return
	}
	appendArgBytesHeaders(&h.h, h.bufK, h.bufV, zerocopy.ArgsHasValue)
}

func (h *ResponseHeader) Set(key, value string) {
	h.bufK, h.bufV = zerocopy.InitHeaderKV(h.bufK, h.bufV, key, value, h.disableNormalizing)
	h.SetCanonical(h.bufK, h.bufV)
} // Set sets the given 'Key: value' header.
//
// Please note that the Set-zerocopy.Cookie header will not clear previous cookies,
// use SetCookie instead to reset cookies.
//
// If the header is set as a Trailer (forbidden trailers will not be set, see SetTrailer for more details),
// it will be sent after the chunked response body.
//
// Use Add for setting multiple header values under the same key.

func (h *ResponseHeader) SetBytesK(key []byte, // SetBytesK sets the given 'Key: value' header.
	//
	// Please note that the Set-zerocopy.Cookie header will not clear previous cookies,
	// use SetCookie instead to reset cookies.
	//
	// If the header is set as a Trailer (forbidden trailers will not be set, see SetTrailer for more details),
	// it will be sent after the chunked response body.
	//
	// Use AddBytesK for setting multiple header values under the same key.
	value string) {
	h.bufV = append(h.bufV[:0], value...)
	h.SetBytesKV(key, h.bufV)
}

func (h *ResponseHeader) SetBytesV(key string, value []byte) { // SetBytesV sets the given 'Key: value' header.
	//
	// Please note that the Set-zerocopy.Cookie header will not clear previous cookies,
	// use SetCookie instead to reset cookies.
	//
	// If the header is set as a Trailer (forbidden trailers will not be set, see SetTrailer for more details),
	// it will be sent after the chunked response body.
	//
	// Use AddBytesV for setting multiple header values under the same key.

	h.bufK = zerocopy.GetHeaderKeyBytes(h.bufK, key, h.disableNormalizing)
	h.SetCanonical(h.bufK, value)
}

func (h *ResponseHeader) SetBytesKV(key, value []byte) { // SetBytesKV sets the given 'Key: value' header.
	//
	// Please note that the Set-zerocopy.Cookie header will not clear previous cookies,
	// use SetCookie instead to reset cookies.
	//
	// If the header is set as a Trailer (forbidden trailers will not be set, see SetTrailer for more details),
	// it will be sent after the chunked response body.
	//
	// Use AddBytesKV for setting multiple header values under the same key.

	h.bufK = append(h.bufK[:0], key...)
	zerocopy.NormalizeHeaderKey(h.bufK, h.disableNormalizing)
	h.SetCanonical(h.bufK, value)
}

func (h *ResponseHeader) SetCanonical(key, value []byte) { // SetCanonical sets the given 'Key: value' header assuming that
	// key is in canonical form.
	//
	// Please note that the Set-zerocopy.Cookie header will not clear previous cookies,
	// use SetCookie instead to reset cookies.
	//
	// If the header is set as a Trailer (forbidden trailers will not be set, see SetTrailer for more details),
	// it will be sent after the chunked response body.

	h.bufV = zerocopy.InitHeaderValueBytes(h.bufV, value)
	if h.setSpecialHeader(key, h.bufV) {
		return
	}
	h.setNonSpecial(key, h.bufV)
}

func (h *ResponseHeader) SetCookie(cookie *zerocopy.Cookie) {
	h.bufK = zerocopy.InitHeaderValueBytes(h.bufK, cookie.Key())
	h.bufV = zerocopy.InitHeaderValueBytes(h.bufV, cookie.Cookie())
	h.cookies = zerocopy.SetArgBytes(h.cookies, h.bufK, h.bufV, zerocopy.ArgsHasValue)
} // SetCookie sets the given response cookie.
//
// It is safe re-using the cookie after the function returns.

func (h *ResponseHeader) DelClientCookie(key string) {
	h.DelCookie(key)
	c := zerocopy.AcquireCookie()
	c.SetKey(key)
	c.SetExpire(zerocopy.CookieExpireDelete)
	h.SetCookie(c)
	zerocopy.ReleaseCookie(c)
} // DelClientCookie instructs the client to remove the given cookie.
// This doesn't work for a cookie with specific domain or path,
// you should delete it manually like:
//
//	c := zerocopy.AcquireCookie()
//	c.SetKey(key)
//	c.SetDomain("example.com")
//	c.SetPath("/path")
//	c.SetExpire(zerocopy.CookieExpireDelete)
//	h.SetCookie(c)
//	zerocopy.ReleaseCookie(c)
//
// Use DelCookie if you want just removing the cookie from response header.

func (h *ResponseHeader) DelClientCookieBytes(key []byte) { // DelClientCookieBytes instructs the client to remove the given cookie.
	// This doesn't work for a cookie with specific domain or path,
	// you should delete it manually like:
	//
	//	c := zerocopy.AcquireCookie()
	//	c.SetKey(key)
	//	c.SetDomain("example.com")
	//	c.SetPath("/path")
	//	c.SetExpire(zerocopy.CookieExpireDelete)
	//	h.SetCookie(c)
	//	zerocopy.ReleaseCookie(c)
	//
	// Use DelCookieBytes if you want just removing the cookie from response header.

	h.DelClientCookie(bytesconv.B2S(key))
}

func (h *ResponseHeader) DelCookie(key string) {
	h.cookies = zerocopy.DelAllArgs(h.cookies, key)
} // DelCookie removes cookie under the given key from response header.
//
// Note that DelCookie doesn't remove the cookie from the client.
// Use DelClientCookie instead.

func (h *ResponseHeader) DelCookieBytes(key []byte) { // DelCookieBytes removes cookie under the given key from response header.
	//
	// Note that DelCookieBytes doesn't remove the cookie from the client.
	// Use DelClientCookieBytes instead.

	h.DelCookie(bytesconv.B2S(key))
}

func (h *ResponseHeader) DelAllCookies() {
	h.cookies = h.cookies[: // DelAllCookies removes all the cookies from response headers.
	0]
}

func (h *ResponseHeader) Peek(key string) []byte { // Peek returns header value for the given key.
	//
	// The returned value is valid until the response is released,
	// either though ReleaseResponse or your request handler returning.
	// Do not store references to the returned value. Make copies instead.

	h.bufK = zerocopy.GetHeaderKeyBytes(h.bufK, key, h.disableNormalizing)
	return h.peek(h.bufK)
}

func (h *ResponseHeader) PeekBytes(key []byte) []byte { // PeekBytes returns header value for the given key.
	//
	// The returned value is valid until the response is released,
	// either though ReleaseResponse or your request handler returning.
	// Do not store references to returned value. Make copies instead.

	h.bufK = append(h.bufK[:0], key...)
	zerocopy.NormalizeHeaderKey(h.bufK, h.disableNormalizing)
	return h.peek(h.bufK)
}

func (h *ResponseHeader) PeekCanonical(key []byte) []byte { // PeekCanonical returns header value for the given key without normalizing it.
	// The key must match the canonical form used with SetCanonical.
	//
	// The returned value is valid until the response is released,
	// either though ReleaseResponse or your request handler returning.
	// Do not store references to the returned value. Make copies instead.

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

func (h *ResponseHeader) PeekAll(key string) [][]byte { // PeekAll returns all header value for the given key.
	//
	// The returned value is valid until the request is released,
	// either though ReleaseResponse or your request handler returning.
	// Any future calls to the Peek* will modify the returned value.
	// Do not store references to returned value. Make copies instead.

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

func (h *ResponseHeader) PeekKeys() [][]byte { // PeekKeys return all header keys.
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

func (h *ResponseHeader) Cookie(cookie *zerocopy.Cookie) bool {
	v := zerocopy.PeekArgBytes(h.cookies, cookie.Key())
	if v == nil {
		return false
	}
	cookie.ParseBytes(v)
	return true
} // zerocopy.Cookie fills cookie for the given cookie.Key.
//
// Returns false if cookie with the given cookie.Key is missing.

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
} // Read reads response header from r.
//
// io.EOF is returned if r is closed before reading the first header byte.

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

func (h *ResponseHeader) Write(w *bufio.Writer) error {
	_, err := w.Write(h.Header())
	return err
} // Write writes response header to w.

func (h *ResponseHeader) WriteTo(w io.Writer) (int64, error) {
	n, err := w.Write(h.Header())
	return int64(n), err
} // WriteTo writes response header to w.
//
// WriteTo implements io.WriterTo interface.

func (h *ResponseHeader) Header() []byte { // Header returns response header representation.
	//
	// Headers that set as Trailer will not represent. Use TrailerHeader for trailers.
	//
	// The returned value is valid until the request is released,
	// either though ReleaseRequest or your request handler returning.
	// Do not store references to returned value. Make copies instead.

	h.bufV = h.AppendBytes(h.bufV[:0])
	return h.bufV
}

func (h *ResponseHeader) writeTrailer(w *bufio.Writer) error {
	_, err := w.Write(h.TrailerHeader())
	return err
} // writeTrailer writes response trailer to w.

func (h *ResponseHeader) TrailerHeader() []byte { // TrailerHeader returns response trailer header representation.
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

func (h *ResponseHeader) String() string {
	return string(h.Header())
} // String returns response header representation.

func (h *ResponseHeader) appendStatusLine(dst []byte) []byte { // appendStatusLine appends the response status line to dst and returns
	// the extended dst.

	statusCode := h.StatusCode()
	if statusCode < 0 {
		statusCode = StatusOK
	}
	return formatStatusLine(dst, h.Protocol(), statusCode, h.StatusMessage())
}

func (h *ResponseHeader) AppendBytes(dst []byte) []byte { // AppendBytes appends response header representation to dst and returns
	// the extended dst.

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
	var s headerScanner
	s.b = buf
	var kv *zerocopy.ArgsKV
	transferEncodingSeen := false
	contentLengthSeen := false
	for s.next() {
		s.key = trimTrailingSpace(s.key)
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
					h.connectionClose = true
					return 0, ErrDuplicateContentLength
				}
				contentLengthSeen = true
				var err error
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
				if bytes.Equal(s.value, zerocopy.StrClose) {
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
	if h.noHTTP11 && !h.connectionClose {
		v := peekArgBytesHeaders(&h.h, zerocopy.StrConnection)
		h.connectionClose = !hasHeaderValue(v, zerocopy.StrKeepAlive)
	}
	return s.r, nil
}

func (h *ResponseHeader) PeekScoped(s *borrow.Scope, key string) borrow.Bytes {
	b := h.Peek(key)
	if len(b) == 0 {
		return borrow.Bytes{}
	}
	return borrow.NewBytes(b, nil)
} // Borrow methods moved from borrow.go
// PeekScoped borrows the header value associated with key into the given borrow scope.

func (h *ResponseHeader) CookieScoped(s *borrow.Scope, key string) borrow.Bytes {
	var c zerocopy.Cookie
	c.SetKey(key)
	if !h.Cookie(&c) {
		return borrow.Bytes{}
	}
	b := c.Value()
	if len(b) == 0 {
		return borrow.Bytes{}
	}
	return borrow.NewBytes(b, nil)
} // CookieScoped borrows the cookie value associated with key into the given borrow scope.

func (h *ResponseHeader) PeekAllScoped(s *borrow.Scope, key string) []borrow. // PeekAllScoped borrows all header values associated with key into a slice of borrowed bytes.
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

func (h *ResponseHeader) TrailerScoped(s *borrow.Scope, key string) borrow.Bytes {
	b := h.Peek(key)
	if len(b) == 0 {
		return borrow.Bytes{}
	}
	return borrow.NewBytes(b, nil)
} // TrailerScoped borrows the trailer value associated with key into the given borrow scope.

func (h *ResponseHeader) SetDisableNormalizing(disable bool) {
	h.disableNormalizing = disable
}
