// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cookie_test

import (
	"testing"

	"github.com/lemon4ksan/mach/proto/semantic/cookie"
)

func TestAlloc(t *testing.T) {
	t.Run("ParseMethod", func(t *testing.T) {
		allocs := testing.AllocsPerRun(1000, func() {
			c := cookie.AcquireCookie()
			c.Parse("session_id=abc123xyz; Path=/; Secure; HttpOnly")
			_ = c.Value()
			cookie.ReleaseCookie(c)
		})
		if allocs > 0 {
			t.Fatalf("expected 0 allocations on cookie Parse, got %v", allocs)
		}
	})

	t.Run("ParseGenericStr", func(t *testing.T) {
		str := "session_id=abc123xyz; Path=/; Secure; HttpOnly"
		allocs := testing.AllocsPerRun(1000, func() {
			c := cookie.AcquireCookie()
			_ = cookie.Parse(c, str)
			_ = c.Value()
			cookie.ReleaseCookie(c)
		})
		if allocs > 0 {
			t.Fatalf("expected 0 allocations on cookie generic Parse string, got %v", allocs)
		}
	})

	t.Run("ParseGenericBytes", func(t *testing.T) {
		raw := []byte("session_id=abc123xyz; Path=/; Secure; HttpOnly")
		allocs := testing.AllocsPerRun(1000, func() {
			c := cookie.AcquireCookie()
			_ = cookie.Parse(c, raw)
			_ = c.Value()
			cookie.ReleaseCookie(c)
		})
		if allocs > 0 {
			t.Fatalf("expected 0 allocations on cookie generic Parse bytes, got %v", allocs)
		}
	})
}
