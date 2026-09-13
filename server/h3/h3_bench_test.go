// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h3_test

import (
	coreh3 "github.com/lemon4ksan/mach/core/h3"

	"bytes"
	"net/http"
	"testing"

	"github.com/lemon4ksan/mach/qpack"
	"github.com/lemon4ksan/mach/quic/quicvarint"
)

func BenchmarkQPACK_EncodeResponseHeaders(b *testing.B) {
	codec := coreh3.NewQPACKCodec()
	headers := make(http.Header)
	headers.Set("Content-Type", "application/json")
	headers.Set("Server", "Sein/2.0")
	headers.Set("X-Powered-By", "Plan9-AVX2")

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = codec.EncodeResponseHeaders(http.StatusOK, headers, 128)
	}
}

func BenchmarkQPACK_DecodeRequestHeaders(b *testing.B) {
	codec := coreh3.NewQPACKCodec()

	var buf bytes.Buffer

	enc := qpack.NewEncoder(&buf)
	_ = enc.WriteField(qpack.HeaderField{Name: ":method", Value: "GET"})
	_ = enc.WriteField(qpack.HeaderField{Name: ":path", Value: "/api/v1/users"})
	_ = enc.WriteField(qpack.HeaderField{Name: ":scheme", Value: "https"})
	_ = enc.WriteField(qpack.HeaderField{Name: ":authority", Value: "api.example.com"})
	_ = enc.WriteField(qpack.HeaderField{Name: "user-agent", Value: "sein-bench-client"})
	_ = enc.WriteField(qpack.HeaderField{Name: "accept", Value: "application/json"})

	raw := buf.Bytes()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _, _, _, _, _ = codec.DecodeRequestHeaders(raw)
	}
}

func BenchmarkH3_FrameHeaderPack(b *testing.B) {
	var frameHdr [16]byte

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		hdrBytes := quicvarint.Append(frameHdr[:0], coreh3.FrameTypeHeaders)
		_ = quicvarint.Append(hdrBytes, 16384)
	}
}
