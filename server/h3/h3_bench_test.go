// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h3_test

import (
	"testing"

	"github.com/lemon4ksan/foundation/encoding/varint"
	"github.com/lemon4ksan/mach/proto/headkit"
	"github.com/lemon4ksan/mach/proto/http/status"
	"github.com/lemon4ksan/mach/qpack"

	coreh3 "github.com/lemon4ksan/mach/proto/h3"
)

func BenchmarkQPACK_EncodeResponseHeaders(b *testing.B) {
	codec := coreh3.NewQPACKCodec()
	headers := headkit.NewWithCapacity(16)
	headers.Set("Content-Type", "application/json")
	headers.Set("Server", "Sein/2.0")
	headers.Set("X-Powered-By", "Plan9-AVX2")

	b.ReportAllocs()

	for b.Loop() {
		_ = codec.EncodeResponseHeaders(0, status.OK, headers, 128)
	}
}

func BenchmarkQPACK_DecodeRequestHeaders(b *testing.B) {
	codec := coreh3.NewQPACKCodec()

	headers := []qpack.HeaderField{
		{Name: ":method", Value: "GET"},
		{Name: ":path", Value: "/api/v1/users"},
		{Name: ":scheme", Value: "https"},
		{Name: ":authority", Value: "api.example.com"},
		{Name: "user-agent", Value: "sein-bench-client"},
		{Name: "accept", Value: "application/json"},
	}
	raw := qpack.NewEncoderWithDefaults(nil).EncodeHeaderList(0, headers, nil)

	b.ReportAllocs()

	var reqHeaders headkit.Headers

	b.ResetTimer()

	for b.Loop() {
		_, _, _, _, _ = codec.DecodeRequestHeaders(0, raw, &reqHeaders)
	}
}

func BenchmarkH3_FrameHeaderPack(b *testing.B) {
	var frameHdr [16]byte

	b.ReportAllocs()

	for b.Loop() {
		hdrBytes := varint.Append(frameHdr[:0], coreh3.FrameTypeHeaders)
		_ = varint.Append(hdrBytes, 16384)
	}
}
