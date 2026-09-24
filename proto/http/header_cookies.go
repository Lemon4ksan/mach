// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package http

import (
	"iter"

	"github.com/lemon4ksan/foundation/silicon/bytesconv"

	"github.com/lemon4ksan/mach/proto/http/zerocopy"
)

// Cookie returns the value of the request cookie identified by key (RFC 6265 Section 4.2.1, Section 5.4).
// Returns nil if no cookie matches. The returned slice is volatile and tied to request lifetime.
//
// Thread-safe: No.
func (h *RequestHeader) Cookie(key string) []byte {
	h.collectCookies()
	return zerocopy.PeekArgStr(h.cookies, key)
}

// CookieBytes returns the value of the request cookie identified by key bytes (RFC 6265 Section 5.4).
//
// Thread-safe: No.
func (h *RequestHeader) CookieBytes(key []byte) []byte {
	h.collectCookies()
	return zerocopy.PeekArgBytes(h.cookies, key)
}

// Cookies returns an iterator yielding cookie name and value byte pairs (RFC 6265 Section 5.4).
// Modifying RequestHeader during iteration causes undefined behavior.
//
// Thread-safe: No.
func (h *RequestHeader) Cookies() iter.Seq2[[]byte, []byte] {
	return func(yield func([]byte, []byte) bool) {
		h.collectCookies()

		for i := range h.cookies {
			if !yield(h.cookies[i].Key, h.cookies[i].Value) {
				break
			}
		}
	}
}

// SetCookie sets a request cookie key-value pair into the Cookie header block (RFC 6265 Section 4.2.1).
//
// Thread-safe: No.
func (h *RequestHeader) SetCookie(key, value string) {
	h.collectCookies()
	h.bufK = zerocopy.InitHeaderValueString(h.bufK, key)
	h.bufV = zerocopy.InitHeaderValueString(h.bufV, value)
	h.cookies = zerocopy.SetArgBytes(h.cookies, h.bufK, h.bufV, zerocopy.ArgsHasValue)
}

// SetCookieBytesK sets a request cookie with byte key and string value (RFC 6265 Section 4.2.1).
//
// Thread-safe: No.
func (h *RequestHeader) SetCookieBytesK(key []byte, value string) {
	h.SetCookie(bytesconv.B2S(key), value)
}

// SetCookieBytesKV sets a request cookie with byte key and byte value (RFC 6265 Section 4.2.1).
//
// Thread-safe: No.
func (h *RequestHeader) SetCookieBytesKV(key, value []byte) {
	h.SetCookie(bytesconv.B2S(key), bytesconv.B2S(value))
}

// DelCookie removes the cookie matching key from request headers (RFC 6265 Section 5.4).
//
// Thread-safe: No.
func (h *RequestHeader) DelCookie(key string) {
	h.collectCookies()
	h.cookies = zerocopy.DelAllArgs(h.cookies, key)
}

// DelCookieBytes removes the cookie matching key bytes from request headers (RFC 6265 Section 5.4).
//
// Thread-safe: No.
func (h *RequestHeader) DelCookieBytes(key []byte) {
	h.DelCookie(bytesconv.B2S(key))
}

// DelAllCookies removes all cookies from the request header (RFC 6265 Section 5.4).
//
// Thread-safe: No.
func (h *RequestHeader) DelAllCookies() {
	h.collectCookies()
	h.cookies = h.cookies[:0]
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

// Cookie populates cookie with the Set-Cookie definition matching cookie.Key (RFC 6265 Section 4.1).
// Returns false if no cookie matching cookie.Key is found.
//
// Thread-safe: No.
func (h *ResponseHeader) Cookie(cookie *zerocopy.Cookie) bool {
	v := zerocopy.PeekArgBytes(h.cookies, cookie.Key())
	if v == nil {
		return false
	}

	return cookie.ParseBytes(v) == nil
}

// PeekCookie returns the raw Set-Cookie header value matching key (RFC 6265 Section 4.1).
// Returns nil if no cookie matches key.
//
// Thread-safe: No.
func (h *ResponseHeader) PeekCookie(key string) []byte {
	return zerocopy.PeekArgStr(h.cookies, key)
}

// Cookies returns an iterator yielding cookie name and value byte pairs (RFC 6265 Section 4.1).
// Modifying ResponseHeader during iteration causes undefined behavior.
//
// Thread-safe: No.
func (h *ResponseHeader) Cookies() iter.Seq2[[]byte, []byte] {
	return func(yield func([]byte, []byte) bool) {
		for i := range h.cookies {
			if !yield(h.cookies[i].Key, h.cookies[i].Value) {
				break
			}
		}
	}
}

// SetCookie sets a Set-Cookie response header field (RFC 6265 Section 4.1).
// The passed cookie is copied and may be safely re-used by the caller immediately.
//
// Thread-safe: No.
func (h *ResponseHeader) SetCookie(cookie *zerocopy.Cookie) {
	h.bufK = zerocopy.InitHeaderValueBytes(h.bufK, cookie.Key())
	h.bufV = zerocopy.InitHeaderValueBytes(h.bufV, cookie.Cookie())
	h.cookies = zerocopy.SetArgBytes(h.cookies, h.bufK, h.bufV, zerocopy.ArgsHasValue)
}

// DelClientCookie appends a Set-Cookie directive commanding the user agent to expire and evict the specified cookie (RFC 6265 Section 5.3).
//
// Thread-safe: No.
func (h *ResponseHeader) DelClientCookie(key string) {
	h.DelCookie(key)

	c := zerocopy.AcquireCookie()
	c.SetKey(key)
	c.SetExpire(zerocopy.CookieExpireDelete)
	h.SetCookie(c)
	zerocopy.ReleaseCookie(c)
}

// DelClientCookieBytes appends a Set-Cookie directive commanding the user agent to expire and evict the specified cookie bytes (RFC 6265 Section 5.3).
//
// Thread-safe: No.
func (h *ResponseHeader) DelClientCookieBytes(key []byte) {
	h.DelClientCookie(bytesconv.B2S(key))
}

// DelCookie removes the Set-Cookie header matching key from the response (RFC 6265 Section 4.1).
// Note that this does not instruct the client to delete the cookie; use DelClientCookie instead.
//
// Thread-safe: No.
func (h *ResponseHeader) DelCookie(key string) {
	h.cookies = zerocopy.DelAllArgs(h.cookies, key)
}

// DelCookieBytes removes the Set-Cookie header matching key bytes from the response (RFC 6265 Section 4.1).
//
// Thread-safe: No.
func (h *ResponseHeader) DelCookieBytes(key []byte) {
	h.DelCookie(bytesconv.B2S(key))
}

// DelAllCookies removes all Set-Cookie headers from the response (RFC 6265 Section 4.1).
//
// Thread-safe: No.
func (h *ResponseHeader) DelAllCookies() {
	h.cookies = h.cookies[:0]
}
