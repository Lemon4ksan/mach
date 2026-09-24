// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package http

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"sync"

	"github.com/lemon4ksan/foundation/silicon/bytesconv"

	"github.com/lemon4ksan/mach/proto/http/zerocopy"
)

type bodyStreamHeader interface {
	ContentLength() int
	ReadTrailer(r *bufio.Reader) error
}

// RequestStream decodes chunked transfer coded or fixed-length streaming HTTP request/response bodies (RFC 9112 Section 7.1).
//
// Thread-safe: No. Must be read from a single goroutine.
type RequestStream struct {
	header          bodyStreamHeader
	prefetchedBytes *bytes.Reader
	reader          *bufio.Reader
	totalBytesRead  int
	chunkLeft       int
}

// Read reads up to len(p) bytes from the underlying HTTP body stream, parsing chunk headers and chunk trailers as needed (RFC 9112 Section 7.1).
func (rs *RequestStream) Read(p []byte) (int, error) {
	var (
		n   int
		err error
	)
	if rs.header.ContentLength() == -1 {
		if rs.chunkLeft == 0 {
			chunkSize, err := parseChunkSize(rs.reader)
			if err != nil {
				return 0, err
			}

			if chunkSize == 0 {
				err = rs.header.ReadTrailer(rs.reader)
				if err != nil && !errors.Is(err, io.EOF) {
					return 0, err
				}

				return 0, io.EOF
			}

			rs.chunkLeft = chunkSize
		}

		bytesToRead := min(rs.chunkLeft, len(p))
		n, err = rs.reader.Read(p[:bytesToRead])
		rs.totalBytesRead += n
		rs.chunkLeft -= n

		if errors.Is(err, io.EOF) {
			err = io.ErrUnexpectedEOF
		}

		if err == nil && rs.chunkLeft == 0 {
			err = readCrLf(rs.reader)
		}

		return n, err
	}

	if rs.totalBytesRead == rs.header.ContentLength() {
		return 0, io.EOF
	}

	prefetchedSize := int(rs.prefetchedBytes.Size())
	if prefetchedSize > rs.totalBytesRead {
		left := prefetchedSize - rs.totalBytesRead
		if len(p) > left {
			p = p[:left]
		}

		n, err = rs.prefetchedBytes.Read(p)

		rs.totalBytesRead += n
		if n == rs.header.ContentLength() {
			return n, io.EOF
		}

		return n, err
	}

	left := rs.header.ContentLength() - rs.totalBytesRead
	if left > 0 && len(p) > left {
		p = p[:left]
	}

	n, err = rs.reader.Read(p)

	rs.totalBytesRead += n
	if err != nil {
		return n, err
	}

	if rs.totalBytesRead == rs.header.ContentLength() {
		err = io.EOF
	}

	return n, err
}

// AcquireRequestStream acquires a pooled RequestStream instance initialized with prefetched bytes, reader, and header parser.
//
// Thread-safe: Yes (backed by sync.Pool).
func AcquireRequestStream(b *bytesconv.ByteBuffer, r *bufio.Reader, h bodyStreamHeader) *RequestStream {
	rs := RequestStreamPool.Get().(*RequestStream) //nolint:forcetypeassert
	rs.prefetchedBytes = bytes.NewReader(b.B)
	rs.reader = r
	rs.header = h

	return rs
}

// ReleaseRequestStream releases a RequestStream instance back to RequestStreamPool after resetting its fields.
//
// Thread-safe: Yes (backed by sync.Pool).
func ReleaseRequestStream(rs *RequestStream) {
	rs.prefetchedBytes = nil
	rs.totalBytesRead = 0
	rs.chunkLeft = 0
	rs.reader = nil
	rs.header = nil
	RequestStreamPool.Put(rs)
}

// RequestStreamPool pools RequestStream instances to reduce garbage collection overhead during streaming I/O.
var RequestStreamPool = sync.Pool{
	New: func() any {
		return &RequestStream{}
	},
}

// SetBodyStream configures bodyStream as the request entity body reader with an expected length (RFC 9112 Section 7.1).
// If bodySize >= 0, Content-Length is set to bodySize. If bodySize < 0, Chunked Transfer Coding is used.
// If bodyStream implements io.Closer, it is automatically closed upon stream exhaustion or request release.
//
// Thread-safe: No.
func (req *Request) SetBodyStream(bodyStream io.Reader, bodySize int) {
	req.ResetBody()
	req.bodyStream = bodyStream
	req.Header.SetContentLength(bodySize)
}

// IsBodyStream reports whether the request body is supplied via a streaming io.Reader.
//
// Thread-safe: No.
func (req *Request) IsBodyStream() bool {
	return req.bodyStream != nil
}

// SetBodyStreamWriter populates the request body asynchronously via sw using a pipe connection.
//
// Thread-safe: No.
func (req *Request) SetBodyStreamWriter(sw StreamWriter) {
	sr := NewStreamReader(sw)
	req.SetBodyStream(sr, -1)
}

// BodyStream returns the configured request body stream reader, or nil if none is set.
// The caller must ensure CloseBodyStream or ReleaseRequest is invoked when finished.
//
// Thread-safe: No.
func (req *Request) BodyStream() io.Reader {
	return req.bodyStream
}

// RequestBodyStream returns the configured request body reader.
//
// Thread-safe: No.
func (req *Request) RequestBodyStream() io.Reader {
	return req.bodyStream
}

// CloseBodyStream closes the active request body stream if it implements io.Closer and returns any error encountered.
//
// Thread-safe: No.
func (req *Request) CloseBodyStream() error {
	return req.closeBodyStream()
}

func (req *Request) closeBodyStream() error {
	if req.bodyStream == nil {
		return nil
	}

	var err error
	if bsc, ok := req.bodyStream.(io.Closer); ok {
		err = bsc.Close()
	}

	if rs, ok := req.bodyStream.(*RequestStream); ok {
		ReleaseRequestStream(rs)
	}

	req.bodyStream = nil

	return err
}

// BodyWriter returns an io.Writer adapter for streaming data into the request body buffer.
//
// Thread-safe: No.
func (req *Request) BodyWriter() io.Writer {
	req.w.r = req
	return &req.w
}

// ContinueReadBodyStream reads a chunked or streaming request body after a 100 Continue handshake has taken place (RFC 9110 Section 10.1.1).
//
// Thread-safe: No.
func (req *Request) ContinueReadBodyStream(r *bufio.Reader, maxBodySize int, preParseMultipartForm ...bool) error {
	var err error

	contentLength := req.Header.ContentLength()
	if contentLength > 0 {
		if len(preParseMultipartForm) == 0 || preParseMultipartForm[0] {
			req.multipartFormBoundary = bytesconv.B2S(req.Header.MultipartFormBoundary())
			if req.multipartFormBoundary != "" && len(req.Header.peek(zerocopy.StrContentEncoding)) == 0 {
				req.multipartForm, err = readMultipartForm(
					r,
					req.multipartFormBoundary,
					contentLength,
					defaultMaxInMemoryFileSize,
				)
				if err != nil {
					req.Reset()
				}

				return err
			}
		}
	}

	if contentLength == -2 {
		if !req.Header.ignoreBody() {
			req.Header.SetContentLength(0)
		}

		return nil
	}

	bodyBuf := req.BodyBuffer()
	bodyBuf.Reset()

	bodyBuf.B, err = readBodyWithStreaming(r, contentLength, maxBodySize, bodyBuf.B)
	if err != nil {
		if errors.Is(err, ErrBodyTooLarge) {
			req.Header.SetContentLength(contentLength)
			req.body = bodyBuf
			req.bodyStream = AcquireRequestStream(bodyBuf, r, &req.Header)

			return nil
		}

		if errors.Is(err, errChunkedStream) {
			req.body = bodyBuf
			req.bodyStream = AcquireRequestStream(bodyBuf, r, &req.Header)
			return nil
		}

		req.Reset()

		return err
	}

	req.body = bodyBuf
	req.bodyStream = AcquireRequestStream(bodyBuf, r, &req.Header)
	req.Header.SetContentLength(contentLength)

	return nil
}
