// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package hpack_test

import (
	"testing"

	"github.com/lemon4ksan/mach/hpack"
)

func FuzzHPACKDecode(f *testing.F) {
	f.Add([]byte{0x82}) // :method: GET
	f.Add(
		[]byte{
			0x82,
			0x86,
			0x84,
			0x41,
			0x0f,
			0x77,
			0x77,
			0x77,
			0x2e,
			0x65,
			0x78,
			0x61,
			0x6d,
			0x70,
			0x6c,
			0x65,
			0x2e,
			0x63,
			0x6f,
			0x6d,
		},
	)
	f.Add([]byte{0x00})
	f.Add(
		[]byte{
			0x40,
			0x0a,
			'c',
			'u',
			's',
			't',
			'o',
			'm',
			'-',
			'k',
			'e',
			'y',
			0x0d,
			'c',
			'u',
			's',
			't',
			'o',
			'm',
			'-',
			'h',
			'e',
			'a',
			'd',
			'e',
			'r',
		},
	)

	f.Fuzz(func(t *testing.T, data []byte) {
		hp := hpack.AcquireHPACK()
		defer hpack.ReleaseHPACK(hp)

		var fields []*hpack.HeaderField
		fields, err := hp.DecodeAll(fields, data)
		for _, hf := range fields {
			hpack.ReleaseHeaderField(hf)
		}
		_ = err
	})
}

func FuzzHPACKEncodeDecodeRoundtrip(f *testing.F) {
	f.Add("custom-key", "custom-val")
	f.Add(":path", "/index.html")
	f.Add("user-agent", "Mozilla/5.0")
	f.Add("content-length", "1024")

	f.Fuzz(func(t *testing.T, key, val string) {
		if len(key) == 0 || len(key) > 512 || len(val) > 2048 {
			return
		}
		hpEnc := hpack.AcquireHPACK()
		defer hpack.ReleaseHPACK(hpEnc)

		hf := hpack.AcquireHeaderField()
		defer hpack.ReleaseHeaderField(hf)

		hf.Set(key, val)
		wantKey := hf.Key()
		wantVal := hf.Value()
		encoded := hpEnc.AppendHeader(nil, hf, true)

		hpDec := hpack.AcquireHPACK()
		defer hpack.ReleaseHPACK(hpDec)

		var fields []*hpack.HeaderField
		fields, err := hpDec.DecodeAll(fields, encoded)
		defer func() {
			for _, f := range fields {
				hpack.ReleaseHeaderField(f)
			}
		}()

		if err != nil {
			t.Fatalf("decode failed on encoded header: %v", err)
		}
		if len(fields) != 1 {
			t.Fatalf("expected 1 decoded header field, got %d", len(fields))
		}
		if fields[0].Key() != wantKey || fields[0].Value() != wantVal {
			t.Fatalf(
				"roundtrip mismatch: got %q: %q, want %q: %q",
				fields[0].Key(),
				fields[0].Value(),
				wantKey,
				wantVal,
			)
		}
	})
}
