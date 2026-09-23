// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h1

import (
	"io"
	"strconv"

	"github.com/lemon4ksan/foundation/net/headkit"
	"github.com/lemon4ksan/foundation/net/http/header"
	"github.com/lemon4ksan/foundation/net/http/status"
	"github.com/lemon4ksan/foundation/net/http/zerocopy"
	"github.com/lemon4ksan/foundation/silicon/bytesconv"
)

// Response carries HTTP/1.1 response state to be serialized directly over the wire (RFC 9112 §2.1, RFC 9110 §15).
//
// Concurrency:
//   - Not safe for concurrent use across multiple goroutines.
//
// Memory Lifecycle:
//   - Acquired from Per-P storage per request and recycled after WriteTo completes.
type Response struct {
	// StatusCode is the HTTP response status code (RFC 9110 §15). Defaults to 200 OK.
	StatusCode int
	// Headers holds response header fields (RFC 9110 §6.3).
	Headers headkit.Headers
	// Cookies holds Set-Cookie attributes (RFC 6265 §4.1).
	Cookies []*zerocopy.Cookie
	// Body holds the static response payload bytes.
	Body []byte
	// StreamWriter provides chunked streaming response generation (RFC 9112 §7.1).
	StreamWriter func(w io.Writer) error
}

// Reset clears the response for recycling into Per-P storage.
func (res *Response) Reset() {
	res.StatusCode = status.OK
	res.Headers.Reset()
	res.Cookies = res.Cookies[:0]

	if cap(res.Body) > 64*1024 {
		res.Body = make([]byte, 0, 1024)
	} else {
		res.Body = res.Body[:0]
	}

	res.StreamWriter = nil
}

// WriteTo writes the full HTTP/1.1 response (status line, headers, cookies, body or stream) to bw (RFC 9112 §2.1, RFC 9110 §15).
// If flush is false, bytes remain buffered in bw to coalesce pipelined responses into a single write syscall.
func (res *Response) WriteTo(bw *bytesconv.ByteBuffer, keepAlive, flush bool) error {
	code := res.StatusCode
	if code == 0 {
		code = status.OK
	}

	// 1. Fast Status Line (from pre-compiled static table)
	if code >= 100 && code < len(statusLines) && statusLines[code] != nil {
		_, _ = bw.Write(statusLines[code])
	} else {
		statusText := status.Message(code)
		if statusText == "" {
			statusText = "Unknown"
		}

		var stBuf [16]byte

		_, _ = bw.WriteString("HTTP/1.1 ")
		_, _ = bw.Write(strconv.AppendInt(stBuf[:0], int64(code), 10))
		_ = bw.WriteByte(' ')
		_, _ = bw.WriteString(statusText)
		_, _ = bw.Write(hdrCRLF)
	}

	// 2. Atomic Cached Date Header
	if res.Headers.Get(header.Date) == "" {
		dateBytes := cachedDateHeader.Load()
		if dateBytes != nil {
			_, _ = bw.Write(*dateBytes)
		}
	}

	// 3. Connection Header
	if keepAlive {
		if res.Headers.Get(header.Connection) == "" {
			_, _ = bw.Write(hdrConnectionKeepAlive)
		}
	} else {
		_, _ = bw.Write(hdrConnectionClose)
	}

	// 4. Transfer-Encoding or Content-Length
	if res.StreamWriter != nil {
		if res.Headers.Get(header.TransferEncoding) == "" {
			_, _ = bw.Write(hdrTransferChunked)
		}
	} else if code != status.NoContent && code != status.NotModified {
		if res.Headers.Get(header.ContentLength) == "" && res.Headers.Get(header.TransferEncoding) == "" {
			var clBuf [24]byte

			_, _ = bw.Write(hdrContentLengthPrefix)
			_, _ = bw.Write(strconv.AppendInt(clBuf[:0], int64(len(res.Body)), 10))
			_, _ = bw.Write(hdrCRLF)
		}
	}

	// 5. User Headers
	res.Headers.WriteTo(bw)

	// 6. Cookies
	for _, c := range res.Cookies {
		if c != nil {
			_, _ = bw.Write(hdrSetCookiePrefix)
			bw.B = c.AppendBytes(bw.B)
			_, _ = bw.Write(hdrCRLF)
		}
	}

	// End of Headers
	_, _ = bw.Write(hdrCRLF)

	// 7. Streaming Body or Static Body
	if res.StreamWriter != nil {
		return res.writeStreamBody(bw)
	}

	if len(res.Body) > 0 && code != status.NoContent && code != status.NotModified {
		_, _ = bw.Write(res.Body)
	}

	if flush {
		return nil
	}

	return nil
}

//go:noinline
func (res *Response) writeStreamBody(bw *bytesconv.ByteBuffer) error {
	cw := NewChunkedWriter(bw)
	err := res.StreamWriter(cw)
	closeErr := cw.Close()

	if err != nil {
		return err
	}

	if closeErr != nil {
		return closeErr
	}

	return nil
}
