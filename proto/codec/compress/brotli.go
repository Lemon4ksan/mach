// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package compress

import (
	"bytes"
	"fmt"
	"io"

	"github.com/lemon4ksan/foundation/silicon/bytesconv"
	"github.com/lemon4ksan/seal/codec/compress"
	"github.com/lemon4ksan/seal/codec/compress/brotli"
)

// AppendBrotliBytesLevel appends Brotli-compressed src to dst using the specified level (RFC 7932 §9.2).
func AppendBrotliBytesLevel(dst, src []byte, level int) []byte {
	var buf bytes.Buffer
	buf.Write(dst)

	w := brotli.NewWriterLevel(&buf, level)
	_, _ = w.Write(src)
	_ = w.Close()

	return buf.Bytes()
}

// WriteBrotliLevel compresses p with Brotli at the given level and writes it to w (RFC 7932 §9.2).
func WriteBrotliLevel(w io.Writer, p []byte, level int) (int, error) {
	b := AppendBrotliBytesLevel(nil, p, level)
	return w.Write(b)
}

// WriteBrotli compresses p with [CompressBrotliDefaultCompression] and writes it to w (RFC 7932 §9.2).
func WriteBrotli(w io.Writer, p []byte) (int, error) {
	return WriteBrotliLevel(w, p, CompressBrotliDefaultCompression)
}

// AppendBrotliBytes appends Brotli-compressed src to dst using [CompressBrotliDefaultCompression] (RFC 7932 §9.2).
func AppendBrotliBytes(dst, src []byte) []byte {
	return AppendBrotliBytesLevel(dst, src, CompressBrotliDefaultCompression)
}

// WriteUnbrotli writes unbrotlied p to w and returns the number of uncompressed bytes (RFC 7932).
func WriteUnbrotli(w io.Writer, p []byte) (int, error) {
	return WriteUnbrotliLimit(w, p, 0)
}

// WriteUnbrotliLimit decompresses Brotli payload p and writes up to maxBodySize uncompressed bytes to w (RFC 7932).
func WriteUnbrotliLimit(w io.Writer, p []byte, maxBodySize int) (int, error) {
	r := &byteSliceReader{b: p}

	zr, err := compress.AcquireBrotliReader(r)
	if err != nil {
		return 0, err
	}
	defer compress.ReleaseBrotliReader(zr)

	n, err := bytesconv.CopyZeroAllocWithLimit(w, zr, maxBodySize)

	nn := int(n)
	if int64(nn) != n {
		return 0, fmt.Errorf("too much data unbrotlied: %d", n)
	}

	return nn, err
}

// AppendUnbrotliBytes appends unbrotlied src to dst and returns the resulting slice (RFC 7932).
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

// AppendUnbrotliBytesLimit appends unbrotlied src to dst up to maxBodySize uncompressed bytes (RFC 7932).
func AppendUnbrotliBytesLimit(dst, src []byte, maxBodySize int) ([]byte, error) {
	w := &byteSliceWriter{b: dst}
	_, err := WriteUnbrotliLimit(w, src, maxBodySize)
	return w.b, err
}
