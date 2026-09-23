// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package http

import (
	"io"
	"os"

	"github.com/lemon4ksan/foundation/borrow"
)

// SendFile opens the local file at path and assigns it as the streaming response body (RFC 9110 Section 8.8.2).
// It populates the Last-Modified header with the file's modification time.
// The file descriptor is automatically closed once the response transmission finishes or the response is reset.
//
// Thread-safe: No.
func (resp *Response) SendFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}

	fileInfo, err := f.Stat()
	if err != nil {
		_ = f.Close()

		return err
	}

	size64 := fileInfo.Size()

	size := int(size64)
	if int64(size) != size64 {
		size = -1
	}

	resp.Header.SetLastModified(fileInfo.ModTime())
	resp.SetBodyStream(f, size)

	return nil
}

// SetBodyStream configures bodyStream as the response body reader with an expected length (RFC 9112 Section 7.1).
// If bodySize >= 0, Content-Length is set to bodySize. If bodySize < 0, Chunked Transfer Coding is used.
// If bodyStream implements io.Closer, it is automatically closed upon stream completion.
//
// Thread-safe: No.
func (resp *Response) SetBodyStream(bodyStream io.Reader, bodySize int) {
	resp.ResetBody()
	resp.bodyStream = bodyStream
	resp.Header.SetContentLength(bodySize)
}

// IsBodyStream reports whether the response body is supplied via an io.Reader stream.
//
// Thread-safe: No.
func (resp *Response) IsBodyStream() bool {
	return resp.bodyStream != nil
}

// SetBodyStreamWriter configures sw to stream response data asynchronously into a piped connection.
//
// Thread-safe: No.
func (resp *Response) SetBodyStreamWriter(sw StreamWriter) {
	sr := NewStreamReader(sw)
	resp.SetBodyStream(sr, -1)
}

// BodyWriter returns an io.Writer adapter for appending data to the response body buffer.
//
// Thread-safe: No.
func (resp *Response) BodyWriter() io.Writer {
	resp.w.r = resp
	return &resp.w
}

// BodyStream returns the active response body stream reader, or nil if none is configured.
//
// Thread-safe: No.
func (resp *Response) BodyStream() io.Reader {
	return resp.bodyStream
}

// CloseBodyStream closes the active response body stream if it implements io.Closer.
//
// Thread-safe: No.
func (resp *Response) CloseBodyStream() error {
	return resp.closeBodyStream(nil)
}

func (resp *Response) closeBodyStream(wErr error) error {
	if resp.bodyStream == nil {
		return nil
	}

	err := closeBodyStreamReader(resp.bodyStream, wErr)
	resp.bodyStream = nil

	return err
}

// ReadStreamScoped reads from the response body stream chunk by chunk, passing each borrowed slice to fn for zero-allocation stream processing.
func (resp *Response) ReadStreamScoped(s *borrow.Scope, fn func(chunk borrow.Bytes) error) error {
	r := resp.BodyStream()
	if r == nil {
		b := resp.Body()
		if len(b) > 0 {
			return fn(borrow.NewBytes(b, nil))
		}

		return nil
	}

	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			if callErr := fn(borrow.NewBytes(buf[:n], nil)); callErr != nil {
				return callErr
			}
		}

		if err != nil {
			if err == io.EOF {
				break
			}

			return err
		}
	}

	return nil
}
