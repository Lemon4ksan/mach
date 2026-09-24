// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package http

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"mime/multipart"

	"github.com/lemon4ksan/mach/proto/http/zerocopy"
)

// PostArgs returns the parsed application/x-www-form-urlencoded POST parameters (RFC 1866).
// The arguments are parsed lazily on first access.
//
// Thread-safe: No.
func (req *Request) PostArgs() *zerocopy.Args {
	req.parsePostArgs()
	return &req.postArgs
}

func (req *Request) parsePostArgs() {
	if req.ParsedPostArgs {
		return
	}

	req.ParsedPostArgs = true
	if !bytes.HasPrefix(req.Header.ContentType(), zerocopy.StrPostArgsContentType) {
		return
	}

	req.postArgs.ParseBytes(req.bodyBytes())
}

// MultipartForm returns the parsed multipart/form-data payload (RFC 7578).
// Returns ErrNoMultipartForm if the Content-Type header lacks a valid multipart boundary.
//
// Lifecycle:
// Callers MUST call RemoveMultipartFormFiles after processing the form to purge temporary disk files.
func (req *Request) MultipartForm() (*multipart.Form, error) {
	return req.MultipartFormWithLimit(0)
}

// MultipartFormWithLimit parses the multipart/form-data payload, enforcing a maximum memory/disk budget of maxBodySize bytes (RFC 7578).
//
// Lifecycle:
// Callers MUST call RemoveMultipartFormFiles after processing the form to purge temporary disk files.
func (req *Request) MultipartFormWithLimit(maxBodySize int) (*multipart.Form, error) {
	if req.multipartForm != nil {
		return req.multipartForm, nil
	}

	req.multipartFormBoundary = string(req.Header.MultipartFormBoundary())
	if req.multipartFormBoundary == "" {
		return nil, ErrNoMultipartForm
	}

	var err error

	ce := req.Header.peek(zerocopy.StrContentEncoding)
	if req.bodyStream != nil {
		bodyStream := req.bodyStream

		var lr *io.LimitedReader

		if bytes.Equal(ce, zerocopy.StrGzip) {
			if bodyStream, err = gzip.NewReader(bodyStream); err != nil {
				return nil, fmt.Errorf("cannot gunzip request body: %w", err)
			}
		} else if len(ce) > 0 {
			return nil, fmt.Errorf("unsupported content-encoding: %q", ce)
		}

		if maxBodySize > 0 {
			lr = &io.LimitedReader{R: bodyStream, N: int64(maxBodySize) + 1}
			bodyStream = lr
		}

		mr := multipart.NewReader(bodyStream, req.multipartFormBoundary)

		req.multipartForm, err = mr.ReadForm(8 * 1024)
		if err != nil {
			if lr != nil && lr.N <= 0 {
				return nil, fmt.Errorf("cannot read multipart/form-data body: %w", ErrBodyTooLarge)
			}

			return nil, fmt.Errorf("cannot read multipart/form-data body: %w", err)
		}

		if lr != nil && lr.N <= 0 {
			req.RemoveMultipartFormFiles()
			return nil, fmt.Errorf("cannot read multipart/form-data body: %w", ErrBodyTooLarge)
		}
	} else {
		body := req.bodyBytes()
		if bytes.Equal(ce, zerocopy.StrGzip) {
			if body, err = gunzipData(body, maxBodySize); err != nil {
				return nil, fmt.Errorf("cannot gunzip request body: %w", err)
			}
		} else if len(ce) > 0 {
			return nil, fmt.Errorf("unsupported content-encoding: %q", ce)
		}

		if maxBodySize > 0 && len(body) > maxBodySize {
			return nil, fmt.Errorf("cannot read multipart/form-data body: %w", ErrBodyTooLarge)
		}

		req.multipartForm, err = readMultipartForm(
			bytes.NewReader(body),
			req.multipartFormBoundary,
			len(body),
			len(body),
		)
		if err != nil {
			return nil, err
		}
	}

	return req.multipartForm, nil
}

// RemoveMultipartFormFiles removes any temporary spool files created on disk during multipart/form-data parsing (RFC 7578).
func (req *Request) RemoveMultipartFormFiles() {
	if req.multipartForm != nil {
		_ = req.multipartForm.RemoveAll()
		req.multipartForm = nil
	}

	req.multipartFormBoundary = ""
}

func (req *Request) onlyMultipartForm() bool {
	return req.multipartForm != nil && (req.body == nil || len(req.body.B) == 0)
}

// BodyWriteTo writes the request entity body or multipart payload directly to w.
func (req *Request) BodyWriteTo(w io.Writer) error {
	if req.bodyStream != nil {
		_, err := copyBodyStream(w, req.bodyStream)
		_ = req.closeBodyStream()
		return err
	}

	if req.onlyMultipartForm() {
		return WriteMultipartForm(w, req.multipartForm, req.multipartFormBoundary)
	}

	_, err := w.Write(req.bodyBytes())

	return err
}
