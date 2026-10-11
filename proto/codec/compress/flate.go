// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package compress

import (
	"compress/zlib"
	"fmt"
	"io"

	"github.com/lemon4ksan/foundation/silicon/bytesconv"
	"github.com/lemon4ksan/foundation/silicon/pool"
)

var deflateWriterPoolMap = newDeflateWriterPoolMap()

func newDeflateWriterPoolMap() [12]*pool.PerCorePool[*zlib.Writer] {
	var m [12]*pool.PerCorePool[*zlib.Writer]
	for i := range 12 {
		lvl := i - 2
		m[i] = pool.NewPerCorePool[*zlib.Writer](func() *zlib.Writer {
			zw, err := zlib.NewWriterLevel(io.Discard, lvl)
			if err != nil {
				if lvl < zlib.HuffmanOnly {
					lvl = zlib.HuffmanOnly
				} else {
					lvl = zlib.BestCompression
				}

				zw, _ = zlib.NewWriterLevel(io.Discard, lvl)
			}

			return zw
		})
	}

	return m
}

var flateReaderPool = pool.NewPerCorePool[io.ReadCloser](func() io.ReadCloser {
	return nil
})

func acquireFlateReader(r io.Reader) (io.ReadCloser, error) {
	zr := flateReaderPool.Get()
	if zr == nil {
		return zlib.NewReader(r)
	}

	zrr, ok := zr.(zlib.Resetter)
	if !ok {
		_ = zr.Close()
		return zlib.NewReader(r)
	}

	if err := zrr.Reset(r, nil); err != nil {
		_ = zr.Close()
		return nil, err
	}

	return zr, nil
}

func releaseFlateReader(zr io.ReadCloser) {
	if zr == nil {
		return
	}

	_ = zr.Close()
	flateReaderPool.Put(zr)
}

// AcquireDeflateWriter acquires a zero-alloc pooled Deflate/zlib writer targeting w at level (RFC 1950, RFC 1951).
func AcquireDeflateWriter(w io.Writer, level int) *zlib.Writer {
	nLevel := normalizeCompressLevel(level)
	zw := deflateWriterPoolMap[nLevel].Get()
	zw.Reset(w)

	return zw
}

// ReleaseDeflateWriter flushes and returns zw to its corresponding PerCorePool.
func ReleaseDeflateWriter(zw *zlib.Writer, level int) {
	if zw == nil {
		return
	}

	_ = zw.Close()
	nLevel := normalizeCompressLevel(level)
	deflateWriterPoolMap[nLevel].Put(zw)
}

// WriteDeflateLevel writes deflated p to w using the given compression level (RFC 1950 §2.2, RFC 1951 §3.1.1).
func WriteDeflateLevel(w io.Writer, p []byte, level int) (int, error) {
	zw := AcquireDeflateWriter(w, level)
	n, err := zw.Write(p)
	ReleaseDeflateWriter(zw, level)

	return n, err
}

// WriteDeflate writes deflated p to w using [CompressDefaultCompression] (RFC 1950, RFC 1951).
func WriteDeflate(w io.Writer, p []byte) (int, error) {
	return WriteDeflateLevel(w, p, CompressDefaultCompression)
}

// AppendDeflateBytesLevel appends deflated src to dst using the given compression level (RFC 1950, RFC 1951).
func AppendDeflateBytesLevel(dst, src []byte, level int) []byte {
	w := &byteSliceWriter{b: dst}
	_, _ = WriteDeflateLevel(w, src, level)
	return w.b
}

// AppendDeflateBytes appends deflated src to dst using [CompressDefaultCompression] (RFC 1950, RFC 1951).
func AppendDeflateBytes(dst, src []byte) []byte {
	return AppendDeflateBytesLevel(dst, src, CompressDefaultCompression)
}

// WriteInflate writes inflated p to w and returns the number of uncompressed bytes (RFC 1950 §2.2, RFC 1951 §3.1.1).
func WriteInflate(w io.Writer, p []byte) (int, error) {
	return WriteInflateLimit(w, p, 0)
}

// WriteInflateLimit decompresses zlib/deflate payload p and writes up to maxBodySize uncompressed bytes to w (RFC 1950 §2.2, RFC 1951 §3.1.1).
func WriteInflateLimit(w io.Writer, p []byte, maxBodySize int) (int, error) {
	r := &byteSliceReader{b: p}

	zr, err := acquireFlateReader(r)
	if err != nil {
		return 0, err
	}
	defer releaseFlateReader(zr)

	n, err := bytesconv.CopyZeroAllocWithLimit(w, zr, maxBodySize)

	nn := int(n)
	if int64(nn) != n {
		return 0, fmt.Errorf("too much data inflated: %d", n)
	}

	return nn, err
}

// AppendInflateBytes appends inflated src to dst and returns the resulting slice (RFC 1950 §2.2, RFC 1951 §3.1.1).
func AppendInflateBytes(dst, src []byte) ([]byte, error) {
	return AppendInflateBytesLimit(dst, src, 0)
}

// AppendInflateBytesLimit appends inflated src to dst up to maxBodySize uncompressed bytes (RFC 1950 §2.2, RFC 1951 §3.1.1).
func AppendInflateBytesLimit(dst, src []byte, maxBodySize int) ([]byte, error) {
	w := &byteSliceWriter{b: dst}
	_, err := WriteInflateLimit(w, src, maxBodySize)
	return w.b, err
}
