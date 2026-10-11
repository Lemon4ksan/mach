// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package compress_test

import (
	"io"
	"testing"

	"github.com/lemon4ksan/mach/proto/compress"
)

func TestAllocations(t *testing.T) {
	payload := []byte("Hot-path payload for zero-allocation compression testing.")

	// Prime the pools
	zw0 := compress.AcquireStacklessGzipWriter(io.Discard, compress.CompressDefaultCompression)
	compress.ReleaseStacklessGzipWriter(zw0, compress.CompressDefaultCompression)
	zw1 := compress.AcquireStacklessDeflateWriter(io.Discard, compress.CompressDefaultCompression)
	compress.ReleaseStacklessDeflateWriter(zw1, compress.CompressDefaultCompression)

	// 1. Gzip acquire, write, release
	allocsGz := testing.AllocsPerRun(1000, func() {
		zw := compress.AcquireStacklessGzipWriter(io.Discard, compress.CompressDefaultCompression)
		_, _ = zw.Write(payload)
		compress.ReleaseStacklessGzipWriter(zw, compress.CompressDefaultCompression)
	})
	if allocsGz > 0 {
		t.Fatalf("expected 0 allocations in AcquireStacklessGzipWriter hot path, got %v", allocsGz)
	}

	// 2. Deflate acquire, write, release
	allocsDeflate := testing.AllocsPerRun(1000, func() {
		zw := compress.AcquireStacklessDeflateWriter(io.Discard, compress.CompressDefaultCompression)
		_, _ = zw.Write(payload)
		compress.ReleaseStacklessDeflateWriter(zw, compress.CompressDefaultCompression)
	})
	if allocsDeflate > 0 {
		t.Fatalf("expected 0 allocations in AcquireStacklessDeflateWriter hot path, got %v", allocsDeflate)
	}
}
