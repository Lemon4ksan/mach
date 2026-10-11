// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package compress_test

import (
	"bytes"
	"testing"

	"github.com/lemon4ksan/mach/proto/codec/compress"
)

var benchmarkData = bytes.Repeat(
	[]byte("Mach high performance HTTP engine compression benchmark payload data 1234567890! "),
	100,
)

func BenchmarkCompress_Gzip(b *testing.B) {
	for b.Loop() {
		_ = compress.AppendGzipBytes(nil, benchmarkData)
	}
}

func BenchmarkCompress_Brotli(b *testing.B) {
	for b.Loop() {
		_ = compress.AppendBrotliBytes(nil, benchmarkData)
	}
}

func BenchmarkCompress_Zstd(b *testing.B) {
	for b.Loop() {
		_ = compress.AppendZstdBytes(nil, benchmarkData)
	}
}

func BenchmarkCompress_Deflate(b *testing.B) {
	for b.Loop() {
		_ = compress.AppendDeflateBytes(nil, benchmarkData)
	}
}

func BenchmarkDecompress_Gzip(b *testing.B) {
	compressed := compress.AppendGzipBytes(nil, benchmarkData)
	for b.Loop() {
		_, _ = compress.AppendGunzipBytes(nil, compressed)
	}
}

func BenchmarkDecompress_Brotli(b *testing.B) {
	compressed := compress.AppendBrotliBytes(nil, benchmarkData)
	for b.Loop() {
		_, _ = compress.AppendUnbrotliBytes(nil, compressed)
	}
}

func BenchmarkDecompress_Zstd(b *testing.B) {
	compressed := compress.AppendZstdBytes(nil, benchmarkData)
	for b.Loop() {
		_, _ = compress.AppendUnzstdBytes(nil, compressed)
	}
}

func BenchmarkDecompress_Deflate(b *testing.B) {
	compressed := compress.AppendDeflateBytes(nil, benchmarkData)
	for b.Loop() {
		_, _ = compress.AppendInflateBytes(nil, compressed)
	}
}
