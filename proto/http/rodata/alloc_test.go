// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rodata_test

import (
	"bytes"
	"testing"

	"github.com/lemon4ksan/mach/proto/http/rodata"
)

func TestAlloc(t *testing.T) {
	key := []byte("Content-Type")

	allocs := testing.AllocsPerRun(1000, func() {
		_ = bytes.Equal(rodata.KeyContentType, []byte("content-type"))
		_ = rodata.InternKeyBytes(key)
	})
	if allocs != 0 {
		t.Errorf("expected 0 allocs, got %v", allocs)
	}
}
