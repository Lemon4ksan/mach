// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package compress

import (
	"fmt"
	"io"

	"github.com/lemon4ksan/foundation/codec/compress"
	"github.com/lemon4ksan/foundation/codec/compress/zstd"
	"github.com/lemon4ksan/mach/core/bytesutil"
)

const (
	CompressZstdSpeedNotSet = iota
	CompressZstdBestSpeed
	CompressZstdDefault
	CompressZstdSpeedBetter
	CompressZstdBestCompression
)

func acquireZstdReader(r io.Reader) (*zstd.Decoder, error) {
	return compress.AcquireZstdReader(r)
}

func releaseZstdReader(zr *zstd.Decoder) {
	compress.ReleaseZstdReader(zr)
}

// AppendZstdBytesLevel appends src to dst.
func AppendZstdBytesLevel(dst, src []byte, level int) []byte {
	res, _ := compress.CompressZstd(src, dst, level)
	return res
}

// WriteZstdLevel writes p to w.
func WriteZstdLevel(w io.Writer, p []byte, level int) (int, error) {
	res, err := compress.CompressZstd(p, nil, level)
	if err != nil {
		return 0, err
	}
	return w.Write(res)
}

// WriteZstd writes p to w.
func WriteZstd(w io.Writer, p []byte) (int, error) {
	return WriteZstdLevel(w, p, CompressZstdDefault)
}

// AppendZstdBytes appends src to dst.
func AppendZstdBytes(dst, src []byte) []byte {
	return AppendZstdBytesLevel(dst, src, CompressZstdDefault)
}

// WriteUnzstd writes unzstd p to w and returns the number of uncompressed bytes written to w.
func WriteUnzstd(w io.Writer, p []byte) (int, error) {
	return WriteUnzstdLimit(w, p, 0)
}

func WriteUnzstdLimit(w io.Writer, p []byte, maxBodySize int) (int, error) {
	r := &byteSliceReader{b: p}

	zr, err := acquireZstdReader(r)
	if err != nil {
		return 0, err
	}

	n, err := bytesutil.CopyZeroAllocWithLimit(w, zr, maxBodySize)
	releaseZstdReader(zr)

	nn := int(n)
	if int64(nn) != n {
		return 0, fmt.Errorf("too much data unzstd: %d", n)
	}

	return nn, err
}

// AppendUnzstdBytes appends unzstd src to dst and returns the resulting dst.
func AppendUnzstdBytes(dst, src []byte) ([]byte, error) {
	return compress.Unzstd(src, dst)
}
