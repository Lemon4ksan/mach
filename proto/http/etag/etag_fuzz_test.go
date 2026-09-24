// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package etag_test

import (
	"testing"

	"github.com/lemon4ksan/mach/proto/http/etag"
)

func FuzzETagMatch(f *testing.F) {
	f.Add(`"xyzzy"`, `"xyzzy"`)
	f.Add(`W/"xyzzy"`, `"xyzzy"`)
	f.Add(`W/"xyzzy"`, `W/"xyzzy"`)
	f.Add(`"123"`, `"456"`)
	f.Add(``, `"123"`)
	f.Add(`*`, `"xyzzy"`)

	f.Fuzz(func(t *testing.T, a, b string) {
		_ = etag.StrongMatch(a, b)
		_ = etag.WeakMatch(a, b)
		_ = etag.IsWeak(a)
		_ = etag.Normalize(a)
	})
}
