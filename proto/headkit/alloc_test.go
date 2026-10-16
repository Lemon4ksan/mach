// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package headkit_test

import (
	"testing"

	"github.com/lemon4ksan/mach/proto/headkit"
)

func TestAlloc(t *testing.T) {
	rawBytes := []byte(`public, max-age=3600, must-revalidate, stale-while-revalidate=60`)
	rawStr := `public, max-age=3600, must-revalidate, stale-while-revalidate=60`
	paramBytes := []byte(`h3=":443"; ma=86400; persist=1`)
	paramStr := `h3=":443"; ma=86400; persist=1`
	ctBytes := []byte(`application/json; charset=utf-8`)
	ctStr := `application/json; charset=utf-8`

	allocs := testing.AllocsPerRun(1000, func() {
		for k, v := range headkit.Directives(rawStr) {
			_ = k
			_ = v
		}
		for k, v := range headkit.Directives(rawBytes) {
			_ = k
			_ = v
		}
		for k, v := range headkit.ParamDirectives(paramStr) {
			_ = k
			_ = v
		}
		for k, v := range headkit.ParamDirectives(paramBytes) {
			_ = k
			_ = v
		}

		_ = headkit.IsSensitive("authorization")
		_ = headkit.IsSensitive("cookie")
		_ = headkit.IsSensitive([]byte("authorization"))
		_ = headkit.IsSensitive([]byte("cookie"))

		_ = headkit.IsCanonical("User-Agent")
		_ = headkit.IsCanonical([]byte("User-Agent"))

		_ = headkit.BaseMediaType(ctStr)
		_ = headkit.BaseMediaType(ctBytes)

		_ = headkit.IsJSON(ctStr)
		_ = headkit.IsJSON(ctBytes)
		_ = headkit.IsXML(ctStr)
		_ = headkit.IsXML(ctBytes)
		_ = headkit.IsForm(ctStr)
		_ = headkit.IsForm(ctBytes)
		_ = headkit.IsMultipart(ctStr)
		_ = headkit.IsMultipart(ctBytes)

		_ = headkit.InternKey("content-type")
		_ = headkit.InternKey([]byte("accept-encoding"))
		_ = headkit.InternValue("application/json")
		_ = headkit.InternValue([]byte("gzip, deflate, br, zstd"))
	})

	if allocs > 0 {
		t.Fatalf("expected 0 allocations on headkit hot path, got %v", allocs)
	}
}
