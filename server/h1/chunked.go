// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h1

import (
	"bufio"

	"github.com/lemon4ksan/foundation/silicon/bytesconv"

	protoh1 "github.com/lemon4ksan/mach/proto/h1"
)

var (
	// ErrInvalidChunkSize is returned when a chunk-size line cannot be parsed as valid hexadecimal (RFC 9112 §7.1).
	ErrInvalidChunkSize = protoh1.ErrInvalidChunkSize

	// ErrChunkBoundaryError is returned when chunk data is not followed by CRLF (RFC 9112 §7.1).
	ErrChunkBoundaryError = protoh1.ErrChunkBoundaryError
)

// ChunkedReader decodes an HTTP/1.1 chunked transfer-encoded byte stream (RFC 9112 §7.1).
type ChunkedReader = protoh1.ChunkedReader

// NewChunkedReader creates a ChunkedReader wrapping the provided bufio.Reader (RFC 9112 §7.1).
func NewChunkedReader(r *bufio.Reader) *ChunkedReader {
	return protoh1.NewChunkedReader(r)
}

// ReadAllChunked drains all chunked content into a preallocated byte slice up to maxBodySize (RFC 9112 §7.1, RFC 9110 §8.6).
func ReadAllChunked(r *bufio.Reader, maxBodySize int64) ([]byte, error) {
	return protoh1.ReadAllChunked(r, maxBodySize)
}

// ChunkedWriter writes data using HTTP/1.1 chunked transfer coding (RFC 9112 §7.1).
type ChunkedWriter = protoh1.ChunkedWriter

// NewChunkedWriter creates a new ChunkedWriter wrapping w (RFC 9112 §7.1).
func NewChunkedWriter(w *bytesconv.ByteBuffer) *ChunkedWriter {
	return protoh1.NewChunkedWriter(w)
}
