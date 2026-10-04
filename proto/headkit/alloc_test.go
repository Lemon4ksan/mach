// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package headkit_test

import (
	"testing"

	"github.com/lemon4ksan/mach/proto/headkit"
)

func TestAlloc(t *testing.T) {
	raw := []byte(`public, max-age=3600, must-revalidate, stale-while-revalidate=60`)

	allocs := testing.AllocsPerRun(1000, func() {
		for k, v := range headkit.DirectivesBytes(raw) {
			_ = k
			_ = v
		}

		_ = headkit.IsSensitive("authorization")
		_ = headkit.IsSensitive("cookie")
	})

	if allocs > 0 {
		t.Fatalf("expected 0 allocations on headkit hot path, got %v", allocs)
	}
}
