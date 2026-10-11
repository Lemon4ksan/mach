// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package zerocopy_test

import (
	"testing"

	"github.com/lemon4ksan/mach/proto/http/zerocopy"
)

func TestAlloc(t *testing.T) {
	query := []byte("foo=bar&baz=qux&num=12345")
	url := []byte("https://example.com/api/v1/resource?foo=bar#section")

	allocs := testing.AllocsPerRun(1000, func() {
		args := zerocopy.AcquireArgs()
		args.ParseBytes(query)
		_ = args.Peek("foo")
		zerocopy.ReleaseArgs(args)

		uri := zerocopy.AcquireURI()
		uri.Parse(nil, url)
		_ = uri.Path()
		_ = uri.QueryString()
		zerocopy.ReleaseURI(uri)

		cookie := zerocopy.AcquireCookie()
		cookie.Parse("session_id=abc123xyz; Path=/; Secure; HttpOnly")
		_ = cookie.Value()
		zerocopy.ReleaseCookie(cookie)
	})

	if allocs > 0 {
		t.Fatalf("expected 0 allocations on zerocopy hot path, got %v", allocs)
	}
}

func TestParseIPv4_ZeroAlloc(t *testing.T) {
	dst := make([]byte, 4)
	ipStr := []byte("192.168.1.100")

	allocs := testing.AllocsPerRun(1000, func() {
		res, err := zerocopy.ParseIPv4(dst, ipStr)
		if err != nil || len(res) != 4 {
			t.Fatalf("ParseIPv4 failed: %v, %v", res, err)
		}
	})

	if allocs > 0 {
		t.Fatalf("expected 0 allocations on ParseIPv4 with reusable buffer, got %v", allocs)
	}
}

func TestFormatHexUint_ZeroAlloc(t *testing.T) {
	var buf [16]byte
	allocs := testing.AllocsPerRun(1000, func() {
		_ = zerocopy.FormatHexUint(&buf, 0x1A2B3C)
	})
	if allocs > 0 {
		t.Fatalf("expected 0 allocations on FormatHexUint, got %v", allocs)
	}
}

func TestDecodeArgAppend_ZeroAlloc(t *testing.T) {
	src := []byte("hello+world%20test%21%2Bmore")
	dst := make([]byte, 0, len(src))
	allocs := testing.AllocsPerRun(1000, func() {
		_ = zerocopy.DecodeArgAppend(dst[:0], src)
		_ = zerocopy.DecodeArgAppendNoPlus(dst[:0], src)
	})
	if allocs > 0 {
		t.Fatalf("expected 0 allocations on DecodeArgAppend, got %v", allocs)
	}
}


