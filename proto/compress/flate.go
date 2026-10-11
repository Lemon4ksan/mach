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

	"github.com/lemon4ksan/mach/internal/stackless"
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

func resetFlateReader(zr io.ReadCloser, r io.Reader) error {
	zrr, ok := zr.(zlib.Resetter)
	if !ok {
		// sanity check. should only be called with a zlib.Reader
		panic("BUG: zlib.Reader doesn't implement zlib.Resetter???")
	}

	return zrr.Reset(r, nil)
}

// AcquireStacklessDeflateWriter acquires a zero-alloc pooled Deflate writer targeting w at the given compression level (RFC 1950 §2.2, RFC 1951 §3.1.1).
func AcquireStacklessDeflateWriter(w io.Writer, level int) stackless.Writer {
	return acquireRealDeflateWriter(w, level)
}

// ReleaseStacklessDeflateWriter closes sw and returns it to the corresponding deflate pool (RFC 1950 §2.2, RFC 1951 §3.1.1).
func ReleaseStacklessDeflateWriter(sw stackless.Writer, level int) {
	if zw, ok := sw.(*zlib.Writer); ok {
		releaseRealDeflateWriter(zw, level)
	} else if sw != nil {
		_ = sw.Close()
	}
}

func acquireRealDeflateWriter(w io.Writer, level int) *zlib.Writer {
	nLevel := normalizeCompressLevel(level)
	zw := deflateWriterPoolMap[nLevel].Get()
	zw.Reset(w)
	return zw
}

func releaseRealDeflateWriter(zw *zlib.Writer, level int) {
	if zw == nil {
		return
	}
	_ = zw.Close()
	nLevel := normalizeCompressLevel(level)
	deflateWriterPoolMap[nLevel].Put(zw)
}

// AppendDeflateBytesLevel appends deflated src to dst using the given
// compression level and returns the resulting slice (RFC 1950 §2.2, RFC 1951 §3.1.1).
//
// Supported compression levels are:
//
//   - [CompressNoCompression]
//   - [CompressBestSpeed]
//   - [CompressBestCompression]
//   - [CompressDefaultCompression]
//   - [CompressHuffmanOnly]
func AppendDeflateBytesLevel(dst, src []byte, level int) []byte {
	w := &byteSliceWriter{b: dst}
	_, _ = WriteDeflateLevel(w, src, level)

	return w.b
}

// WriteDeflateLevel writes deflated p to w using the given compression level
// and returns the number of compressed bytes written to w (RFC 1950 §2.2, RFC 1951 §3.1.1).
//
// Supported compression levels are:
//
//   - [CompressNoCompression]
//   - [CompressBestSpeed]
//   - [CompressBestCompression]
//   - [CompressDefaultCompression]
//   - [CompressHuffmanOnly]
func WriteDeflateLevel(w io.Writer, p []byte, level int) (int, error) {
	zw := acquireRealDeflateWriter(w, level)
	n, err := zw.Write(p)
	releaseRealDeflateWriter(zw, level)
	return n, err
}

// WriteDeflate writes deflated p to w using [CompressDefaultCompression] and returns the number of compressed
// bytes written to w (RFC 1950 §2.2, RFC 1951 §3.1.1).
func WriteDeflate(w io.Writer, p []byte) (int, error) {
	return WriteDeflateLevel(w, p, CompressDefaultCompression)
}

// AppendDeflateBytes appends deflated src to dst using [CompressDefaultCompression] and returns the resulting slice (RFC 1950 §2.2, RFC 1951 §3.1.1).
func AppendDeflateBytes(dst, src []byte) []byte {
	return AppendDeflateBytesLevel(dst, src, CompressDefaultCompression)
}

// WriteInflate writes inflated p to w and returns the number of uncompressed
// bytes written to w (RFC 1950 §2.2, RFC 1951 §3.1.1).
func WriteInflate(w io.Writer, p []byte) (int, error) {
	return WriteInflateLimit(w, p, 0)
}

// WriteInflateLimit decompresses zlib/deflate payload p and writes up to maxBodySize uncompressed bytes to w (RFC 1950 §2.2, RFC 1951 §3.1.1).
//
// RFC 1950 §2.2 defines the 2-byte header (CMF/FLG with FCHECK verification) and 4-byte Adler-32
// checksum trailer, wrapping RFC 1951 §3.1.1/§3.2 DEFLATE compressed data blocks.
// If maxBodySize is 0 or negative, uncompressed size is unlimited. If decompression produces
// more than maxBodySize bytes, decompression halts and an error is returned to prevent decompression bombs.
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

// AppendInflateBytesLimit appends inflated src to dst up to maxBodySize uncompressed bytes and returns the resulting slice (RFC 1950 §2.2, RFC 1951 §3.1.1).
//
// RFC 1950 §2.2 defines the 2-byte header (CMF/FLG) and Adler-32 trailer wrapping RFC 1951 §3.1.1 DEFLATE.
// If maxBodySize is 0 or negative, uncompressed size is unlimited. If decompression produces
// more than maxBodySize bytes, decompression halts and an error is returned to prevent decompression bombs.
func AppendInflateBytesLimit(dst, src []byte, maxBodySize int) ([]byte, error) {
	w := &byteSliceWriter{b: dst}
	_, err := WriteInflateLimit(w, src, maxBodySize)
	return w.b, err
}
