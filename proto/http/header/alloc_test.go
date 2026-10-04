// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package header_test

import (
	"testing"

	"github.com/lemon4ksan/mach/proto/http/header"
)

func TestAlloc(t *testing.T) {
	allocs := testing.AllocsPerRun(1000, func() {
		_ = header.ContentType == "Content-Type"
		_ = header.PseudoMethod == ":method"
	})
	if allocs != 0 {
		t.Errorf("expected 0 allocs, got %v", allocs)
	}
}
