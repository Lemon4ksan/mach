// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package compress

import (
	"fmt"
	"io"

	"github.com/lemon4ksan/foundation/silicon/bytesconv"
	"github.com/lemon4ksan/foundation/silicon/pool"
	"github.com/lemon4ksan/seal/codec/compress"
	"github.com/lemon4ksan/seal/codec/compress/gzip"
)

var gzipWriterPoolMap = newGzipWriterPoolMap()

func newGzipWriterPoolMap() [12]*pool.PerCorePool[*gzip.Writer] {
	var m [12]*pool.PerCorePool[*gzip.Writer]
	for i := range 12 {
		lvl := i - 2
		m[i] = pool.NewPerCorePool[*gzip.Writer](func() *gzip.Writer {
			zw, err := gzip.NewWriterLevel(io.Discard, lvl)
			if err != nil {
				if lvl < gzip.HuffmanOnly {
					lvl = gzip.HuffmanOnly
				} else {
					lvl = gzip.BestCompression
				}

				zw, _ = gzip.NewWriterLevel(io.Discard, lvl)
			}

			return zw
		})
	}

	return m
}

// AcquireGzipWriter acquires a zero-alloc pooled Gzip writer targeting w at the given compression level (RFC 1952).
func AcquireGzipWriter(w io.Writer, level int) *gzip.Writer {
	nLevel := normalizeCompressLevel(level)
	zw := gzipWriterPoolMap[nLevel].Get()
	zw.Reset(w)

	return zw
}

// ReleaseGzipWriter flushes and returns zw to its corresponding PerCorePool.
func ReleaseGzipWriter(zw *gzip.Writer, level int) {
	if zw == nil {
		return
	}

	_ = zw.Close()
	nLevel := normalizeCompressLevel(level)
	gzipWriterPoolMap[nLevel].Put(zw)
}

// WriteGzipLevel writes gzipped p to w using the given compression level (RFC 1952).
func WriteGzipLevel(w io.Writer, p []byte, level int) (int, error) {
	zw := AcquireGzipWriter(w, level)
	n, err := zw.Write(p)
	ReleaseGzipWriter(zw, level)

	return n, err
}

// WriteGzip writes gzipped p to w using [CompressDefaultCompression] (RFC 1952).
func WriteGzip(w io.Writer, p []byte) (int, error) {
	return WriteGzipLevel(w, p, CompressDefaultCompression)
}

// AppendGzipBytesLevel appends gzipped src to dst using the given compression level (RFC 1952).
func AppendGzipBytesLevel(dst, src []byte, level int) []byte {
	w := &byteSliceWriter{b: dst}
	_, _ = WriteGzipLevel(w, src, level)
	return w.b
}

// AppendGzipBytes appends gzipped src to dst using [CompressDefaultCompression] (RFC 1952).
func AppendGzipBytes(dst, src []byte) []byte {
	return AppendGzipBytesLevel(dst, src, CompressDefaultCompression)
}

// WriteGunzip writes ungzipped p to w and returns the number of uncompressed bytes (RFC 1952).
func WriteGunzip(w io.Writer, p []byte) (int, error) {
	return WriteGunzipLimit(w, p, 0)
}

// WriteGunzipLimit decompresses gzipped payload p and writes up to maxBodySize uncompressed bytes to w (RFC 1952).
func WriteGunzipLimit(w io.Writer, p []byte, maxBodySize int) (int, error) {
	r := &byteSliceReader{b: p}

	zr, err := compress.AcquireGzipReader(r)
	if err != nil {
		return 0, err
	}
	defer compress.ReleaseGzipReader(zr)

	n, err := bytesconv.CopyZeroAllocWithLimit(w, zr, maxBodySize)

	nn := int(n)
	if int64(nn) != n {
		return 0, fmt.Errorf("too much data gunzipped: %d", n)
	}

	return nn, err
}

// AppendGunzipBytes appends gunzipped src to dst and returns the resulting slice (RFC 1952 §2.2 & §2.3.1).
func AppendGunzipBytes(dst, src []byte) ([]byte, error) {
	if len(dst) == 0 {
		return compress.Gunzip(src, dst)
	}

	res, err := compress.Gunzip(src, nil)
	if err != nil {
		return dst, err
	}

	return append(dst, res...), nil
}

// AppendGunzipBytesLimit decompresses gzipped src and appends up to maxBodySize uncompressed bytes to dst (RFC 1952 §2.2 & §2.3.1).
func AppendGunzipBytesLimit(dst, src []byte, maxBodySize int) ([]byte, error) {
	w := &byteSliceWriter{b: dst}
	_, err := WriteGunzipLimit(w, src, maxBodySize)
	return w.b, err
}
