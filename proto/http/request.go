// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package http

import (
	"io"
	"mime/multipart"
	"time"

	"github.com/lemon4ksan/mach/proto/http/zerocopy"
	"github.com/lemon4ksan/foundation/silicon/bytesconv"
)

// Request represents an HTTP request message adhering to RFC 9110 Section 3 and RFC 9112 Section 2.
//
// A Request encapsulates the request-line (method, request-target, protocol version),
// message headers, and optional entity/chunked message body.
//
// Concurrency:
// A Request instance MUST NOT be used concurrently from multiple goroutines.
//
// Lifecycle & Pooling:
// Instances should be acquired via AcquireRequest and recycled via ReleaseRequest to avoid
// heap allocations. Copying Request by value is forbidden; use CopyTo instead.
type Request struct {
	noCopy                zerocopy.NoCopy
	bodyStream            io.Reader
	w                     requestBodyWriter
	body                  *bytesconv.ByteBuffer
	multipartForm         *multipart.Form
	multipartFormBoundary string
	postArgs              zerocopy.Args
	bodyRaw               []byte

	uri zerocopy.URI

	// Header is the request header.
	//
	// Copying Header by value is forbidden. Use pointer to Header instead.
	Header RequestHeader

	Timeout               time.Duration
	SecureErrorLogMessage bool
	parsedURI             bool
	ParsedPostArgs        bool
	uriParseErr           error
	KeepBodyBuffer        bool
	isTLS                 bool
	UseHostHeader         bool

	// DisableRedirectPathNormalizing disables redirect path normalization when used with DoRedirects.
	//
	// By default redirect path values are normalized, i.e.
	// extra slashes are removed, special characters are encoded.
	DisableRedirectPathNormalizing bool
}

// SetHost sets the host component of the request target and updates the Host header (RFC 9110 Section 7.2).
//
// Thread-safe: No. Must be invoked from the owning goroutine only.
func (req *Request) SetHost(host string) {
	req.URI().SetHost(host)
}

// SetHostBytes sets the host component of the request target from a byte slice (RFC 9110 Section 7.2).
//
// Thread-safe: No.
func (req *Request) SetHostBytes(host []byte) {
	req.URI().SetHostBytes(host)
}

// Host returns the host component of the request target (RFC 9110 Section 7.2).
// The returned byte slice is borrowed from the underlying URI or header buffer and remains
// valid until the request is modified or released.
//
// Thread-safe: No.
func (req *Request) Host() []byte {
	return req.URI().Host()
}

// SetRequestURI sets the raw request-target string (RFC 9112 Section 3.2).
// This invalidates any previously cached parsed URI state.
//
// Thread-safe: No.
func (req *Request) SetRequestURI(requestURI string) {
	req.Header.SetRequestURI(requestURI)
	req.parsedURI = false
	req.uriParseErr = nil
}

// SetRequestURIBytes sets the raw request-target byte slice (RFC 9112 Section 3.2).
// This invalidates any previously cached parsed URI state.
//
// Thread-safe: No.
func (req *Request) SetRequestURIBytes(requestURI []byte) {
	req.Header.SetRequestURIBytes(requestURI)
	req.parsedURI = false
	req.uriParseErr = nil
}

// RequestURI returns the raw request-target bytes (RFC 9112 Section 3.2).
// If the URI was previously modified through the URI() accessor, the formatted request-target
// is synchronized back to the request headers.
//
// Thread-safe: No.
func (req *Request) RequestURI() []byte {
	if req.parsedURI {
		requestURI := req.uri.RequestURI()
		req.SetRequestURIBytes(requestURI)
	}

	return req.Header.RequestURI()
}

// URI returns a pointer to the parsed zerocopy.URI representation of the request-target (RFC 3986, RFC 9112 Section 3.2).
// The returned pointer is valid until the request is modified or released.
//
// Thread-safe: No.
func (req *Request) URI() *zerocopy.URI {
	_ = req.ParseURI()
	return &req.uri
}

// SetURI sets the request URI by copying from the provided zerocopy.URI instance (RFC 3986).
// If newURI is nil, the request URI is cleared.
//
// Thread-safe: No.
func (req *Request) SetURI(newURI *zerocopy.URI) {
	if newURI != nil {
		newURI.CopyTo(&req.uri)
		req.parsedURI = true
		req.uriParseErr = nil

		return
	}

	req.uri.Reset()
	req.parsedURI = false
	req.uriParseErr = nil
}

// ParseURI parses the request-line authority and request-target into the internal URI cache (RFC 3986).
// Subsequent calls return the cached parse error if already parsed.
//
// Thread-safe: No.
func (req *Request) ParseURI() error {
	if req.parsedURI {
		return req.uriParseErr
	}

	req.parsedURI = true
	req.uriParseErr = req.uri.ParseInternal(req.Header.Host(), req.Header.RequestURI(), req.isTLS)

	return req.uriParseErr
}

// ConnectionClose reports whether the "Connection: close" token is present in the request headers
// (RFC 9110 Section 9.6, RFC 9112 Section 9.3).
//
// Thread-safe: No.
func (req *Request) ConnectionClose() bool {
	return req.Header.ConnectionClose()
}

// SetConnectionClose sets the "Connection: close" header token, indicating that the connection
// should be closed after completing this request (RFC 9110 Section 9.6, RFC 9112 Section 9.3).
//
// Thread-safe: No.
func (req *Request) SetConnectionClose() {
	req.Header.SetConnectionClose()
}

// GetTimeOut returns the maximum duration allowed for the request lifecycle.
//
// Thread-safe: No.
func (req *Request) GetTimeOut() time.Duration {
	return req.Timeout
}

// SetTimeout sets the maximum duration allowed for the request lifecycle.
//
// Thread-safe: No.
func (req *Request) SetTimeout(t time.Duration) {
	req.Timeout = t
}

// Reset clears all request contents and returns internal buffers to their respective pools.
// Must be called prior to returning the request to a sync.Pool.
//
// Thread-safe: No.
func (req *Request) Reset() {
	if bodyPoolSizeLimit := int(requestBodyPoolSizeLimit.Load()); bodyPoolSizeLimit >= 0 && req.body != nil {
		req.ReleaseBody(bodyPoolSizeLimit)
	}

	req.Header.Reset()
	req.resetSkipHeader()
	req.Timeout = 0
	req.UseHostHeader = false
	req.DisableRedirectPathNormalizing = false
}

func (req *Request) resetSkipHeader() {
	req.ResetBody()
	req.uri.Reset()
	req.parsedURI = false
	req.uriParseErr = nil
	req.postArgs.Reset()
	req.ParsedPostArgs = false
	req.isTLS = false
}

// String returns the diagnostic wire representation of the request.
// This allocates and formats the entire request message; use Write in performance-critical paths.
func (req *Request) String() string {
	return getHTTPString(req)
}
