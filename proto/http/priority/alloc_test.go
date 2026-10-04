// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package priority_test

import (
	"testing"

	"github.com/lemon4ksan/mach/proto/http/priority"
)

func TestAlloc(t *testing.T) {
	buf := make([]byte, 0, 16)
	p := priority.New(1, true)

	t.Run("AppendFormat", func(t *testing.T) {
		allocs := testing.AllocsPerRun(1000, func() {
			_ = p.AppendFormat(buf[:0])
		})
		if allocs != 0 {
			t.Errorf("expected 0 allocs, got %v", allocs)
		}
	})

	t.Run("Parse", func(t *testing.T) {
		raw := "u=1, i"
		allocs := testing.AllocsPerRun(1000, func() {
			_, _ = priority.Parse(raw)
		})
		// strings.SplitSeq with minimal allocations
		if allocs > 5 {
			t.Errorf("expected minimal allocs, got %v", allocs)
		}
	})
}
