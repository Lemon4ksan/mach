// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"io"
	"iter"
	"net/http"
	"strings"

	"github.com/lemon4ksan/foundation/silicon/bytesconv"

	"github.com/lemon4ksan/mach/proto/headkit"
	"github.com/lemon4ksan/mach/proto/semantic/status"
	"github.com/lemon4ksan/mach/server/h1"
	"github.com/lemon4ksan/mach/server/h2"
	"github.com/lemon4ksan/mach/server/h3"
)

// Protocol version identifiers.
const (
	ProtoHTTP11 = "HTTP/1.1"
	ProtoHTTP2  = "HTTP/2"
	ProtoHTTP3  = "HTTP/3"
)

// Context represents the unified request and response exchange across HTTP/1.1, HTTP/2, and HTTP/3.
//
// Concurrency:
// Instances are pooled and must not be retained or used concurrently across goroutines without
// synchronization.
type Context struct {
	server *Server
	proto  string

	// Protocol-specific request and response bindings
	h1Req *h1.Request
	h1Res *h1.Response

	h2Req *h2.ServerRequest
	h2Res *h2.ServerResponse

	h3Req *h3.ServerRequest
	h3Res *h3.ServerResponse

	// Cached parsed paths/queries
	path      string
	query     string
	parsedURI bool

	// Stream or custom context override
	ctx context.Context

	// User values key-value store
	userValues map[any]any

	// Per-request Alt-Svc suppression
	disableAltSvc bool
}

// Proto returns the protocol identifier for the current exchange ("HTTP/1.1", "HTTP/2", or "HTTP/3").
func (c *Context) Proto() string {
	return c.proto
}

// IsH1 reports whether the request was received over HTTP/1.1.
func (c *Context) IsH1() bool {
	return c.proto == ProtoHTTP11
}

// IsH2 reports whether the request was received over HTTP/2.
func (c *Context) IsH2() bool {
	return c.proto == ProtoHTTP2
}

// IsH3 reports whether the request was received over HTTP/3.
func (c *Context) IsH3() bool {
	return c.proto == ProtoHTTP3
}

// Method returns the HTTP request method (e.g. "GET", "POST").
func (c *Context) Method() string {
	switch c.proto {
	case ProtoHTTP11:
		if c.h1Req != nil {
			return c.h1Req.Method
		}
	case ProtoHTTP2:
		if c.h2Req != nil {
			return c.h2Req.Method
		}
	case ProtoHTTP3:
		if c.h3Req != nil {
			return c.h3Req.Method
		}
	}

	return ""
}

func (c *Context) parseURI() {
	if c.parsedURI {
		return
	}

	var raw string
	switch c.proto {
	case ProtoHTTP11:
		if c.h1Req != nil {
			c.path = c.h1Req.Path
			c.query = c.h1Req.Query
			c.parsedURI = true

			return
		}

	case ProtoHTTP2:
		if c.h2Req != nil {
			raw = c.h2Req.Path
		}
	case ProtoHTTP3:
		if c.h3Req != nil {
			raw = c.h3Req.Path
		}
	}

	if idx := strings.IndexByte(raw, '?'); idx != -1 {
		c.path = raw[:idx]
		c.query = raw[idx+1:]
	} else {
		c.path = raw
		c.query = ""
	}

	c.parsedURI = true
}

// Path returns the request path component, excluding query parameters.
func (c *Context) Path() string {
	c.parseURI()
	return c.path
}

// Query returns the raw query string, excluding the leading '?'.
func (c *Context) Query() string {
	c.parseURI()
	return c.query
}

// RequestURI returns the full request target including path and query string.
func (c *Context) RequestURI() string {
	switch c.proto {
	case ProtoHTTP11:
		if c.h1Req != nil {
			if c.h1Req.Query != "" {
				return c.h1Req.Path + "?" + c.h1Req.Query
			}

			return c.h1Req.Path
		}

	case ProtoHTTP2:
		if c.h2Req != nil {
			return c.h2Req.Path
		}
	case ProtoHTTP3:
		if c.h3Req != nil {
			return c.h3Req.Path
		}
	}

	return ""
}

// Host returns the Host or :authority target host of the request.
func (c *Context) Host() string {
	switch c.proto {
	case ProtoHTTP11:
		if c.h1Req != nil {
			return c.h1Req.Headers.Get("Host")
		}
	case ProtoHTTP2:
		if c.h2Req != nil {
			if c.h2Req.Authority != "" {
				return c.h2Req.Authority
			}

			return c.h2Req.Headers.Get("Host")
		}

	case ProtoHTTP3:
		if c.h3Req != nil {
			if c.h3Req.Authority != "" {
				return c.h3Req.Authority
			}

			return c.h3Req.Headers.Get("Host")
		}
	}

	return ""
}

// Authority returns the target authority of the request.
func (c *Context) Authority() string {
	return c.Host()
}

// Scheme returns the request scheme ("http" or "https").
func (c *Context) Scheme() string {
	switch c.proto {
	case ProtoHTTP11:
		if c.h1Req != nil && c.h1Req.TLS != nil {
			return "https"
		}

		return "http"

	case ProtoHTTP2:
		if c.h2Req != nil {
			if c.h2Req.Scheme != "" {
				return c.h2Req.Scheme
			}

			return "https"
		}

	case ProtoHTTP3:
		if c.h3Req != nil {
			if c.h3Req.Scheme != "" {
				return c.h3Req.Scheme
			}

			return "https"
		}
	}

	return "http"
}

// RemoteAddr returns the peer remote network address string.
func (c *Context) RemoteAddr() string {
	switch c.proto {
	case ProtoHTTP11:
		if c.h1Req != nil {
			if c.h1Req.RemoteAddr != "" {
				return c.h1Req.RemoteAddr
			}

			if c.h1Req.Conn != nil && c.h1Req.Conn.RemoteAddr() != nil {
				return c.h1Req.Conn.RemoteAddr().String()
			}
		}

	case ProtoHTTP2:
		if c.h2Req != nil {
			return c.h2Req.RemoteAddr
		}
	case ProtoHTTP3:
		if c.h3Req != nil {
			return c.h3Req.RemoteAddr
		}
	}

	return ""
}

// Context returns the stream or request context.
func (c *Context) Context() context.Context {
	if c.ctx != nil {
		return c.ctx
	}

	switch c.proto {
	case ProtoHTTP2:
		if c.h2Req != nil && c.h2Req.Ctx != nil {
			return c.h2Req.Ctx
		}
	case ProtoHTTP3:
		if c.h3Req != nil && c.h3Req.Ctx != nil {
			return c.h3Req.Ctx
		}
	}

	return context.Background()
}

// SetContext sets a custom context for the request.
func (c *Context) SetContext(ctx context.Context) {
	c.ctx = ctx
}

// TLS returns the TLS connection state if the request was received over TLS.
func (c *Context) TLS() *tls.ConnectionState {
	if c.proto == ProtoHTTP11 && c.h1Req != nil {
		return c.h1Req.TLS
	}

	return nil
}

// StreamID returns the stream identifier for multiplexed protocols (H2/H3), or 0 for H1.
func (c *Context) StreamID() uint64 {
	switch c.proto {
	case ProtoHTTP2:
		if c.h2Req != nil {
			return uint64(c.h2Req.StreamID)
		}
	case ProtoHTTP3:
		if c.h3Req != nil {
			return c.h3Req.StreamID
		}
	}

	return 0
}

// Header returns the first request header value associated with key.
func (c *Context) Header(key string) string {
	switch c.proto {
	case ProtoHTTP11:
		if c.h1Req != nil {
			return c.h1Req.Headers.Get(key)
		}
	case ProtoHTTP2:
		if c.h2Req != nil {
			return c.h2Req.Headers.Get(key)
		}
	case ProtoHTTP3:
		if c.h3Req != nil {
			return c.h3Req.Headers.Get(key)
		}
	}

	return ""
}

// HasHeader reports whether the request contains a non-empty header for key.
func (c *Context) HasHeader(key string) bool {
	switch c.proto {
	case ProtoHTTP11:
		if c.h1Req != nil {
			return c.h1Req.Headers.Has(key)
		}
	case ProtoHTTP2:
		if c.h2Req != nil {
			return c.h2Req.Headers.Get(key) != ""
		}
	case ProtoHTTP3:
		if c.h3Req != nil {
			return c.h3Req.Headers.Has(key)
		}
	}

	return false
}

// RequestHeaders returns an iterator over all request header key-value pairs without heap allocations.
func (c *Context) RequestHeaders() iter.Seq2[string, string] {
	return func(yield func(string, string) bool) {
		switch c.proto {
		case ProtoHTTP11:
			if c.h1Req != nil {
				for k, v := range c.h1Req.Headers.All() {
					if !yield(k, v) {
						return
					}
				}
			}

		case ProtoHTTP2:
			if c.h2Req != nil {
				for k, vals := range c.h2Req.Headers {
					for _, v := range vals {
						if !yield(k, v) {
							return
						}
					}
				}
			}

		case ProtoHTTP3:
			if c.h3Req != nil {
				for k, v := range c.h3Req.Headers.All() {
					if !yield(k, v) {
						return
					}
				}
			}
		}
	}
}

// VisitRequestHeaders iterates over all request headers calling fn.
func (c *Context) VisitRequestHeaders(fn func(key, val string)) {
	for k, v := range c.RequestHeaders() {
		fn(k, v)
	}
}

// Body returns the request payload bytes without copying.
func (c *Context) Body() []byte {
	switch c.proto {
	case ProtoHTTP11:
		if c.h1Req != nil {
			return c.h1Req.Body
		}
	case ProtoHTTP2:
		if c.h2Req != nil {
			return c.h2Req.Body
		}
	case ProtoHTTP3:
		if c.h3Req != nil {
			return c.h3Req.Body
		}
	}

	return nil
}

// BodyReader returns an io.Reader over the request payload bytes.
func (c *Context) BodyReader() io.Reader {
	return bytes.NewReader(c.Body())
}

// SetStatusCode sets the HTTP response status code.
func (c *Context) SetStatusCode(code int) {
	switch c.proto {
	case ProtoHTTP11:
		if c.h1Res != nil {
			c.h1Res.StatusCode = code
		}
	case ProtoHTTP2:
		if c.h2Res != nil {
			c.h2Res.StatusCode = code
		}
	case ProtoHTTP3:
		if c.h3Res != nil {
			c.h3Res.StatusCode = code
		}
	}
}

// StatusCode returns the currently configured response status code. Defaults to 200 OK.
func (c *Context) StatusCode() int {
	switch c.proto {
	case ProtoHTTP11:
		if c.h1Res != nil {
			if c.h1Res.StatusCode != 0 {
				return c.h1Res.StatusCode
			}

			return status.OK
		}

	case ProtoHTTP2:
		if c.h2Res != nil {
			if c.h2Res.StatusCode != 0 {
				return c.h2Res.StatusCode
			}

			return status.OK
		}

	case ProtoHTTP3:
		if c.h3Res != nil {
			if c.h3Res.StatusCode != 0 {
				return c.h3Res.StatusCode
			}

			return status.OK
		}
	}

	return status.OK
}

// SetHeader sets the response header associated with key to val.
func (c *Context) SetHeader(key, val string) {
	switch c.proto {
	case ProtoHTTP11:
		if c.h1Res != nil {
			c.h1Res.Headers.Set(key, val)
		}
	case ProtoHTTP2:
		if c.h2Res != nil {
			if c.h2Res.Headers == nil {
				c.h2Res.Headers = make(http.Header, 4)
			}

			canonicalKey := http.CanonicalHeaderKey(key)
			if existing, ok := c.h2Res.Headers[canonicalKey]; ok && cap(existing) > 0 {
				c.h2Res.Headers[canonicalKey] = append(existing[:0], val)
			} else {
				c.h2Res.Headers[canonicalKey] = []string{val}
			}
		}

	case ProtoHTTP3:
		if c.h3Res != nil {
			c.h3Res.Headers.Set(key, val)
		}
	}
}

// AddHeader appends val to the response header associated with key.
func (c *Context) AddHeader(key, val string) {
	switch c.proto {
	case ProtoHTTP11:
		if c.h1Res != nil {
			c.h1Res.Headers.Add(key, val)
		}
	case ProtoHTTP2:
		if c.h2Res != nil {
			c.h2Res.Headers.Add(key, val)
		}
	case ProtoHTTP3:
		if c.h3Res != nil {
			c.h3Res.Headers.Add(key, val)
		}
	}
}

// DelHeader removes the response header associated with key.
func (c *Context) DelHeader(key string) {
	switch c.proto {
	case ProtoHTTP11:
		if c.h1Res != nil {
			c.h1Res.Headers.Del(key)
		}
	case ProtoHTTP2:
		if c.h2Res != nil {
			c.h2Res.Headers.Del(key)
		}
	case ProtoHTTP3:
		if c.h3Res != nil {
			c.h3Res.Headers.Del(key)
		}
	}
}

// ResponseHeader returns the current outgoing response header value associated with key.
func (c *Context) ResponseHeader(key string) string {
	switch c.proto {
	case ProtoHTTP11:
		if c.h1Res != nil {
			return c.h1Res.Headers.Get(key)
		}
	case ProtoHTTP2:
		if c.h2Res != nil {
			return c.h2Res.Headers.Get(key)
		}
	case ProtoHTTP3:
		if c.h3Res != nil {
			return c.h3Res.Headers.Get(key)
		}
	}

	return ""
}

// ResponseHeaders returns an iterator over all outgoing response headers without heap allocations.
func (c *Context) ResponseHeaders() iter.Seq2[string, string] {
	return func(yield func(string, string) bool) {
		switch c.proto {
		case ProtoHTTP11:
			if c.h1Res != nil {
				for k, v := range c.h1Res.Headers.All() {
					if !yield(k, v) {
						return
					}
				}
			}

		case ProtoHTTP2:
			if c.h2Res != nil {
				for k, vals := range c.h2Res.Headers {
					for _, v := range vals {
						if !yield(k, v) {
							return
						}
					}
				}
			}

		case ProtoHTTP3:
			if c.h3Res != nil {
				for k, v := range c.h3Res.Headers.All() {
					if !yield(k, v) {
						return
					}
				}
			}
		}
	}
}

// VisitResponseHeaders iterates over all response headers calling fn.
func (c *Context) VisitResponseHeaders(fn func(key, val string)) {
	for k, v := range c.ResponseHeaders() {
		fn(k, v)
	}
}

// SetBody replaces the response payload with body.
func (c *Context) SetBody(body []byte) {
	switch c.proto {
	case ProtoHTTP11:
		if c.h1Res != nil {
			c.h1Res.Body = body
		}
	case ProtoHTTP2:
		if c.h2Res != nil {
			c.h2Res.Body = body
		}
	case ProtoHTTP3:
		if c.h3Res != nil {
			c.h3Res.Body = body
		}
	}
}

// SetBodyString replaces the response payload with string s.
func (c *Context) SetBodyString(s string) {
	switch c.proto {
	case ProtoHTTP11:
		if c.h1Res != nil {
			c.h1Res.Body = append(c.h1Res.Body[:0], s...)
		}
	case ProtoHTTP2:
		if c.h2Res != nil {
			c.h2Res.Body = append(c.h2Res.Body[:0], s...)
		}
	case ProtoHTTP3:
		if c.h3Res != nil {
			c.h3Res.Body = append(c.h3Res.Body[:0], s...)
		}
	}
}

// Write appends p to the response body, implementing io.Writer.
func (c *Context) Write(p []byte) (int, error) {
	switch c.proto {
	case ProtoHTTP11:
		if c.h1Res != nil {
			c.h1Res.Body = append(c.h1Res.Body, p...)
			return len(p), nil
		}
	case ProtoHTTP2:
		if c.h2Res != nil {
			c.h2Res.Body = append(c.h2Res.Body, p...)
			return len(p), nil
		}
	case ProtoHTTP3:
		if c.h3Res != nil {
			c.h3Res.Body = append(c.h3Res.Body, p...)
			return len(p), nil
		}
	}

	return 0, io.ErrClosedPipe
}

// WriteString appends s to the response body.
func (c *Context) WriteString(s string) (int, error) {
	return c.Write(bytesconv.S2B(s))
}

// ResponseBody returns the current response payload bytes.
func (c *Context) ResponseBody() []byte {
	switch c.proto {
	case ProtoHTTP11:
		if c.h1Res != nil {
			return c.h1Res.Body
		}
	case ProtoHTTP2:
		if c.h2Res != nil {
			return c.h2Res.Body
		}
	case ProtoHTTP3:
		if c.h3Res != nil {
			return c.h3Res.Body
		}
	}

	return nil
}

// String sets the status code, Content-Type to text/plain, and response body to s.
func (c *Context) String(code int, s string) {
	c.SetStatusCode(code)
	c.SetHeader(headkit.ContentType, "text/plain; charset=utf-8")
	c.SetBodyString(s)
}

// JSON sets the status code, Content-Type to application/json, and serializes v as JSON body.
func (c *Context) JSON(code int, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}

	c.SetStatusCode(code)
	c.SetHeader(headkit.ContentType, "application/json; charset=utf-8")
	c.SetBody(data)

	return nil
}

// Bytes sets the status code, Content-Type, and response body bytes.
func (c *Context) Bytes(code int, contentType string, b []byte) {
	c.SetStatusCode(code)
	c.SetHeader(headkit.ContentType, contentType)
	c.SetBody(b)
}

// HTML sets the status code, Content-Type to text/html, and response body to html.
func (c *Context) HTML(code int, html string) {
	c.SetStatusCode(code)
	c.SetHeader(headkit.ContentType, "text/html; charset=utf-8")
	c.SetBodyString(html)
}

// SetUserValue stores a key-value pair associated with this request lifecycle.
func (c *Context) SetUserValue(key, val any) {
	if c.userValues == nil {
		c.userValues = make(map[any]any, 4)
	}

	c.userValues[key] = val
}

// UserValue retrieves the value associated with key from the request store.
func (c *Context) UserValue(key any) any {
	if c.userValues == nil {
		return nil
	}

	return c.userValues[key]
}

// DisableAltSvc disables automatic emission of Alt-Svc headers for this response.
func (c *Context) DisableAltSvc() {
	c.disableAltSvc = true
}

// IsAltSvcDisabled reports whether Alt-Svc emission has been explicitly disabled for this request.
func (c *Context) IsAltSvcDisabled() bool {
	return c.disableAltSvc
}

// H1 returns the underlying HTTP/1.1 request and response pointers if this is an HTTP/1.1 exchange.
func (c *Context) H1() (*h1.Request, *h1.Response, bool) {
	if c.proto == ProtoHTTP11 {
		return c.h1Req, c.h1Res, true
	}

	return nil, nil, false
}

// H2 returns the underlying HTTP/2 request and response pointers if this is an HTTP/2 exchange.
func (c *Context) H2() (*h2.ServerRequest, *h2.ServerResponse, bool) {
	if c.proto == ProtoHTTP2 {
		return c.h2Req, c.h2Res, true
	}

	return nil, nil, false
}

// H3 returns the underlying HTTP/3 request and response pointers if this is an HTTP/3 exchange.
func (c *Context) H3() (*h3.ServerRequest, *h3.ServerResponse, bool) {
	if c.proto == ProtoHTTP3 {
		return c.h3Req, c.h3Res, true
	}

	return nil, nil, false
}

// Reset clears the Context fields for reuse by the object pool.
func (c *Context) Reset() {
	c.server = nil
	c.proto = ""
	c.h1Req = nil
	c.h1Res = nil
	c.h2Req = nil
	c.h2Res = nil
	c.h3Req = nil
	c.h3Res = nil
	c.path = ""
	c.query = ""
	c.parsedURI = false
	c.ctx = nil
	c.disableAltSvc = false
	clear(c.userValues)
}

func (c *Context) initH1(s *Server, req *h1.Request, res *h1.Response) {
	c.server = s
	c.proto = ProtoHTTP11
	c.h1Req = req
	c.h1Res = res
	c.path = req.Path
	c.query = req.Query
	c.parsedURI = true
	c.disableAltSvc = false
}

func (c *Context) initH2(s *Server, req *h2.ServerRequest, res *h2.ServerResponse) {
	c.server = s
	c.proto = ProtoHTTP2
	c.h2Req = req
	c.h2Res = res
	c.parsedURI = false
	c.disableAltSvc = false
}

func (c *Context) initH3(s *Server, req *h3.ServerRequest, res *h3.ServerResponse) {
	c.server = s
	c.proto = ProtoHTTP3
	c.h3Req = req
	c.h3Res = res
	c.parsedURI = false
	c.disableAltSvc = false
}
