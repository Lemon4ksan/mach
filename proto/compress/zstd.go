// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package compress

import (
	"fmt"
	"io"

	"github.com/lemon4ksan/foundation/codec/compress"
	"github.com/lemon4ksan/foundation/codec/compress/zstd"
	"github.com/lemon4ksan/foundation/silicon/bytesconv"
)

// Supported Zstandard compression speed levels (RFC 8878).
const (
	// CompressZstdSpeedNotSet indicates default compression speed.
	CompressZstdSpeedNotSet = iota
	// CompressZstdBestSpeed indicates fastest compression speed.
	CompressZstdBestSpeed
	// CompressZstdDefault indicates standard balanced compression speed.
	CompressZstdDefault
	// CompressZstdSpeedBetter indicates improved compression ratio over default.
	CompressZstdSpeedBetter
	// CompressZstdBestCompression indicates maximum compression ratio.
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

// WriteUnzstdLimit decompresses Zstandard payload p and writes up to maxBodySize uncompressed bytes to w (RFC 8878).
//
// If maxBodySize is 0 or negative, uncompressed size is unlimited. If decompression produces
// more than maxBodySize bytes, decompression halts and an error is returned to prevent decompression bombs.
// Concurrency: Thread-safe; utilizes pooled zstd decoders.
func WriteUnzstdLimit(w io.Writer, p []byte, maxBodySize int) (int, error) {
	r := &byteSliceReader{b: p}

	zr, err := acquireZstdReader(r)
	if err != nil {
		return 0, err
	}

	n, err := bytesconv.CopyZeroAllocWithLimit(w, zr, maxBodySize)
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
