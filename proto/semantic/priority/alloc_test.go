// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package priority_test

import (
	"testing"

	"github.com/lemon4ksan/mach/proto/semantic/priority"
)

func TestAlloc(t *testing.T) {
	buf := make([]byte, 0, 16)
	p := priority.New(1, true)

	t.Run("AppendFormat", func(t *testing.T) {
		allocs := testing.AllocsPerRun(1000, func() {
			_ = p.AppendFormat(buf[:0])
		})
		if allocs != 0 {
			t.Errorf("expected 0 allocs for AppendFormat, got %v", allocs)
		}
	})

	t.Run("Parse", func(t *testing.T) {
		raw := "u=1, i"
		allocs := testing.AllocsPerRun(1000, func() {
			_, _ = priority.Parse(raw)
		})
		if allocs != 0 {
			t.Errorf("expected 0 allocs for Parse, got %v", allocs)
		}
	})

	t.Run("ParseBytes", func(t *testing.T) {
		raw := []byte("u=1, i")
		allocs := testing.AllocsPerRun(1000, func() {
			_, _ = priority.Parse(raw)
		})
		if allocs != 0 {
			t.Errorf("expected 0 allocs for ParseBytes, got %v", allocs)
		}
	})

	t.Run("ParseResult", func(t *testing.T) {
		raw := "u=1, i"
		allocs := testing.AllocsPerRun(1000, func() {
			res := priority.ParseResult(raw)
			if !res.IsSuccess() {
				t.Fatal("unexpected failure")
			}
			p := res.ValueOr(priority.Priority{})
			if p.Urgency != 1 || !p.Incremental {
				t.Fatalf("unexpected priority: %v", p)
			}
		})
		if allocs != 0 {
			t.Errorf("expected 0 allocs for ParseResult, got %v", allocs)
		}
	})
}
