// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package compress

import (
	"bytes"
	"fmt"
	"io"
	"sync"

	"github.com/lemon4ksan/foundation/codec/compress"
	"github.com/lemon4ksan/foundation/codec/compress/gzip"
	"github.com/lemon4ksan/foundation/silicon/bytesconv"

	"github.com/lemon4ksan/mach/proto/http/stackless"
)

func acquireGzipReader(r io.Reader) (*gzip.Reader, error) {
	return compress.AcquireGzipReader(r)
}

func releaseGzipReader(zr *gzip.Reader) {
	compress.ReleaseGzipReader(zr)
}

// AcquireStacklessGzipWriter acquires an asynchronous stackless Gzip writer targeting w at the given compression level (RFC 1952).
//
// It shields calling goroutines from large stack allocations by offloading compression work
// to a worker pool managed by stackless.Writer. Callers must release the returned writer
// using ReleaseStacklessGzipWriter when finished.
// Concurrency: Acquired writer is single-goroutine; pool acquisition is thread-safe.
func AcquireStacklessGzipWriter(w io.Writer, level int) stackless.Writer {
	nLevel := normalizeCompressLevel(level)
	p := stacklessGzipWriterPoolMap[nLevel]

	v := p.Get()
	if v == nil {
		return stackless.NewWriter(w, func(w io.Writer) stackless.Writer {
			return acquireRealGzipWriter(w, level)
		})
	}

	sw := v.(stackless.Writer) //nolint:forcetypeassert
	sw.Reset(w)

	return sw
}

// ReleaseStacklessGzipWriter closes sw and returns it to the corresponding compression level pool (RFC 1952).
//
// Callers must not use sw after calling ReleaseStacklessGzipWriter.
// Concurrency: Thread-safe.
func ReleaseStacklessGzipWriter(sw stackless.Writer, level int) {
	_ = sw.Close()

	nLevel := normalizeCompressLevel(level)
	p := stacklessGzipWriterPoolMap[nLevel]
	p.Put(sw)
}

func acquireRealGzipWriter(w io.Writer, level int) *gzip.Writer {
	nLevel := normalizeCompressLevel(level)
	p := realGzipWriterPoolMap[nLevel]

	v := p.Get()
	if v == nil {
		zw, err := gzip.NewWriterLevel(w, level)
		if err != nil {
			// gzip.NewWriterLevel only errors for invalid
			// compression levels. Clamp it to be min or max.
			if level < gzip.HuffmanOnly {
				level = gzip.HuffmanOnly
			} else {
				level = gzip.BestCompression
			}

			zw, _ = gzip.NewWriterLevel(w, level)
		}

		return zw
	}

	zw := v.(*gzip.Writer) //nolint:forcetypeassert
	zw.Reset(w)

	return zw
}

func releaseRealGzipWriter(zw *gzip.Writer, level int) {
	_ = zw.Close()

	nLevel := normalizeCompressLevel(level)
	p := realGzipWriterPoolMap[nLevel]
	p.Put(zw)
}

var (
	stacklessGzipWriterPoolMap = newCompressWriterPoolMap()
	realGzipWriterPoolMap      = newCompressWriterPoolMap()
)

// AppendGzipBytesLevel appends gzipped src to dst using the given
// compression level and returns the resulting dst (RFC 1952).
//
// Supported compression levels are:
//
//   - CompressNoCompression
//   - CompressBestSpeed
//   - CompressBestCompression
//   - CompressDefaultCompression
//   - CompressHuffmanOnly
//
// Concurrency: Thread-safe.
func AppendGzipBytesLevel(dst, src []byte, level int) []byte {
	w := &byteSliceWriter{b: dst}
	_, _ = WriteGzipLevel(w, src, level)

	return w.b
}

// WriteGzipLevel writes gzipped p to w using the given compression level
// and returns the number of compressed bytes written to w (RFC 1952).
//
// Supported compression levels are:
//
//   - CompressNoCompression
//   - CompressBestSpeed
//   - CompressBestCompression
//   - CompressDefaultCompression
//   - CompressHuffmanOnly
//
// Concurrency: Thread-safe.
func WriteGzipLevel(w io.Writer, p []byte, level int) (int, error) {
	switch w.(type) {
	case *byteSliceWriter,
		*bytes.Buffer,
		*bytesconv.ByteBuffer:
		// These writers don't block, so we can just use stacklessWriteGzip
		ctx := &compressCtx{
			w:     w,
			p:     p,
			level: level,
		}
		stacklessWriteGzip(ctx)

		return len(p), nil

	default:
		zw := AcquireStacklessGzipWriter(w, level)
		n, err := zw.Write(p)
		ReleaseStacklessGzipWriter(zw, level)

		return n, err
	}
}

var (
	stacklessWriteGzipOnce sync.Once
	stacklessWriteGzipFunc func(ctx any) bool
)

func stacklessWriteGzip(ctx any) {
	stacklessWriteGzipOnce.Do(func() {
		stacklessWriteGzipFunc = stackless.NewFunc(nonblockingWriteGzip)
	})
	stacklessWriteGzipFunc(ctx)
}

func nonblockingWriteGzip(ctxv any) {
	ctx := ctxv.(*compressCtx) //nolint:forcetypeassert
	zw := acquireRealGzipWriter(ctx.w, ctx.level)

	_, _ = zw.Write(ctx.p)

	releaseRealGzipWriter(zw, ctx.level)
}

// WriteGzip writes gzipped p to w using default compression level and returns the number of compressed
// bytes written to w (RFC 1952).
// Concurrency: Thread-safe.
func WriteGzip(w io.Writer, p []byte) (int, error) {
	return WriteGzipLevel(w, p, CompressDefaultCompression)
}

// AppendGzipBytes appends gzipped src to dst using default compression and returns the resulting dst (RFC 1952).
// Concurrency: Thread-safe.
func AppendGzipBytes(dst, src []byte) []byte {
	return AppendGzipBytesLevel(dst, src, CompressDefaultCompression)
}

// WriteGunzip writes ungzipped p to w and returns the number of uncompressed
// bytes written to w (RFC 1952).
// Concurrency: Thread-safe.
func WriteGunzip(w io.Writer, p []byte) (int, error) {
	return WriteGunzipLimit(w, p, 0)
}

// WriteGunzipLimit decompresses gzipped payload p and writes up to maxBodySize uncompressed bytes to w (RFC 1952).
//
// If maxBodySize is 0 or negative, uncompressed size is unlimited. If decompression produces
// more than maxBodySize bytes, decompression halts and an error is returned to prevent decompression bombs.
// Concurrency: Thread-safe; utilizes pooled gzip readers.
func WriteGunzipLimit(w io.Writer, p []byte, maxBodySize int) (int, error) {
	r := &byteSliceReader{b: p}

	zr, err := acquireGzipReader(r)
	if err != nil {
		return 0, err
	}

	n, err := bytesconv.CopyZeroAllocWithLimit(w, zr, maxBodySize)
	releaseGzipReader(zr)

	nn := int(n)
	if int64(nn) != n {
		return 0, fmt.Errorf("too much data gunzipped: %d", n)
	}

	return nn, err
}

// AppendGunzipBytes appends gunzipped src to dst and returns the resulting dst (RFC 1952).
// Concurrency: Thread-safe.
func AppendGunzipBytes(dst, src []byte) ([]byte, error) {
	return compress.Gunzip(src, dst)
}
