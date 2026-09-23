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

type responseBodyWriter struct{ r *Response }

func (w *responseBodyWriter) Write(p []byte) (int, error) {
	w.r.AppendBody(p)
	return len(p), nil
}

func (w *responseBodyWriter) WriteString(s string) (int, error) {
	w.r.AppendBodyString(s)
	return len(s), nil
}

// SwapResponseBody swaps response body between a and b per RFC 9110 Section 6.4.
//
// Concurrency: Not thread-safe.
func SwapResponseBody(a, b *Response) {
	a.body, b.body = b.body, a.body
	a.bodyRaw, b.bodyRaw = b.bodyRaw, a.bodyRaw
	a.bodyStream, b.bodyStream = b.bodyStream, a.bodyStream
}

// Body returns the response entity body as a byte slice (RFC 9110 Section 6.4).
// If the body is backed by a stream, Body reads the entire stream into memory.
//
// Thread-safe: No.
func (resp *Response) Body() []byte {
	if resp.bodyStream != nil {
		bodyBuf := resp.BodyBuffer()
		bodyBuf.Reset()
		_, err := copyBodyStream(bodyBuf, resp.bodyStream)
		_ = resp.closeBodyStream(err)

		if err != nil {
			bodyBuf.SetString(err.Error())
		}
	}

	return resp.bodyBytes()
}

func (resp *Response) bodyBytes() []byte {
	if resp.bodyRaw != nil {
		return resp.bodyRaw
	}

	if resp.body == nil {
		return nil
	}

	return resp.body.B
}

// BodyBuffer returns the underlying ByteBuffer storing the in-memory response body.
// If no buffer is currently allocated, one is acquired from the per-P response body pool.
//
// Thread-safe: No.
func (resp *Response) BodyBuffer() *bytesconv.ByteBuffer {
	if resp.body == nil {
		resp.body = responseBodyPool.Get()
	}

	resp.bodyRaw = nil

	return resp.body
}

// SetBody sets the response body to a copy of body (RFC 9110 Section 6.4).
//
// Thread-safe: No.
func (resp *Response) SetBody(body []byte) {
	_ = resp.closeBodyStream(nil)
	bodyBuf := resp.BodyBuffer()
	bodyBuf.Reset()
	_, _ = bodyBuf.Write(body)
}

// SetBodyString sets the response body to the provided string content.
//
// Thread-safe: No.
func (resp *Response) SetBodyString(body string) {
	_ = resp.closeBodyStream(nil)
	bodyBuf := resp.BodyBuffer()
	bodyBuf.Reset()
	_, _ = bodyBuf.WriteString(body)
}

// SetBodyRaw sets the response body to point directly to body without copying.
//
// Thread-safe: No.
func (resp *Response) SetBodyRaw(body []byte) {
	resp.ResetBody()
	resp.bodyRaw = body
}

// AppendBody appends the given byte slice to the response entity body.
//
// Thread-safe: No.
func (resp *Response) AppendBody(p []byte) {
	_ = resp.closeBodyStream(nil)
	_, _ = resp.BodyBuffer().Write(p)
}

// AppendBodyString appends the given string to the response entity body.
//
// Thread-safe: No.
func (resp *Response) AppendBodyString(s string) {
	_ = resp.closeBodyStream(nil)
	_, _ = resp.BodyBuffer().WriteString(s)
}

// ResetBody clears the response entity body and returns large buffers to the pool.
//
// Thread-safe: No.
func (resp *Response) ResetBody() {
	resp.bodyRaw = nil
	_ = resp.closeBodyStream(nil)

	if resp.body != nil {
		if resp.KeepBodyBuffer {
			resp.body.Reset()
		} else {
			responseBodyPool.Put(resp.body)
			resp.body = nil
		}
	}
}

// ReleaseBody reclaims the internal body buffer if its capacity exceeds size bytes.
//
// Thread-safe: No.
func (resp *Response) ReleaseBody(size int) {
	resp.bodyRaw = nil
	if resp.body == nil {
		return
	}

	if cap(resp.body.B) > size {
		_ = resp.closeBodyStream(nil)
		resp.body = nil
	}
}

// SwapBody swaps the response body buffer with the provided slice and returns the old body slice.
//
// Thread-safe: No.
func (resp *Response) SwapBody(body []byte) []byte {
	bb := resp.BodyBuffer()
	if resp.bodyStream != nil {
		bb.Reset()
		_, err := copyBodyStream(bb, resp.bodyStream)
		_ = resp.closeBodyStream(err)

		if err != nil {
			bb.Reset()
			bb.SetString(err.Error())
		}
	}

	resp.bodyRaw = nil
	oldBody := bb.B
	bb.B = body

	return oldBody
}

// CopyTo deep-copies the response contents into dst, excluding streaming readers.
//
// Thread-safe: No.
func (resp *Response) CopyTo(dst *Response) {
	resp.CopyToSkipBody(dst)

	switch {
	case resp.bodyRaw != nil:
		dst.bodyRaw = append(dst.bodyRaw, resp.bodyRaw...)
		if dst.body != nil {
			dst.body.Reset()
		}
	case resp.body != nil:
		dst.BodyBuffer().Set(resp.body.B)
	case dst.body != nil:
		dst.body.Reset()
	}
}

// CopyToSkipBody copies all response metadata (headers, status code, addresses) into dst while omitting the entity body.
//
// Thread-safe: No.
func (resp *Response) CopyToSkipBody(dst *Response) {
	dst.Reset()
	resp.Header.CopyTo(&dst.Header)
	dst.SkipBody = resp.SkipBody
	dst.raddr = resp.raddr
	dst.laddr = resp.laddr
}

// BodyScoped borrows the response body without memory allocation using a borrow.Scope.
func (resp *Response) BodyScoped(s *borrow.Scope) borrow.Bytes {
	b := resp.Body()
	if len(b) == 0 {
		return borrow.Bytes{}
	}

	return borrow.NewBytes(b, nil)
}

// ReadBodyScoped invokes fn with a borrowed reference to the response body buffer.
func (resp *Response) ReadBodyScoped(fn func([]byte) error) error {
	s := borrow.AcquireScope()
	defer s.Release()

	b := resp.Body()

	return fn(b)
}

// BodyGunzip decompresses the response body using Gzip (RFC 1952, RFC 9110 Section 8.4).
func (resp *Response) BodyGunzip() ([]byte, error) {
	return resp.BodyGunzipWithLimit(0)
}

// BodyGunzipWithLimit decompresses the response body using Gzip with an upper bound of maxBodySize bytes (RFC 1952).
func (resp *Response) BodyGunzipWithLimit(maxBodySize int) ([]byte, error) {
	return gunzipData(resp.Body(), maxBodySize)
}

// BodyUnbrotli decompresses the response body using Brotli (RFC 7932, RFC 9110 Section 8.4).
func (resp *Response) BodyUnbrotli() ([]byte, error) {
	return resp.BodyUnbrotliWithLimit(0)
}

// BodyUnbrotliWithLimit decompresses the response body using Brotli with an upper bound of maxBodySize bytes (RFC 7932).
func (resp *Response) BodyUnbrotliWithLimit(maxBodySize int) ([]byte, error) {
	return unBrotliData(resp.Body(), maxBodySize)
}

// BodyInflate decompresses the response body using Deflate (RFC 1951, RFC 9110 Section 8.4).
func (resp *Response) BodyInflate() ([]byte, error) {
	return resp.BodyInflateWithLimit(0)
}

// BodyInflateWithLimit decompresses the response body using Deflate with an upper bound of maxBodySize bytes (RFC 1951).
func (resp *Response) BodyInflateWithLimit(maxBodySize int) ([]byte, error) {
	return inflateData(resp.Body(), maxBodySize)
}

// BodyUnzstd decompresses the response body using Zstandard (RFC 8878, RFC 9110 Section 8.4).
func (resp *Response) BodyUnzstd() ([]byte, error) {
	return resp.BodyUnzstdWithLimit(0)
}

// BodyUnzstdWithLimit decompresses the response body using Zstandard with an upper bound of maxBodySize bytes (RFC 8878).
func (resp *Response) BodyUnzstdWithLimit(maxBodySize int) ([]byte, error) {
	return unzstdData(resp.Body(), maxBodySize)
}

// BodyUncompressed inspects the response Content-Encoding header and decompresses the entity body (RFC 9110 Section 8.4).
func (resp *Response) BodyUncompressed() ([]byte, error) {
	return resp.BodyUncompressedWithLimit(0)
}

// BodyUncompressedWithLimit decompresses the response body according to Content-Encoding up to maxBodySize bytes.
func (resp *Response) BodyUncompressedWithLimit(maxBodySize int) ([]byte, error) {
	enc := string(resp.Header.ContentEncoding())
	if enc == "" {
		return resp.Body(), nil
	}

	if maxBodySize <= 0 {
		res, err := compress.Decompress(enc, resp.Body(), nil)
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
		return resp.BodyInflateWithLimit(maxBodySize)
	case "gzip":
		return resp.BodyGunzipWithLimit(maxBodySize)
	case "br":
		return resp.BodyUnbrotliWithLimit(maxBodySize)
	case "zstd":
		return resp.BodyUnzstdWithLimit(maxBodySize)
	default:
		return nil, ErrContentEncodingUnsupported
	}
}
