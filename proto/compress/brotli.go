// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package compress

import (
	"bytes"
	"fmt"
	"io"

	"github.com/lemon4ksan/seal/codec/compress"
	"github.com/lemon4ksan/seal/codec/compress/brotli"
	"github.com/lemon4ksan/foundation/silicon/bytesconv"
)

// Supported Brotli compression levels (RFC 7932).
const (
	// CompressBrotliNoCompression disables Brotli compression (RFC 7932).
	CompressBrotliNoCompression = 0
	// CompressBrotliBestSpeed selects the lowest compression level tuned for throughput (RFC 7932).
	CompressBrotliBestSpeed = 1
	// CompressBrotliBestCompression selects the highest compression level tuned for ratio (RFC 7932).
	CompressBrotliBestCompression = 11
	// CompressBrotliDefaultCompression selects the default balanced Brotli compression level (RFC 7932).
	CompressBrotliDefaultCompression = 4
)

func acquireBrotliReader(r io.Reader) (*brotli.Reader, error) {
	return compress.AcquireBrotliReader(r)
}

func releaseBrotliReader(zr *brotli.Reader) {
	compress.ReleaseBrotliReader(zr)
}

// AppendBrotliBytesLevel appends Brotli-compressed src to dst using the specified compression level (RFC 7932).
func AppendBrotliBytesLevel(dst, src []byte, level int) []byte {
	var buf bytes.Buffer
	buf.Write(dst)

	w := brotli.NewWriterLevel(&buf, level)
	_, _ = w.Write(src)
	_ = w.Close()

	return buf.Bytes()
}

// WriteBrotliLevel compresses p with Brotli at the given level and writes it to w (RFC 7932).
func WriteBrotliLevel(w io.Writer, p []byte, level int) (int, error) {
	b := AppendBrotliBytesLevel(nil, p, level)
	return w.Write(b)
}

// WriteBrotli compresses p with [CompressBrotliDefaultCompression] and writes it to w (RFC 7932).
func WriteBrotli(w io.Writer, p []byte) (int, error) {
	return WriteBrotliLevel(w, p, CompressBrotliDefaultCompression)
}

// AppendBrotliBytes appends Brotli-compressed src to dst using [CompressBrotliDefaultCompression] (RFC 7932).
func AppendBrotliBytes(dst, src []byte) []byte {
	return AppendBrotliBytesLevel(dst, src, CompressBrotliDefaultCompression)
}

// WriteUnbrotli decompresses Brotli payload p and writes the uncompressed data to w.
func WriteUnbrotli(w io.Writer, p []byte) (int, error) {
	return WriteUnbrotliLimit(w, p, 0)
}

// WriteUnbrotliLimit decompresses Brotli payload p and writes up to maxBodySize uncompressed bytes to w (RFC 7932).
//
// If maxBodySize is 0 or negative, uncompressed size is unlimited. If decompression produces
// more than maxBodySize bytes, decompression halts and an error is returned to prevent decompression bombs.
func WriteUnbrotliLimit(w io.Writer, p []byte, maxBodySize int) (int, error) {
	r := &byteSliceReader{b: p}

	zr, err := acquireBrotliReader(r)
	if err != nil {
		return 0, err
	}
	defer releaseBrotliReader(zr)

	n, err := bytesconv.CopyZeroAllocWithLimit(w, zr, maxBodySize)
	nn := int(n)
	if int64(nn) != n {
		return 0, fmt.Errorf("too much data unbrotlied: %d", n)
	}

	return nn, err
}

// AppendUnbrotliBytes decompresses Brotli payload src and appends the uncompressed data to dst (RFC 7932).
func AppendUnbrotliBytes(dst, src []byte) ([]byte, error) {
	if len(dst) == 0 {
		return compress.Unbrotli(src, dst)
	}
	res, err := compress.Unbrotli(src, nil)
	if err != nil {
		return dst, err
	}
	return append(dst, res...), nil
}
