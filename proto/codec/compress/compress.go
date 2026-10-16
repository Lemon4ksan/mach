// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package compress

import (
	"io"
	"io/fs"

	"github.com/lemon4ksan/foundation/silicon/bytesconv"
	"github.com/lemon4ksan/seal/codec/compress/flate"
)

// Supported Deflate/Gzip compression levels (RFC 1951 §3.2).
const (
	CompressNoCompression      = flate.NoCompression
	CompressBestSpeed          = flate.BestSpeed
	CompressBestCompression    = flate.BestCompression
	CompressDefaultCompression = 6
	CompressHuffmanOnly        = -2
)

// Supported Brotli compression levels (RFC 7932 §9.1, §9.2).
const (
	CompressBrotliNoCompression      = 0
	CompressBrotliBestSpeed          = 1
	CompressBrotliBestCompression    = 11
	CompressBrotliDefaultCompression = 4
)

// Supported Zstandard compression levels (RFC 8878).
const (
	CompressZstdSpeedNotSet = iota
	CompressZstdBestSpeed
	CompressZstdDefault
	CompressZstdSpeedBetter
	CompressZstdBestCompression
)

type byteSliceWriter struct {
	b []byte
}

func (w *byteSliceWriter) Write(p []byte) (int, error) {
	w.b = append(w.b, p...)
	return len(p), nil
}

// normalizeCompressLevel normalizes compression level into [0..11] to index into PerCorePool tables.
func normalizeCompressLevel(level int) int {
	if level < -2 || level > 9 {
		level = CompressDefaultCompression
	}

	return level + 2
}

// IsFileCompressible checks if the file has high compression ratio on its initial block.
func IsFileCompressible(f fs.File, minCompressRatio float64) bool {
	b := bytesconv.AcquireByteBuffer()
	defer bytesconv.ReleaseByteBuffer(b)

	zw := AcquireGzipWriter(b, CompressDefaultCompression)
	lr := &io.LimitedReader{
		R: f,
		N: 4096,
	}
	_, err := bytesconv.CopyZeroAlloc(zw, lr)
	ReleaseGzipWriter(zw, CompressDefaultCompression)

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

	return float64(zn) < float64(n)*minCompressRatio
}
