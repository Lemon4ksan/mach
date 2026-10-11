// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package stackless

import "io"

// Writer is an interface stackless writer must conform to.
//
// The interface contains common subset for Writers from compress/* packages.
type Writer interface {
	Write(p []byte) (int, error)
	Flush() error
	Close() error
	Reset(w io.Writer)
}

// NewWriterFunc must return new writer that will be wrapped into stackless writer.
type NewWriterFunc func(w io.Writer) Writer

// NewWriter creates a direct zero-alloc writer around a writer returned from newWriter.
func NewWriter(dstW io.Writer, newWriter NewWriterFunc) Writer {
	return newWriter(dstW)
}
