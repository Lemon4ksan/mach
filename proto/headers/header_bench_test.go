// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package headers

import (
	"testing"
)

func BenchmarkParseHeaderBlockSWAR(b *testing.B) {
	rawHeaders := []byte(
		"Host: example.com\r\nUser-Agent: mach-benchmark-tool/1.0\r\nAccept: application/json\r\nConnection: keep-alive\r\nX-Custom-Header: some-value\r\n",
	)

	h := NewWithCapacity(16)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		h.ParseHeaderBlockSWAR(rawHeaders)
	}
}

func BenchmarkHeaderGet_PerfectHash(b *testing.B) {
	rawHeaders := []byte(
		"Host: example.com\r\nUser-Agent: mach-benchmark-tool/1.0\r\nAccept: application/json\r\nConnection: keep-alive\r\nX-Custom-Header: some-value\r\n",
	)

	h := NewWithCapacity(16)
	h.ParseHeaderBlockSWAR(rawHeaders)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = h.Get("Host")
	}
}

func BenchmarkHeaderGet_PackedScan(b *testing.B) {
	rawHeaders := []byte(
		"Host: example.com\r\nUser-Agent: mach-benchmark-tool/1.0\r\nAccept: application/json\r\nConnection: keep-alive\r\nX-Custom-Header: some-value\r\n",
	)

	h := NewWithCapacity(16)
	h.ParseHeaderBlockSWAR(rawHeaders)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = h.Get("X-Custom-Header")
	}
}
