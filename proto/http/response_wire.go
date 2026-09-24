// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package http

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"

	machcompress "github.com/lemon4ksan/mach/proto/compress"
	"github.com/lemon4ksan/mach/proto/http/status"
	"github.com/lemon4ksan/mach/proto/http/zerocopy"
)

// Read deserializes an HTTP response (handling interim 1xx responses like Early Hints) from r (RFC 9110 Section 15.2, RFC 9112 Section 2.1).
//
// Thread-safe: No.
func (resp *Response) Read(r *bufio.Reader) error {
	return resp.ReadLimitBody(r, 0)
}

// ReadLimitBody deserializes an HTTP response from r, rejecting bodies exceeding maxBodySize with ErrBodyTooLarge.
//
// Thread-safe: No.
func (resp *Response) ReadLimitBody(r *bufio.Reader, maxBodySize int) error {
	resp.resetSkipHeader()

	err := resp.Header.Read(r)
	if err != nil {
		return err
	}

	for n := 0; ; n++ {
		if resp.Header.statusCode < 100 || resp.Header.statusCode > 199 ||
			resp.Header.statusCode == status.SwitchingProtocols {
			break
		}

		if n >= maxInterimResponses {
			return errTooManyInterimResponses
		}

		if resp.OnInterimResponse != nil {
			resp.OnInterimResponse(resp.Header.statusCode, &resp.Header)
		}

		if err = resp.Header.Read(r); err != nil {
			return err
		}
	}

	if !resp.mustSkipBody() {
		err = resp.ReadBody(r, maxBodySize)
		if err != nil {
			return err
		}
	}

	if resp.Header.ContentLength() == -1 && !resp.StreamBody && !resp.mustSkipBody() {
		err = resp.Header.ReadTrailer(r)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return ErrBrokenChunk{error: io.ErrUnexpectedEOF}
			}

			return err
		}
	}

	return nil
}

// ReadBody reads the response entity body from r according to Content-Length or Chunked Transfer Coding (RFC 9112 Section 6, Section 7).
//
// Thread-safe: No.
func (resp *Response) ReadBody(r *bufio.Reader, maxBodySize int) (err error) {
	bodyBuf := resp.BodyBuffer()
	bodyBuf.Reset()

	contentLength := resp.Header.ContentLength()
	switch {
	case contentLength >= 0:
		bodyBuf.B, err = readBody(r, contentLength, maxBodySize, bodyBuf.B)
		if errors.Is(err, ErrBodyTooLarge) && resp.StreamBody {
			resp.bodyStream = AcquireRequestStream(bodyBuf, r, &resp.Header)
			err = nil
		}

	case contentLength == -1:
		if resp.StreamBody {
			resp.bodyStream = AcquireRequestStream(bodyBuf, r, &resp.Header)
		} else {
			bodyBuf.B, err = readBodyChunked(r, maxBodySize, bodyBuf.B)
		}

	default:
		if resp.StreamBody {
			resp.bodyStream = AcquireRequestStream(bodyBuf, r, &resp.Header)
		} else {
			bodyBuf.B, err = readBodyIdentity(r, maxBodySize, bodyBuf.B)
			resp.Header.SetContentLength(len(bodyBuf.B))
		}
	}

	if err == nil && resp.StreamBody && resp.bodyStream == nil {
		resp.bodyStream = bytes.NewReader(bodyBuf.B)
	}

	return err
}

func (resp *Response) mustSkipBody() bool {
	return resp.SkipBody || resp.Header.mustSkipContentLength()
}

// WriteTo writes the serialized response to w, implementing io.WriterTo (RFC 9112 Section 2.1).
//
// Thread-safe: No.
func (resp *Response) WriteTo(w io.Writer) (int64, error) {
	return writeBufio(resp, w)
}

// BodyWriteTo writes the response entity body directly to w.
func (resp *Response) BodyWriteTo(w io.Writer) error {
	if resp.bodyStream != nil {
		_, err := copyBodyStream(w, resp.bodyStream)
		_ = resp.closeBodyStream(err)
		return err
	}

	_, err := w.Write(resp.bodyBytes())

	return err
}

// WriteGzip compresses the response body using default Gzip compression and writes the response to w (RFC 1952, RFC 9110 Section 8.4).
// Automatically sets "Content-Encoding: gzip" and appends "Vary: Accept-Encoding" (RFC 9110 Section 12.5.5).
func (resp *Response) WriteGzip(w *bufio.Writer) error {
	return resp.WriteGzipLevel(w, machcompress.CompressDefaultCompression)
}

// WriteGzipLevel compresses the response body using the specified Gzip compression level and writes to w (RFC 1952).
func (resp *Response) WriteGzipLevel(w *bufio.Writer, level int) error {
	resp.gzipBody(level)
	return resp.Write(w)
}

// WriteDeflate compresses the response body using default Deflate compression and writes to w (RFC 1951, RFC 9110 Section 8.4).
// Automatically sets "Content-Encoding: deflate" and appends "Vary: Accept-Encoding" (RFC 9110 Section 12.5.5).
func (resp *Response) WriteDeflate(w *bufio.Writer) error {
	return resp.WriteDeflateLevel(w, machcompress.CompressDefaultCompression)
}

// WriteDeflateLevel compresses the response body using the specified Deflate compression level and writes to w (RFC 1951).
func (resp *Response) WriteDeflateLevel(w *bufio.Writer, level int) error {
	resp.deflateBody(level)
	return resp.Write(w)
}

// WriteBrotli compresses the response body using default Brotli compression and writes to w (RFC 7932, RFC 9110 Section 8.4).
// Automatically sets "Content-Encoding: br" and appends "Vary: Accept-Encoding" (RFC 9110 Section 12.5.5).
func (resp *Response) WriteBrotli(w *bufio.Writer) error {
	return resp.WriteBrotliLevel(w, machcompress.CompressBrotliDefaultCompression)
}

// WriteBrotliLevel compresses the response body using the specified Brotli compression level and writes to w (RFC 7932).
func (resp *Response) WriteBrotliLevel(w *bufio.Writer, level int) error {
	resp.brotliBody(level)
	return resp.Write(w)
}

// WriteZstd compresses the response body using default Zstandard compression and writes to w (RFC 8878, RFC 9110 Section 8.4).
// Automatically sets "Content-Encoding: zstd" and appends "Vary: Accept-Encoding" (RFC 9110 Section 12.5.5).
func (resp *Response) WriteZstd(w *bufio.Writer) error {
	return resp.WriteZstdLevel(w, machcompress.CompressZstdDefault)
}

// WriteZstdLevel compresses the response body using the specified Zstandard compression level and writes to w (RFC 8878).
func (resp *Response) WriteZstdLevel(w *bufio.Writer, level int) error {
	resp.zstdBody(level)
	return resp.Write(w)
}

func (resp *Response) brotliBody(level int) {
	if len(resp.Header.ContentEncoding()) > 0 {
		return
	}

	if !resp.Header.isCompressibleContentType() {
		return
	}

	if resp.bodyStream != nil {
		resp.Header.SetContentLength(-1)
		resp.bodyStream = newCompressedBodyStream(resp.bodyStream, level, compressBrotliBodyStream)
	} else {
		bodyBytes := resp.bodyBytes()
		if len(bodyBytes) < minCompressLen {
			return
		}

		w := responseBodyPool.Get()
		w.B = machcompress.AppendBrotliBytesLevel(w.B, bodyBytes, level)

		if resp.body != nil {
			responseBodyPool.Put(resp.body)
		}

		resp.body = w
		resp.bodyRaw = nil
	}

	resp.Header.SetContentEncodingBytes(zerocopy.StrBr)
	resp.Header.addVaryBytes(zerocopy.StrAcceptEncoding)
}

func (resp *Response) gzipBody(level int) {
	if len(resp.Header.ContentEncoding()) > 0 {
		return
	}

	if !resp.Header.isCompressibleContentType() {
		return
	}

	if resp.bodyStream != nil {
		resp.Header.SetContentLength(-1)
		resp.bodyStream = newCompressedBodyStream(resp.bodyStream, level, compressGzipBodyStream)
	} else {
		bodyBytes := resp.bodyBytes()
		if len(bodyBytes) < minCompressLen {
			return
		}

		w := responseBodyPool.Get()
		w.B = machcompress.AppendGzipBytesLevel(w.B, bodyBytes, level)

		if resp.body != nil {
			responseBodyPool.Put(resp.body)
		}

		resp.body = w
		resp.bodyRaw = nil
	}

	resp.Header.SetContentEncodingBytes(zerocopy.StrGzip)
	resp.Header.addVaryBytes(zerocopy.StrAcceptEncoding)
}

func (resp *Response) deflateBody(level int) {
	if len(resp.Header.ContentEncoding()) > 0 {
		return
	}

	if !resp.Header.isCompressibleContentType() {
		return
	}

	if resp.bodyStream != nil {
		resp.Header.SetContentLength(-1)
		resp.bodyStream = newCompressedBodyStream(resp.bodyStream, level, compressDeflateBodyStream)
	} else {
		bodyBytes := resp.bodyBytes()
		if len(bodyBytes) < minCompressLen {
			return
		}

		w := responseBodyPool.Get()
		w.B = machcompress.AppendDeflateBytesLevel(w.B, bodyBytes, level)

		if resp.body != nil {
			responseBodyPool.Put(resp.body)
		}

		resp.body = w
		resp.bodyRaw = nil
	}

	resp.Header.SetContentEncodingBytes(zerocopy.StrDeflate)
	resp.Header.addVaryBytes(zerocopy.StrAcceptEncoding)
}

func (resp *Response) zstdBody(level int) {
	if len(resp.Header.ContentEncoding()) > 0 {
		return
	}

	if !resp.Header.isCompressibleContentType() {
		return
	}

	if resp.bodyStream != nil {
		resp.Header.SetContentLength(-1)
		resp.bodyStream = newCompressedBodyStream(resp.bodyStream, level, compressZstdBodyStream)
	} else {
		bodyBytes := resp.bodyBytes()
		if len(bodyBytes) < minCompressLen {
			return
		}

		w := responseBodyPool.Get()
		w.B = machcompress.AppendZstdBytesLevel(w.B, bodyBytes, level)

		if resp.body != nil {
			responseBodyPool.Put(resp.body)
		}

		resp.body = w
		resp.bodyRaw = nil
	}

	resp.Header.SetContentEncodingBytes(zerocopy.StrZstd)
	resp.Header.addVaryBytes(zerocopy.StrAcceptEncoding)
}

// Write serializes the HTTP response message to w without flushing (RFC 9112 Section 2.1).
//
// Thread-safe: No.
func (resp *Response) Write(w *bufio.Writer) error {
	sendBody := !resp.mustSkipBody()
	if resp.bodyStream != nil {
		return resp.writeBodyStream(w, sendBody)
	}

	body := resp.bodyBytes()

	bodyLen := len(body)
	if sendBody || bodyLen > 0 {
		resp.Header.SetContentLength(bodyLen)
	}

	if err := resp.Header.Write(w); err != nil {
		return err
	}

	if sendBody {
		if _, err := w.Write(body); err != nil {
			return err
		}
	}

	return nil
}

func (resp *Response) writeBodyStream(w *bufio.Writer, sendBody bool) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = &ErrBodyStreamWritePanic{error: fmt.Errorf("panic while writing body stream: %+v", r)}
		}
	}()

	contentLength := resp.Header.ContentLength()
	if contentLength < 0 {
		lrSize := limitedReaderSize(resp.bodyStream)
		if lrSize >= 0 {
			contentLength = int(lrSize)
			if int64(contentLength) != lrSize {
				contentLength = -1
			}

			if contentLength >= 0 {
				resp.Header.SetContentLength(contentLength)
			}
		}
	}

	if contentLength >= 0 {
		if err = resp.Header.Write(w); err == nil {
			if resp.ImmediateHeaderFlush {
				err = w.Flush()
			}

			if err == nil && sendBody {
				err = writeBodyFixedSize(w, resp.bodyStream, int64(contentLength))
			}
		}
	} else {
		resp.Header.SetContentLength(-1)

		if err = resp.Header.Write(w); err == nil {
			if resp.ImmediateHeaderFlush {
				err = w.Flush()
			}

			if err == nil && sendBody {
				err = writeBodyChunked(w, resp.bodyStream)
			}

			if err == nil {
				err = resp.Header.writeTrailer(w)
			}
		}
	}

	errc := resp.closeBodyStream(err)
	if err == nil {
		err = errc
	}

	return err
}
