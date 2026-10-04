// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package hpkp_test

import (
	"testing"

	"github.com/lemon4ksan/mach/proto/http/hpkp"
)

func TestAlloc(t *testing.T) {
	header := `pin-sha256="cUPcTAZWKaASuYWhhneDttWpY3oBAkE3h2+soZS7sWs="; max-age=5184000; includeSubDomains`

	t.Run("ParseHeader", func(t *testing.T) {
		allocs := testing.AllocsPerRun(1000, func() {
			policy, err := hpkp.ParseHeader(header)
			if err != nil || len(policy.Pins) == 0 {
				t.Fatalf("unexpected parse result: policy=%v, err=%v", policy, err)
			}
		})
		if allocs > 10 {
			t.Errorf("expected minimal allocs, got %v", allocs)
		}
	})
}
