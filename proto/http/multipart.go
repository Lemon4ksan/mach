// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package http

import (
	"errors"
	"fmt"
	"io"
	"mime/multipart"

	"github.com/lemon4ksan/foundation/silicon/bytesconv"
)

const defaultMaxInMemoryFileSize = 16 * 1024 * 1024

// ErrNoMultipartForm is returned when attempting to parse or write a multipart form
// on a request whose Content-Type is not multipart/form-data or lacks a valid boundary parameter
// (RFC 7578 Section 4.2 / RFC 9110 Section 8.6).
var ErrNoMultipartForm = errors.New("mach: request content-type has bad boundary or is not multipart/form-data")

func marshalMultipartForm(f *multipart.Form, boundary string) ([]byte, error) {
	var buf bytesconv.ByteBuffer
	if err := WriteMultipartForm(&buf, f, boundary); err != nil {
		return nil, err
	}

	return buf.B, nil
}

// WriteMultipartForm serializes the multipart form f using boundary to w per RFC 7578.
//
// It streams all form field key-value pairs and file attachments using zero-allocation
// chunked buffer transfers. Returns an error if boundary is empty or if socket I/O fails.
//
// Concurrency: Caller must ensure f is not concurrently modified during serialization.
func WriteMultipartForm(w io.Writer, f *multipart.Form, boundary string) error {
	if boundary == "" {
		return errors.New("form boundary cannot be empty")
	}

	mw := multipart.NewWriter(w)
	if err := mw.SetBoundary(boundary); err != nil {
		return fmt.Errorf("cannot use form boundary %q: %w", boundary, err)
	}

	for k, vv := range f.Value {
		for _, v := range vv {
			if err := mw.WriteField(k, v); err != nil {
				return fmt.Errorf("cannot write form field %q value %q: %w", k, v, err)
			}
		}
	}

	for k, fvv := range f.File {
		for _, fv := range fvv {
			vw, err := mw.CreatePart(fv.Header)
			if err != nil {
				return fmt.Errorf("cannot create form file %q (%q): %w", k, fv.Filename, err)
			}

			fh, err := fv.Open()
			if err != nil {
				return fmt.Errorf("cannot open form file %q (%q): %w", k, fv.Filename, err)
			}

			if _, err = copyZeroAlloc(vw, fh); err != nil {
				_ = fh.Close()
				return fmt.Errorf("error when copying form file %q (%q): %w", k, fv.Filename, err)
			}

			if err = fh.Close(); err != nil {
				return fmt.Errorf("cannot close form file %q (%q): %w", k, fv.Filename, err)
			}
		}
	}

	if err := mw.Close(); err != nil {
		return fmt.Errorf("error when closing multipart form writer: %w", err)
	}

	return nil
}

func readMultipartForm(r io.Reader, boundary string, size, maxInMemoryFileSize int) (*multipart.Form, error) {
	if size <= 0 {
		return nil, fmt.Errorf("form size must be greater than 0: given %d", size)
	}

	lr := io.LimitReader(r, int64(size))
	mr := multipart.NewReader(lr, boundary)

	f, err := mr.ReadForm(int64(maxInMemoryFileSize))
	if err != nil {
		return nil, fmt.Errorf("cannot read multipart/form-data body: %w", err)
	}

	return f, nil
}
