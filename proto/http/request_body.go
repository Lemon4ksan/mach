// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package http

import (
	"errors"

	"github.com/lemon4ksan/foundation/borrow"
	"github.com/lemon4ksan/foundation/codec/compress"
	"github.com/lemon4ksan/foundation/silicon/bytesconv"
)

type requestBodyWriter struct{ r *Request }

func (w *requestBodyWriter) Write(p []byte) (int, error) {
	w.r.AppendBody(p)
	return len(p), nil
}

func (w *requestBodyWriter) WriteString(s string) (int, error) {
	w.r.AppendBodyString(s)
	return len(s), nil
}

// SwapRequestBody swaps request body between a and b per RFC 9110 Section 6.4.
//
// Concurrency: Not thread-safe.
func SwapRequestBody(a, b *Request) {
	a.body, b.body = b.body, a.body
	a.bodyRaw, b.bodyRaw = b.bodyRaw, a.bodyRaw

	a.bodyStream, b.bodyStream = b.bodyStream, a.bodyStream
	if rs, ok := a.bodyStream.(*RequestStream); ok {
		rs.header = &a.Header
	}

	if rs, ok := b.bodyStream.(*RequestStream); ok {
		rs.header = &b.Header
	}
}

// Body returns the request entity body as a byte slice (RFC 9110 Section 6.4).
// If the body is backed by a stream, Body reads the entire stream into memory.
// The returned slice is borrowed and remains valid until the request is reset or released.
//
// Thread-safe: No.
func (req *Request) Body() []byte {
	if req.bodyRaw != nil {
		return req.bodyRaw
	} else if req.onlyMultipartForm() {
		body, err := marshalMultipartForm(req.multipartForm, req.multipartFormBoundary)
		if err != nil {
			return []byte(err.Error())
		}

		return body
	}

	return req.bodyBytes()
}

func (req *Request) bodyBytes() []byte {
	if req.bodyRaw != nil {
		return req.bodyRaw
	}

	if req.bodyStream != nil {
		bodyBuf := req.BodyBuffer()
		bodyBuf.Reset()
		_, err := copyBodyStream(bodyBuf, req.bodyStream)
		_ = req.closeBodyStream()

		if err != nil {
			bodyBuf.SetString(err.Error())
		}
	}

	if req.body == nil {
		return nil
	}

	return req.body.B
}

// BodyBuffer returns the underlying ByteBuffer storing the in-memory request body.
// If no buffer is currently allocated, one is acquired from the per-P request body pool.
//
// Thread-safe: No.
func (req *Request) BodyBuffer() *bytesconv.ByteBuffer {
	if req.body == nil {
		req.body = requestBodyPool.Get()
	}

	req.bodyRaw = nil

	return req.body
}

// SetBody sets the request body to a copy of body (RFC 9110 Section 6.4).
// It resets any existing body stream or multipart files.
//
// Thread-safe: No.
func (req *Request) SetBody(body []byte) {
	req.RemoveMultipartFormFiles()
	_ = req.closeBodyStream()
	req.BodyBuffer().Set(body)
}

// SetBodyString sets the request body to the provided string content.
//
// Thread-safe: No.
func (req *Request) SetBodyString(body string) {
	req.RemoveMultipartFormFiles()
	_ = req.closeBodyStream()
	req.BodyBuffer().SetString(body)
}

// SetBodyRaw sets the request body to point directly to body without copying.
// The caller must guarantee that the provided slice is not mutated while the Request is in use.
//
// Thread-safe: No.
func (req *Request) SetBodyRaw(body []byte) {
	req.ResetBody()
	req.bodyRaw = body
}

// AppendBody appends the given byte slice to the request entity body.
//
// Thread-safe: No.
func (req *Request) AppendBody(p []byte) {
	req.RemoveMultipartFormFiles()
	_ = req.closeBodyStream()
	_, _ = req.BodyBuffer().Write(p)
}

// AppendBodyString appends the given string to the request entity body.
//
// Thread-safe: No.
func (req *Request) AppendBodyString(s string) {
	req.RemoveMultipartFormFiles()
	_ = req.closeBodyStream()
	_, _ = req.BodyBuffer().WriteString(s)
}

// ResetBody resets the request body buffer, releases streaming readers, and deletes temporary multipart files.
//
// Thread-safe: No.
func (req *Request) ResetBody() {
	req.bodyRaw = nil
	req.RemoveMultipartFormFiles()
	_ = req.closeBodyStream()

	if req.body != nil {
		if req.KeepBodyBuffer {
			req.body.Reset()
		} else {
			requestBodyPool.Put(req.body)
			req.body = nil
		}
	}
}

// ReleaseBody reclaims the internal body buffer if its capacity exceeds size bytes, reducing GC pressure.
//
// Thread-safe: No.
func (req *Request) ReleaseBody(size int) {
	req.bodyRaw = nil
	if req.body == nil {
		return
	}

	if cap(req.body.B) > size {
		_ = req.closeBodyStream()
		req.body = nil
	}
}

// SwapBody swaps the request body buffer with the provided slice and returns the old body slice.
//
// Thread-safe: No.
func (req *Request) SwapBody(body []byte) []byte {
	bb := req.BodyBuffer()
	if req.bodyStream != nil {
		bb.Reset()
		_, err := copyBodyStream(bb, req.bodyStream)
		_ = req.closeBodyStream()

		if err != nil {
			bb.Reset()
			bb.SetString(err.Error())
		}
	}

	req.bodyRaw = nil
	oldBody := bb.B
	bb.B = body

	return oldBody
}

// CopyTo deep-copies the request contents into dst, excluding streaming readers.
//
// Thread-safe: No.
func (req *Request) CopyTo(dst *Request) {
	req.CopyToSkipBody(dst)

	switch {
	case req.bodyRaw != nil:
		dst.bodyRaw = append(dst.bodyRaw[:0], req.bodyRaw...)
		if dst.body != nil {
			dst.body.Reset()
		}
	case req.body != nil:
		dst.BodyBuffer().Set(req.body.B)
	case dst.body != nil:
		dst.body.Reset()
	}
}

// CopyToSkipBody copies all request metadata (headers, URI, POST parameters, flags) into dst while omitting the entity body.
//
// Thread-safe: No.
func (req *Request) CopyToSkipBody(dst *Request) {
	dst.Reset()
	req.Header.CopyTo(&dst.Header)
	req.uri.CopyTo(&dst.uri)
	dst.parsedURI = req.parsedURI
	dst.uriParseErr = req.uriParseErr
	req.postArgs.CopyTo(&dst.postArgs)
	dst.ParsedPostArgs = req.ParsedPostArgs
	dst.isTLS = req.isTLS
	dst.UseHostHeader = req.UseHostHeader
}

// BodyScoped borrows the request body without memory allocation using a borrow.Scope.
//
// Concurrency & Lifetime:
// The returned borrow.Bytes is valid only for the lifetime of scope s.
func (req *Request) BodyScoped(s *borrow.Scope) borrow.Bytes {
	b := req.Body()
	if len(b) == 0 {
		return borrow.Bytes{}
	}

	return borrow.NewBytes(b, nil)
}

// ReadBodyScoped invokes fn with a borrowed reference to the request body buffer for zero-allocation reading.
//
// Concurrency & Lifetime:
// The byte slice passed to fn must not be referenced after fn returns.
func (req *Request) ReadBodyScoped(fn func([]byte) error) error {
	s := borrow.AcquireScope()
	defer s.Release()

	b := req.Body()

	return fn(b)
}

// BodyGunzip decompresses the request body using Gzip (RFC 1952, RFC 9110 Section 8.4).
func (req *Request) BodyGunzip() ([]byte, error) {
	return req.BodyGunzipWithLimit(0)
}

// BodyGunzipWithLimit decompresses the request body using Gzip with an upper bound of maxBodySize bytes (RFC 1952).
func (req *Request) BodyGunzipWithLimit(maxBodySize int) ([]byte, error) {
	return gunzipData(req.Body(), maxBodySize)
}

// BodyUnbrotli decompresses the request body using Brotli (RFC 7932, RFC 9110 Section 8.4).
func (req *Request) BodyUnbrotli() ([]byte, error) {
	return req.BodyUnbrotliWithLimit(0)
}

// BodyUnbrotliWithLimit decompresses the request body using Brotli with an upper bound of maxBodySize bytes (RFC 7932).
func (req *Request) BodyUnbrotliWithLimit(maxBodySize int) ([]byte, error) {
	return unBrotliData(req.Body(), maxBodySize)
}

// BodyInflate decompresses the request body using Deflate (RFC 1951, RFC 9110 Section 8.4).
func (req *Request) BodyInflate() ([]byte, error) {
	return req.BodyInflateWithLimit(0)
}

// BodyInflateWithLimit decompresses the request body using Deflate with an upper bound of maxBodySize bytes (RFC 1951).
func (req *Request) BodyInflateWithLimit(maxBodySize int) ([]byte, error) {
	return inflateData(req.Body(), maxBodySize)
}

// BodyUnzstd decompresses the request body using Zstandard (RFC 8878, RFC 9110 Section 8.4).
func (req *Request) BodyUnzstd() ([]byte, error) {
	return req.BodyUnzstdWithLimit(0)
}

// BodyUnzstdWithLimit decompresses the request body using Zstandard with an upper bound of maxBodySize bytes (RFC 8878).
func (req *Request) BodyUnzstdWithLimit(maxBodySize int) ([]byte, error) {
	return unzstdData(req.Body(), maxBodySize)
}

// BodyUncompressed inspects the request Content-Encoding header and decompresses the entity body (RFC 9110 Section 8.4).
func (req *Request) BodyUncompressed() ([]byte, error) {
	return req.BodyUncompressedWithLimit(0)
}

// BodyUncompressedWithLimit decompresses the request body according to Content-Encoding up to maxBodySize bytes.
func (req *Request) BodyUncompressedWithLimit(maxBodySize int) ([]byte, error) {
	enc := string(req.Header.ContentEncoding())
	if enc == "" {
		return req.Body(), nil
	}

	if maxBodySize <= 0 {
		res, err := compress.Decompress(enc, req.Body(), nil)
		if err != nil {
			if errors.Is(err, compress.ErrUnsupportedEncoding) {
				return nil, ErrContentEncodingUnsupported
			}

			return nil, err
		}

		return res, nil
	}

	switch enc {
	case "deflate":
		return req.BodyInflateWithLimit(maxBodySize)
	case "gzip":
		return req.BodyGunzipWithLimit(maxBodySize)
	case "br":
		return req.BodyUnbrotliWithLimit(maxBodySize)
	case "zstd":
		return req.BodyUnzstdWithLimit(maxBodySize)
	default:
		return nil, ErrContentEncodingUnsupported
	}
}
