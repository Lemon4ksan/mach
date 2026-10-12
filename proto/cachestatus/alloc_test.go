// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cachestatus_test

import (
	"testing"

	"github.com/lemon4ksan/mach/proto/cachestatus"
)

func TestAlloc(t *testing.T) {
	header := `OriginCache; hit; ttl=120, EdgeCache; fwd=uri-miss; key="abc"`

	t.Run("AllIterator", func(t *testing.T) {
		allocs := testing.AllocsPerRun(1000, func() {
			count := 0
			for entry, err := range cachestatus.All(header) {
				if err != nil {
					t.Fatalf("unexpected parse error: %v", err)
				}
				if entry.CacheName == "" {
					t.Fatal("empty cache name")
				}
				count++
			}
			if count != 2 {
				t.Fatalf("expected 2 entries, got %d", count)
			}
		})
		// Iterating over string tokens does not accumulate slices
		if allocs > 10 {
			t.Errorf("expected minimal allocs, got %v", allocs)
		}
	})

	t.Run("AllIteratorBytes", func(t *testing.T) {
		headerBytes := []byte(header)
		allocs := testing.AllocsPerRun(1000, func() {
			count := 0
			for entry, err := range cachestatus.All(headerBytes) {
				if err != nil {
					t.Fatalf("unexpected parse error: %v", err)
				}
				if entry.CacheName == "" {
					t.Fatal("empty cache name")
				}
				count++
			}
			if count != 2 {
				t.Fatalf("expected 2 entries, got %d", count)
			}
		})
		if allocs > 15 {
			t.Errorf("expected minimal allocs, got %v", allocs)
		}
	})
}
