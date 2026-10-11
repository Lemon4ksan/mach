// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package uri_test

import (
	"testing"

	"github.com/lemon4ksan/mach/proto/semantic/uri"
)

func TestAlloc(t *testing.T) {
	query := []byte("foo=bar&baz=qux&num=12345")
	url := []byte("https://example.com/api/v1/resource?foo=bar#section")

	allocs := testing.AllocsPerRun(1000, func() {
		args := uri.AcquireArgs()
		args.ParseBytes(query)
		_ = args.Peek("foo")
		uri.ReleaseArgs(args)

		u := uri.AcquireURI()
		u.Parse(nil, url)
		_ = u.Path()
		_ = u.QueryString()
		uri.ReleaseURI(u)
	})

	if allocs > 0 {
		t.Fatalf("expected 0 allocations on uri hot path, got %v", allocs)
	}
}

func TestDecodeArgAppend_ZeroAlloc(t *testing.T) {
	dst := make([]byte, 0, 64)
	src := []byte("hello%20world%21%2Bmore%2Fbytes")

	allocs := testing.AllocsPerRun(1000, func() {
		_ = uri.DecodeArgAppend(dst[:0], src)
	})

	if allocs > 0 {
		t.Fatalf("expected 0 allocations on DecodeArgAppend, got %v", allocs)
	}
}
