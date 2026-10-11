// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package compress_test

import (
	"io"
	"testing"

	"github.com/lemon4ksan/mach/proto/codec/compress"
)

func TestAllocations(t *testing.T) {
	payload := []byte("Benchmark and allocation testing payload for modern compress codecs!")

	// Prime the pools
	zw0 := compress.AcquireGzipWriter(io.Discard, compress.CompressDefaultCompression)
	compress.ReleaseGzipWriter(zw0, compress.CompressDefaultCompression)
	zw1 := compress.AcquireDeflateWriter(io.Discard, compress.CompressDefaultCompression)
	compress.ReleaseDeflateWriter(zw1, compress.CompressDefaultCompression)

	// 1. Gzip acquire/release and WriteGzipLevel
	allocsGz := testing.AllocsPerRun(1000, func() {
		zw := compress.AcquireGzipWriter(io.Discard, compress.CompressDefaultCompression)
		_, _ = zw.Write(payload)
		compress.ReleaseGzipWriter(zw, compress.CompressDefaultCompression)
	})
	if allocsGz > 0 {
		t.Errorf("got %v allocs for AcquireGzipWriter/ReleaseGzipWriter, want 0", allocsGz)
	}

	// 2. Deflate acquire/release and WriteDeflateLevel
	allocsDeflate := testing.AllocsPerRun(1000, func() {
		zw := compress.AcquireDeflateWriter(io.Discard, compress.CompressDefaultCompression)
		_, _ = zw.Write(payload)
		compress.ReleaseDeflateWriter(zw, compress.CompressDefaultCompression)
	})
	if allocsDeflate > 0 {
		t.Errorf("got %v allocs for AcquireDeflateWriter/ReleaseDeflateWriter, want 0", allocsDeflate)
	}
}
