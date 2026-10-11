// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package etag_test

import (
	"testing"

	"github.com/lemon4ksan/mach/proto/semantic/etag"
)

func TestAlloc(t *testing.T) {
	tagA := `"123456789"`
	tagB := `W/"123456789"`

	tagABytes := []byte(tagA)
	tagBBytes := []byte(tagB)

	t.Run("StrongMatch", func(t *testing.T) {
		allocs := testing.AllocsPerRun(1000, func() {
			_ = etag.StrongMatch(tagA, tagA)
		})
		if allocs != 0 {
			t.Errorf("expected 0 allocs, got %v", allocs)
		}
	})

	t.Run("StrongMatchBytes", func(t *testing.T) {
		allocs := testing.AllocsPerRun(1000, func() {
			_ = etag.StrongMatch(tagABytes, tagABytes)
		})
		if allocs != 0 {
			t.Errorf("expected 0 allocs, got %v", allocs)
		}
	})

	t.Run("WeakMatch", func(t *testing.T) {
		allocs := testing.AllocsPerRun(1000, func() {
			_ = etag.WeakMatch(tagA, tagB)
		})
		if allocs != 0 {
			t.Errorf("expected 0 allocs, got %v", allocs)
		}
	})

	t.Run("WeakMatchBytes", func(t *testing.T) {
		allocs := testing.AllocsPerRun(1000, func() {
			_ = etag.WeakMatch(tagABytes, tagBBytes)
		})
		if allocs != 0 {
			t.Errorf("expected 0 allocs, got %v", allocs)
		}
	})

	t.Run("Normalize", func(t *testing.T) {
		allocs := testing.AllocsPerRun(1000, func() {
			_ = etag.Normalize(tagB)
		})
		if allocs != 0 {
			t.Errorf("expected 0 allocs, got %v", allocs)
		}
	})
}
