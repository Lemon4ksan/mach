// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package http

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"

	"github.com/lemon4ksan/mach/proto/http/zerocopy"
)

// Read deserializes an HTTP request (including headers and entity body) from the buffered reader r (RFC 9112 Section 2.1).
//
// Thread-safe: No.
func (req *Request) Read(r *bufio.Reader) error {
	return req.ReadLimitBody(r, 0)
}

// ReadLimitBody deserializes an HTTP request from r, rejecting bodies exceeding maxBodySize with ErrBodyTooLarge.
//
// Thread-safe: No.
func (req *Request) ReadLimitBody(r *bufio.Reader, maxBodySize int) error {
	req.resetSkipHeader()

	if err := req.Header.Read(r); err != nil {
		return err
	}

	return req.readLimitBody(r, maxBodySize, false, true)
}

func (req *Request) readLimitBody(r *bufio.Reader, maxBodySize int, getOnly, preParseMultipartForm bool) error {
	if getOnly && !req.Header.IsGet() && !req.Header.IsHead() {
		return ErrGetOnly
	}

	if req.MayContinue() {
		return nil
	}

	return req.ContinueReadBody(r, maxBodySize, preParseMultipartForm)
}

// MayContinue reports whether the request contains an "Expect: 100-continue" header (RFC 9110 Section 10.1.1).
//
// Thread-safe: No.
func (req *Request) MayContinue() bool {
	return bytes.Equal(req.Header.peek(zerocopy.StrExpect), zerocopy.Str100Continue)
}

// ContinueReadBody completes reading the entity body after an interim 100 Continue response has been transmitted (RFC 9110 Section 10.1.1).
//
// Thread-safe: No.
func (req *Request) ContinueReadBody(r *bufio.Reader, maxBodySize int, preParseMultipartForm ...bool) error {
	var err error

	contentLength := req.Header.ContentLength()
	if contentLength > 0 {
		if maxBodySize > 0 && contentLength > maxBodySize {
			return ErrBodyTooLarge
		}

		if len(preParseMultipartForm) == 0 || preParseMultipartForm[0] {
			req.multipartFormBoundary = string(req.Header.MultipartFormBoundary())
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

	if err = req.ReadBody(r, contentLength, maxBodySize); err != nil {
		return err
	}

	if contentLength == -1 {
		err = req.Header.ReadTrailer(r)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return ErrBrokenChunk{error: io.ErrUnexpectedEOF}
			}

			return err
		}
	}

	return nil
}

// ReadBody reads the request body from r according to contentLength and maxBodySize (RFC 9112 Section 6, Section 7).
//
// Thread-safe: No.
func (req *Request) ReadBody(r *bufio.Reader, contentLength, maxBodySize int) (err error) {
	bodyBuf := req.BodyBuffer()
	bodyBuf.Reset()

	switch {
	case contentLength >= 0:
		bodyBuf.B, err = readBody(r, contentLength, maxBodySize, bodyBuf.B)
	case contentLength == -1:
		bodyBuf.B, err = readBodyChunked(r, maxBodySize, bodyBuf.B)
		if err == nil && len(bodyBuf.B) == 0 {
			req.Header.SetContentLength(0)
		}
	default:
		bodyBuf.B, err = readBodyIdentity(r, maxBodySize, bodyBuf.B)
		req.Header.SetContentLength(len(bodyBuf.B))
	}

	if err != nil {
		req.Reset()
		return err
	}

	return nil
}

// Write serializes the HTTP request to the buffered writer w without flushing (RFC 9112 Section 2.1).
//
// Thread-safe: No.
func (req *Request) Write(w *bufio.Writer) error {
	if len(req.Header.Host()) == 0 || req.parsedURI {
		uri := req.URI()

		host := uri.Host()
		if len(req.Header.Host()) == 0 {
			if len(host) == 0 {
				return errRequestHostRequired
			}

			req.Header.SetHostBytes(host)
		} else if !req.UseHostHeader {
			req.Header.SetHostBytes(host)
		}

		req.Header.SetRequestURIBytes(uri.RequestURI())

		if len(uri.Username()) > 0 {
			nl := len(uri.Username()) + len(uri.Password()) + 1
			nb := nl + len(zerocopy.StrBasicSpace)

			tl := nb + base64.StdEncoding.EncodedLen(nl)
			if tl > cap(req.Header.bufV) {
				req.Header.bufV = make([]byte, 0, tl)
			}

			buf := req.Header.bufV[:0]
			buf = append(buf, uri.Username()...)
			buf = append(buf, zerocopy.StrColon...)
			buf = append(buf, uri.Password()...)
			buf = append(buf, zerocopy.StrBasicSpace...)
			base64.StdEncoding.Encode(buf[nb:tl], buf[:nl])
			req.Header.SetBytesKV(zerocopy.StrAuthorization, buf[nl:tl])
		}
	}

	if req.bodyStream != nil {
		return req.writeBodyStream(w)
	}

	body := req.bodyBytes()

	var err error
	if req.onlyMultipartForm() {
		body, err = marshalMultipartForm(req.multipartForm, req.multipartFormBoundary)
		if err != nil {
			return fmt.Errorf("error when marshaling multipart form: %w", err)
		}

		req.Header.SetMultipartFormBoundary(req.multipartFormBoundary)
	}

	hasBody := false

	if len(body) == 0 {
		body = req.postArgs.QueryString()
	}

	if len(body) != 0 || !req.Header.ignoreBody() {
		hasBody = true

		req.Header.SetContentLength(len(body))
	}

	if err = req.Header.Write(w); err != nil {
		return err
	}

	if hasBody {
		_, err = w.Write(body)
	} else if len(body) > 0 {
		if req.SecureErrorLogMessage {
			return errors.New("non-zero body for non-post request")
		}

		return fmt.Errorf("non-zero body for non-post request: body=%q", body)
	}

	return err
}

// WriteTo writes the serialized request to w, implementing the io.WriterTo interface (RFC 9112 Section 2.1).
//
// Thread-safe: No.
func (req *Request) WriteTo(w io.Writer) (int64, error) {
	return writeBufio(req, w)
}

// WriteVectored writes the request headers and body directly to conn via net.Buffers (vectored I/O), eliminating buffer copying.
//
// Thread-safe: No.
func (req *Request) WriteVectored(conn net.Conn) error {
	if len(req.Header.Host()) == 0 || req.parsedURI {
		uri := req.URI()

		host := uri.Host()
		if len(req.Header.Host()) == 0 {
			if len(host) == 0 {
				return errRequestHostRequired
			}

			req.Header.SetHostBytes(host)
		} else if !req.UseHostHeader {
			req.Header.SetHostBytes(host)
		}

		req.Header.SetRequestURIBytes(uri.RequestURI())
	}

	if req.bodyStream != nil {
		bw := acquireBufioWriter(conn)

		err := req.writeBodyStream(bw)
		if err == nil {
			err = bw.Flush()
		}

		releaseBufioWriter(bw)

		return err
	}

	body := req.bodyBytes()

	var err error
	if req.onlyMultipartForm() {
		body, err = marshalMultipartForm(req.multipartForm, req.multipartFormBoundary)
		if err != nil {
			return fmt.Errorf("error when marshaling multipart form: %w", err)
		}

		req.Header.SetMultipartFormBoundary(req.multipartFormBoundary)
	}

	hasBody := false

	if len(body) == 0 {
		body = req.postArgs.QueryString()
	}

	if len(body) != 0 || !req.Header.ignoreBody() {
		hasBody = true

		req.Header.SetContentLength(len(body))
	}

	headerBytes := req.Header.Header()
	if !hasBody || len(body) == 0 {
		_, err = conn.Write(headerBytes)
		return err
	}

	bufs := net.Buffers{headerBytes, body}
	_, err = bufs.WriteTo(conn)

	return err
}

func (req *Request) writeBodyStream(w *bufio.Writer) error {
	var err error

	contentLength := req.Header.ContentLength()
	if contentLength < 0 {
		lrSize := limitedReaderSize(req.bodyStream)
		if lrSize >= 0 {
			contentLength = int(lrSize)
			if int64(contentLength) != lrSize {
				contentLength = -1
			}

			if contentLength >= 0 {
				req.Header.SetContentLength(contentLength)
			}
		}
	}

	if contentLength >= 0 {
		if err = req.Header.Write(w); err == nil {
			err = writeBodyFixedSize(w, req.bodyStream, int64(contentLength))
		}
	} else {
		req.Header.SetContentLength(-1)

		err = req.Header.Write(w)
		if err == nil {
			err = writeBodyChunked(w, req.bodyStream)
		}

		if err == nil {
			err = req.Header.writeTrailer(w)
		}
	}

	errc := req.closeBodyStream()
	if err == nil {
		err = errc
	}

	return err
}
