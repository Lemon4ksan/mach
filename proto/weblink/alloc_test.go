// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package weblink_test

import (
	"testing"

	"github.com/lemon4ksan/mach/proto/weblink"
)

func TestAlloc(t *testing.T) {
	header := `</page/2>; rel="next", </page/1>; rel="prev"`

	t.Run("AllIterator", func(t *testing.T) {
		allocs := testing.AllocsPerRun(1000, func() {
			count := 0
			for link, err := range weblink.All(header) {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if link.Target == "" {
					t.Fatal("empty target")
				}
				count++
			}
			if count != 2 {
				t.Fatalf("expected 2 links, got %d", count)
			}
		})
		// Iterating over link header tokens
		if allocs > 10 {
			t.Errorf("expected minimal allocs, got %v", allocs)
		}
	})
}
