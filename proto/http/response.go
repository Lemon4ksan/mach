// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package http

import (
	"io"
	"net"

	"github.com/lemon4ksan/foundation/net/http/zerocopy"
	"github.com/lemon4ksan/foundation/silicon/bytesconv"
)

// Response represents an HTTP response message adhering to RFC 9110 Section 3 and RFC 9112 Section 2.
//
// A Response encapsulates the status-line (protocol version, status code, reason phrase),
// response headers, entity body or body stream, and connection network addresses.
//
// Concurrency:
// A Response instance MUST NOT be used concurrently from multiple goroutines.
//
// Lifecycle & Pooling:
// Instances should be acquired via AcquireResponse and recycled via ReleaseResponse to avoid
// heap allocations. Copying Response by value is forbidden; use CopyTo instead.
type Response struct {
	noCopy     zerocopy.NoCopy
	bodyStream io.Reader
	raddr      net.Addr
	laddr      net.Addr
	w          responseBodyWriter
	body       *bytesconv.ByteBuffer
	bodyRaw    []byte

	// Header is the response header.
	//
	// Copying Header by value is forbidden. Use pointer to Header instead.
	Header ResponseHeader

	ImmediateHeaderFlush bool
	StreamBody           bool

	// SkipBody skips reading body if set to true.
	// Use it for reading HEAD responses.
	SkipBody              bool
	KeepBodyBuffer        bool
	SecureErrorLogMessage bool

	// OnInterimResponse is an optional callback that is fired when the client receives
	// an interim 1xx response (such as 103 Early Hints or 100 Continue) before the final response.
	// The provided ResponseHeader is valid only during the callback.
	OnInterimResponse func(statusCode int, header *ResponseHeader)
}

// StatusCode returns the 3-digit HTTP response status code (RFC 9110 Section 15, RFC 9112 Section 3.1.2).
//
// Thread-safe: No.
func (resp *Response) StatusCode() int {
	return resp.Header.StatusCode()
}

// SetStatusCode sets the 3-digit HTTP response status code (RFC 9110 Section 15, RFC 9112 Section 3.1.2).
//
// Thread-safe: No.
func (resp *Response) SetStatusCode(statusCode int) {
	resp.Header.SetStatusCode(statusCode)
}

// ConnectionClose reports whether the "Connection: close" token is set in the response headers (RFC 9110 Section 9.6, RFC 9112 Section 9.3).
//
// Thread-safe: No.
func (resp *Response) ConnectionClose() bool {
	return resp.Header.ConnectionClose()
}

// SetConnectionClose sets the "Connection: close" header token, indicating the connection will be closed upon response delivery (RFC 9110 Section 9.6).
//
// Thread-safe: No.
func (resp *Response) SetConnectionClose() {
	resp.Header.SetConnectionClose()
}

// ParseNetConn extracts and caches the local and remote network addresses from conn.
//
// Thread-safe: No.
func (resp *Response) ParseNetConn(conn net.Conn) {
	resp.raddr = conn.RemoteAddr()
	resp.laddr = conn.LocalAddr()
}

// RemoteAddr returns the remote network address of the client connection.
//
// Thread-safe: No.
func (resp *Response) RemoteAddr() net.Addr {
	return resp.raddr
}

// LocalAddr returns the local network address of the server listener.
//
// Thread-safe: No.
func (resp *Response) LocalAddr() net.Addr {
	return resp.laddr
}

// Reset clears all response fields and recycles the internal body buffer to its pool.
//
// Thread-safe: No.
func (resp *Response) Reset() {
	if bodyPoolSizeLimit := int(responseBodyPoolSizeLimit.Load()); bodyPoolSizeLimit >= 0 && resp.body != nil {
		resp.ReleaseBody(bodyPoolSizeLimit)
	}

	resp.resetSkipHeader()
	resp.Header.Reset()
	resp.SkipBody = false
	resp.raddr = nil
	resp.laddr = nil
	resp.ImmediateHeaderFlush = false
	resp.StreamBody = false
}

func (resp *Response) resetSkipHeader() {
	resp.ResetBody()
}

// String returns the diagnostic wire representation of the response.
// This allocates memory for string formatting; use Write in performance-critical paths.
func (resp *Response) String() string {
	return getHTTPString(resp)
}
