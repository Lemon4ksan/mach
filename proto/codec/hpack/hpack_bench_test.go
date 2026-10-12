// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package hpack_test

import (
	"testing"

	"github.com/lemon4ksan/mach/proto/codec/hpack"
)

func BenchmarkHPACK_Encode_RequestHeaders(b *testing.B) {
	hp := hpack.AcquireHPACK()
	defer hpack.ReleaseHPACK(hp)

	hf := hpack.AcquireHeaderField()
	defer hpack.ReleaseHeaderField(hf)

	headers := []struct{ k, v string }{
		{":method", "GET"},
		{":path", "/api/v1/stream"},
		{":scheme", "https"},
		{":authority", "api.example.com"},
		{"accept", "application/json"},
		{"accept-encoding", "gzip, deflate"},
		{"user-agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64)"},
	}

	buf := make([]byte, 0, 512)
	b.ReportAllocs()

	for b.Loop() {
		buf = buf[:0]
		for _, h := range headers {
			hf.Reset()
			hf.Set(h.k, h.v)
			buf = hp.AppendHeader(buf, hf, false)
		}
	}
}

func BenchmarkHPACK_Decode_RequestHeaders(b *testing.B) {
	enc := hpack.AcquireHPACK()
	defer hpack.ReleaseHPACK(enc)

	hf := hpack.AcquireHeaderField()
	defer hpack.ReleaseHeaderField(hf)

	headers := []struct{ k, v string }{
		{":method", "GET"},
		{":path", "/api/v1/stream"},
		{":scheme", "https"},
		{":authority", "api.example.com"},
		{"accept", "application/json"},
		{"accept-encoding", "gzip, deflate"},
		{"user-agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64)"},
	}

	var encoded []byte
	for _, h := range headers {
		hf.Reset()
		hf.Set(h.k, h.v)
		encoded = enc.AppendHeader(encoded, hf, true)
	}

	dec := hpack.AcquireHPACK()
	defer hpack.ReleaseHPACK(dec)

	b.ReportAllocs()

	for b.Loop() {
		dec.BeginHeaderBlock()
		bBytes := encoded
		for len(bBytes) > 0 {
			hf.Reset()
			rem, err := dec.Next(hf, bBytes)
			if err != nil {
				b.Fatal(err)
			}
			bBytes = rem
		}
	}
}

func BenchmarkHPACK_StaticTableLookup(b *testing.B) {
	hp := hpack.AcquireHPACK()
	defer hpack.ReleaseHPACK(hp)

	hf := hpack.AcquireHeaderField()
	defer hpack.ReleaseHeaderField(hf)

	buf := make([]byte, 0, 128)
	b.ReportAllocs()

	for b.Loop() {
		buf = buf[:0]
		hf.Reset()
		hf.Set(":method", "GET")
		buf = hp.AppendHeader(buf, hf, false)

		hf.Reset()
		hf.Set(":status", "200")
		buf = hp.AppendHeader(buf, hf, false)

		hf.Reset()
		hf.Set("accept-encoding", "gzip, deflate")
		buf = hp.AppendHeader(buf, hf, false)
	}
}
