// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package compress

import (
	"fmt"
	"io"

	"github.com/lemon4ksan/foundation/silicon/bytesconv"
	"github.com/lemon4ksan/seal/codec/compress"
	"github.com/lemon4ksan/seal/codec/compress/zstd"
)

func acquireZstdReader(r io.Reader) (*zstd.Decoder, error) {
	return compress.AcquireZstdReader(r)
}

func releaseZstdReader(zr *zstd.Decoder) {
	compress.ReleaseZstdReader(zr)
}

// AppendZstdBytesLevel compresses src using Zstandard at the specified level and appends the result to dst (RFC 8878).
func AppendZstdBytesLevel(dst, src []byte, level int) []byte {
	if len(dst) == 0 {
		res, _ := compress.CompressZstd(src, dst, level)
		return res
	}

	res, _ := compress.CompressZstd(src, nil, level)

	return append(dst, res...)
}

// WriteZstdLevel compresses p using Zstandard at the specified level and writes it to w (RFC 8878).
func WriteZstdLevel(w io.Writer, p []byte, level int) (int, error) {
	res, err := compress.CompressZstd(p, nil, level)
	if err != nil {
		return 0, err
	}

	return w.Write(res)
}

// WriteZstd compresses p using [CompressZstdDefault] and writes it to w (RFC 8878).
func WriteZstd(w io.Writer, p []byte) (int, error) {
	return WriteZstdLevel(w, p, CompressZstdDefault)
}

// AppendZstdBytes appends Zstandard-compressed src to dst using [CompressZstdDefault] (RFC 8878).
func AppendZstdBytes(dst, src []byte) []byte {
	return AppendZstdBytesLevel(dst, src, CompressZstdDefault)
}

// WriteUnzstd writes un-zstd'd p to w and returns the number of uncompressed bytes (RFC 8878).
func WriteUnzstd(w io.Writer, p []byte) (int, error) {
	return WriteUnzstdLimit(w, p, 0)
}

// WriteUnzstdLimit decompresses Zstandard payload p and writes up to maxBodySize uncompressed bytes to w (RFC 8878).
func WriteUnzstdLimit(w io.Writer, p []byte, maxBodySize int) (int, error) {
	r := &byteSliceReader{b: p}

	zr, err := acquireZstdReader(r)
	if err != nil {
		return 0, err
	}
	defer releaseZstdReader(zr)

	n, err := bytesconv.CopyZeroAllocWithLimit(w, zr, maxBodySize)

	nn := int(n)
	if int64(nn) != n {
		return 0, fmt.Errorf("too much data unzstd'd: %d", n)
	}

	return nn, err
}

// AppendUnzstdBytes appends un-zstd'd src to dst and returns the resulting slice (RFC 8878).
func AppendUnzstdBytes(dst, src []byte) ([]byte, error) {
	if len(dst) == 0 {
		return compress.Unzstd(src, dst)
	}

	res, err := compress.Unzstd(src, nil)
	if err != nil {
		return dst, err
	}

	return append(dst, res...), nil
}

// AppendUnzstdBytesLimit appends un-zstd'd src to dst up to maxBodySize uncompressed bytes (RFC 8878).
func AppendUnzstdBytesLimit(dst, src []byte, maxBodySize int) ([]byte, error) {
	w := &byteSliceWriter{b: dst}
	_, err := WriteUnzstdLimit(w, src, maxBodySize)
	return w.b, err
}
