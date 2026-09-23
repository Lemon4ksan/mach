// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package compress

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"io"
	"sync"

	"github.com/lemon4ksan/foundation/codec/compress"
	"github.com/lemon4ksan/foundation/silicon/bytesconv"

	"github.com/lemon4ksan/mach/proto/http/stackless"
)

func acquireFlateReader(r io.Reader) (io.ReadCloser, error) {
	v := flateReaderPool.Get()
	if v == nil {
		zr, err := zlib.NewReader(r)
		if err != nil {
			return nil, err
		}

		return zr, nil
	}

	zr := v.(io.ReadCloser) //nolint:forcetypeassert
	if err := resetFlateReader(zr, r); err != nil {
		return nil, err
	}

	return zr, nil
}

func releaseFlateReader(zr io.ReadCloser) {
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

var flateReaderPool sync.Pool

// AcquireStacklessDeflateWriter acquires an asynchronous stackless Deflate writer targeting w at the given compression level (RFC 1951).
//
// It offloads deflate compression work to a dedicated worker pool, avoiding stack expansion on the caller goroutine.
// Callers must return the writer to the pool with ReleaseStacklessDeflateWriter when completed.
// Concurrency: Acquired writer is single-goroutine; pool acquisition is thread-safe.
func AcquireStacklessDeflateWriter(w io.Writer, level int) stackless.Writer {
	nLevel := normalizeCompressLevel(level)
	p := stacklessDeflateWriterPoolMap[nLevel]

	v := p.Get()
	if v == nil {
		return stackless.NewWriter(w, func(w io.Writer) stackless.Writer {
			return acquireRealDeflateWriter(w, level)
		})
	}

	sw := v.(stackless.Writer) //nolint:forcetypeassert
	sw.Reset(w)

	return sw
}

// ReleaseStacklessDeflateWriter closes sw and returns it to the corresponding deflate pool (RFC 1951).
//
// Callers must not use sw after calling ReleaseStacklessDeflateWriter.
// Concurrency: Thread-safe.
func ReleaseStacklessDeflateWriter(sw stackless.Writer, level int) {
	_ = sw.Close()

	nLevel := normalizeCompressLevel(level)
	p := stacklessDeflateWriterPoolMap[nLevel]
	p.Put(sw)
}

func acquireRealDeflateWriter(w io.Writer, level int) *zlib.Writer {
	nLevel := normalizeCompressLevel(level)
	p := realDeflateWriterPoolMap[nLevel]

	v := p.Get()
	if v == nil {
		zw, err := zlib.NewWriterLevel(w, level)
		if err != nil {
			// zlib.NewWriterLevel only errors for invalid
			// compression levels. Clamp it to be min or max.
			if level < zlib.HuffmanOnly {
				level = zlib.HuffmanOnly
			} else {
				level = zlib.BestCompression
			}

			zw, _ = zlib.NewWriterLevel(w, level)
		}

		return zw
	}

	zw := v.(*zlib.Writer) //nolint:forcetypeassert
	zw.Reset(w)

	return zw
}

func releaseRealDeflateWriter(zw *zlib.Writer, level int) {
	_ = zw.Close()

	nLevel := normalizeCompressLevel(level)
	p := realDeflateWriterPoolMap[nLevel]
	p.Put(zw)
}

var (
	stacklessDeflateWriterPoolMap = newCompressWriterPoolMap()
	realDeflateWriterPoolMap      = newCompressWriterPoolMap()
)

// AppendDeflateBytesLevel appends deflated src to dst using the given
// compression level and returns the resulting dst (RFC 1951).
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
func AppendDeflateBytesLevel(dst, src []byte, level int) []byte {
	w := &byteSliceWriter{b: dst}
	_, _ = WriteDeflateLevel(w, src, level)

	return w.b
}

// WriteDeflateLevel writes deflated p to w using the given compression level
// and returns the number of compressed bytes written to w (RFC 1951).
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
func WriteDeflateLevel(w io.Writer, p []byte, level int) (int, error) {
	switch w.(type) {
	case *byteSliceWriter,
		*bytes.Buffer,
		*bytesconv.ByteBuffer:
		// These writers don't block, so we can just use stacklessWriteDeflate
		ctx := &compressCtx{
			w:     w,
			p:     p,
			level: level,
		}
		stacklessWriteDeflate(ctx)

		return len(p), nil

	default:
		zw := AcquireStacklessDeflateWriter(w, level)
		n, err := zw.Write(p)
		ReleaseStacklessDeflateWriter(zw, level)

		return n, err
	}
}

var (
	stacklessWriteDeflateOnce sync.Once
	stacklessWriteDeflateFunc func(ctx any) bool
)

func stacklessWriteDeflate(ctx any) {
	stacklessWriteDeflateOnce.Do(func() {
		stacklessWriteDeflateFunc = stackless.NewFunc(nonblockingWriteDeflate)
	})
	stacklessWriteDeflateFunc(ctx)
}

func nonblockingWriteDeflate(ctxv any) {
	ctx := ctxv.(*compressCtx) //nolint:forcetypeassert
	zw := acquireRealDeflateWriter(ctx.w, ctx.level)

	_, _ = zw.Write(ctx.p)

	releaseRealDeflateWriter(zw, ctx.level)
}

// WriteDeflate writes deflated p to w using default compression level and returns the number of compressed
// bytes written to w (RFC 1951).
// Concurrency: Thread-safe.
func WriteDeflate(w io.Writer, p []byte) (int, error) {
	return WriteDeflateLevel(w, p, CompressDefaultCompression)
}

// AppendDeflateBytes appends deflated src to dst using default compression level and returns the resulting dst (RFC 1951).
// Concurrency: Thread-safe.
func AppendDeflateBytes(dst, src []byte) []byte {
	return AppendDeflateBytesLevel(dst, src, CompressDefaultCompression)
}

// WriteInflate writes inflated p to w and returns the number of uncompressed
// bytes written to w (RFC 1951).
// Concurrency: Thread-safe.
func WriteInflate(w io.Writer, p []byte) (int, error) {
	return WriteInflateLimit(w, p, 0)
}

// WriteInflateLimit decompresses zlib/deflate payload p and writes up to maxBodySize uncompressed bytes to w (RFC 1950, RFC 1951).
//
// If maxBodySize is 0 or negative, uncompressed size is unlimited. If decompression produces
// more than maxBodySize bytes, decompression halts and an error is returned to prevent decompression bombs.
// Concurrency: Thread-safe; utilizes pooled zlib readers.
func WriteInflateLimit(w io.Writer, p []byte, maxBodySize int) (int, error) {
	r := &byteSliceReader{b: p}

	zr, err := acquireFlateReader(r)
	if err != nil {
		return 0, err
	}

	n, err := bytesconv.CopyZeroAllocWithLimit(w, zr, maxBodySize)
	releaseFlateReader(zr)

	nn := int(n)
	if int64(nn) != n {
		return 0, fmt.Errorf("too much data inflated: %d", n)
	}

	return nn, err
}

// AppendInflateBytes appends inflated src to dst and returns the resulting dst (RFC 1951).
// Concurrency: Thread-safe.
func AppendInflateBytes(dst, src []byte) ([]byte, error) {
	return compress.Inflate(src, dst)
}
