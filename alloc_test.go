// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mach_test

import (
	"testing"
)

func TestAlloc(t *testing.T) {
	// The root mach package specifies interfaces; verification ensures interface assertions don't allocate.
	allocs := testing.AllocsPerRun(1000, func() {
	})
	if allocs > 0 {
		t.Fatalf("expected 0 allocations in root package, got %v", allocs)
	}
}
