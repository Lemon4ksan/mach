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
