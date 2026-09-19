// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h3_test

import (
	"github.com/lemon4ksan/foundation/net/http/status"

	"bytes"
	"testing"

	"github.com/lemon4ksan/foundation/encoding/varint"

	coreheaders "github.com/lemon4ksan/foundation/net/headkit"
	"github.com/lemon4ksan/foundation/net/qpack"
	coreh3 "github.com/lemon4ksan/mach/proto/h3"
)

func BenchmarkQPACK_EncodeResponseHeaders(b *testing.B) {
	codec := coreh3.NewQPACKCodec()
	headers := coreheaders.NewWithCapacity(16)
	headers.Set("Content-Type", "application/json")
	headers.Set("Server", "Sein/2.0")
	headers.Set("X-Powered-By", "Plan9-AVX2")

	b.ReportAllocs()

	for b.Loop() {
		_ = codec.EncodeResponseHeaders(status.StatusOK, headers, 128)
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

	var reqHeaders coreheaders.Headers

	b.ResetTimer()

	for b.Loop() {
		_, _, _, _, _ = codec.DecodeRequestHeaders(raw, &reqHeaders)
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
