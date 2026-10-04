// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth_test

import (
	"testing"

	"github.com/lemon4ksan/mach/proto/http/auth"
)

func TestAlloc(t *testing.T) {
	header := "Basic QWxhZGRpbjpvcGVuIHNlc2FtZQ=="

	t.Run("ParseBasic", func(t *testing.T) {
		allocs := testing.AllocsPerRun(1000, func() {
			user, pass, err := auth.ParseBasic(header)
			if err != nil || user != "Aladdin" || pass != "open sesame" {
				t.Fatalf("unexpected parse: user=%s pass=%s err=%v", user, pass, err)
			}
		})
		// Small number of allocs due to string returns
		if allocs > 5 {
			t.Errorf("expected minimal allocs, got %v", allocs)
		}
	})
}
