// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package http

import (
	"github.com/lemon4ksan/foundation/borrow"
	"github.com/lemon4ksan/foundation/net/http/zerocopy"
)

// PeekScoped borrows the header value associated with key into scope s (RFC 9110 Section 5.1).
//
// Lifecycle:
// The returned borrow.Bytes reference is valid strictly within the lifetime of borrow.Scope s.
// It avoids heap allocation by borrowing directly from the underlying header buffer without retain copying.
func (h *RequestHeader) PeekScoped(s *borrow.Scope, key string) borrow.Bytes {
	b := h.Peek(key)
	if len(b) == 0 {
		return borrow.Bytes{}
	}

	return borrow.NewBytes(b, nil)
}

// CookieScoped borrows the cookie value associated with key into scope s (RFC 6265 Section 5.4).
func (h *RequestHeader) CookieScoped(s *borrow.Scope, key string) borrow.Bytes {
	b := h.Cookie(key)
	if len(b) == 0 {
		return borrow.Bytes{}
	}

	return borrow.NewBytes(b, nil)
}

// PeekAllScoped borrows all values matching key as a slice of borrow.Bytes in scope s (RFC 9110 Section 5.2).
func (h *RequestHeader) PeekAllScoped(s *borrow.Scope, key string) []borrow.Bytes {
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

// TrailerScoped borrows the trailing header value associated with key into scope s (RFC 9112 Section 7.1.2).
func (h *RequestHeader) TrailerScoped(s *borrow.Scope, key string) borrow.Bytes {
	b := h.Peek(key)
	if len(b) == 0 {
		return borrow.Bytes{}
	}

	return borrow.NewBytes(b, nil)
}

// PeekScoped borrows the response header value associated with key into scope s (RFC 9110 Section 5.1).
func (h *ResponseHeader) PeekScoped(s *borrow.Scope, key string) borrow.Bytes {
	b := h.Peek(key)
	if len(b) == 0 {
		return borrow.Bytes{}
	}

	return borrow.NewBytes(b, nil)
}

// CookieScoped borrows the response cookie value associated with key into scope s (RFC 6265 Section 4.1).
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
}

// PeekAllScoped borrows all response values matching key into scope s (RFC 9110 Section 5.2).
func (h *ResponseHeader) PeekAllScoped(s *borrow.Scope, key string) []borrow.Bytes {
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

// TrailerScoped borrows the response trailer value associated with key into scope s (RFC 9112 Section 7.1.2).
func (h *ResponseHeader) TrailerScoped(s *borrow.Scope, key string) borrow.Bytes {
	b := h.Peek(key)
	if len(b) == 0 {
		return borrow.Bytes{}
	}

	return borrow.NewBytes(b, nil)
}
