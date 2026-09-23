// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package compress

import (
	"io"
	"io/fs"
	"sync"

	"github.com/lemon4ksan/foundation/codec/compress/flate"
	"github.com/lemon4ksan/foundation/silicon/bytesconv"
)

// Supported compression levels (RFC 1951).
const (
	// CompressNoCompression disables compression (RFC 1951).
	CompressNoCompression = flate.NoCompression
	// CompressBestSpeed uses the fastest compression speed (RFC 1951).
	CompressBestSpeed = flate.BestSpeed
	// CompressBestCompression uses the maximum compression ratio (RFC 1951).
	CompressBestCompression = flate.BestCompression
	// CompressDefaultCompression uses standard balanced compression (RFC 1951).
	CompressDefaultCompression = 6 // flate.DefaultCompression
	// CompressHuffmanOnly restricts deflate compression to Huffman coding only (RFC 1951).
	CompressHuffmanOnly = -2 // flate.HuffmanOnly
)

type compressCtx struct {
	w     io.Writer
	p     []byte
	level int
}

type byteSliceWriter struct {
	b []byte
}

func (w *byteSliceWriter) Write(p []byte) (int, error) {
	w.b = append(w.b, p...)
	return len(p), nil
}

func (w *byteSliceWriter) WriteString(s string) (int, error) {
	w.b = append(w.b, s...)
	return len(s), nil
}

type byteSliceReader struct {
	b []byte
}

func (r *byteSliceReader) Read(p []byte) (int, error) {
	if len(r.b) == 0 {
		return 0, io.EOF
	}

	n := copy(p, r.b)
	r.b = r.b[n:]

	return n, nil
}

func (r *byteSliceReader) ReadByte() (byte, error) {
	if len(r.b) == 0 {
		return 0, io.EOF
	}

	n := r.b[0]
	r.b = r.b[1:]

	return n, nil
}

func newCompressWriterPoolMap() []*sync.Pool {
	// Initialize pools for all the compression levels defined
	// in https://pkg.go.dev/compress/flate#pkg-constants .
	// Compression levels are normalized with normalizeCompressLevel,
	// so the fit [0..11].
	m := make([]*sync.Pool, 0, 12)
	for range 12 {
		m = append(m, &sync.Pool{})
	}

	return m
}

//nolint:unused
func isFileCompressible(f fs.File, minCompressRatio float64) bool {
	// Try compressing the first 4kb of the file
	// and see if it can be compressed by more than
	// the given minCompressRatio.
	b := bytesconv.AcquireByteBuffer()
	zw := AcquireStacklessGzipWriter(b, CompressDefaultCompression)
	lr := &io.LimitedReader{
		R: f,
		N: 4096,
	}
	_, err := bytesconv.CopyZeroAlloc(zw, lr)
	ReleaseStacklessGzipWriter(zw, CompressDefaultCompression)

	seeker, ok := f.(io.Seeker)
	if !ok {
		return false
	}

	_, _ = seeker.Seek(0, io.SeekStart)

	if err != nil {
		return false
	}

	n := 4096 - lr.N
	zn := len(b.B)
	bytesconv.ReleaseByteBuffer(b)

	return float64(zn) < float64(n)*minCompressRatio
}

// normalizes compression level into [0..11], so it could be used as an index
// in *PoolMap.
func normalizeCompressLevel(level int) int {
	// -2 is the lowest compression level - CompressHuffmanOnly
	// 9 is the highest compression level - CompressBestCompression
	if level < -2 || level > 9 {
		level = CompressDefaultCompression
	}

	return level + 2
}
