// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package compress

import (
	"bytes"
	"fmt"
	"io"

	"github.com/lemon4ksan/foundation/codec/compress"
	"github.com/lemon4ksan/foundation/codec/compress/brotli"
	"github.com/lemon4ksan/foundation/silicon/bytesconv"
)

// Supported compression levels.
const (
	CompressBrotliNoCompression      = 0
	CompressBrotliBestSpeed          = 1
	CompressBrotliBestCompression    = 11
	CompressBrotliDefaultCompression = 4
)

func acquireBrotliReader(r io.Reader) (*brotli.Reader, error) {
	return compress.AcquireBrotliReader(r)
}

func releaseBrotliReader(zr *brotli.Reader) {
	compress.ReleaseBrotliReader(zr)
}

// AppendBrotliBytesLevel appends brotlied src to dst.
func AppendBrotliBytesLevel(dst, src []byte, level int) []byte {
	var buf bytes.Buffer
	buf.Write(dst)

	w := brotli.NewWriterLevel(&buf, level)
	_, _ = w.Write(src)
	_ = w.Close()

	return buf.Bytes()
}

// WriteBrotliLevel writes p to w.
func WriteBrotliLevel(w io.Writer, p []byte, level int) (int, error) {
	b := AppendBrotliBytesLevel(nil, p, level)
	return w.Write(b)
}

// WriteBrotli writes p to w.
func WriteBrotli(w io.Writer, p []byte) (int, error) {
	return WriteBrotliLevel(w, p, CompressBrotliDefaultCompression)
}

// AppendBrotliBytes appends src to dst.
func AppendBrotliBytes(dst, src []byte) []byte {
	return AppendBrotliBytesLevel(dst, src, CompressBrotliDefaultCompression)
}

// WriteUnbrotli writes unbrotlied p to w and returns the number of uncompressed bytes written to w.
func WriteUnbrotli(w io.Writer, p []byte) (int, error) {
	return WriteUnbrotliLimit(w, p, 0)
}

func WriteUnbrotliLimit(w io.Writer, p []byte, maxBodySize int) (int, error) {
	r := &byteSliceReader{b: p}

	zr, err := acquireBrotliReader(r)
	if err != nil {
		return 0, err
	}

	n, err := bytesconv.CopyZeroAllocWithLimit(w, zr, maxBodySize)
	releaseBrotliReader(zr)

	nn := int(n)
	if int64(nn) != n {
		return 0, fmt.Errorf("too much data unbrotlied: %d", n)
	}

	return nn, err
}

// AppendUnbrotliBytes appends unbrotlied src to dst and returns the resulting dst.
func AppendUnbrotliBytes(dst, src []byte) ([]byte, error) {
	return compress.Unbrotli(src, dst)
}
