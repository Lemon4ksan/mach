// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package altsvc_test

import (
	"testing"

	"github.com/lemon4ksan/mach/proto/http/altsvc"
)

func TestAlloc(t *testing.T) {
	header := `h3=":443"; ma=2592000, h2="alt.example.com:443"; persist=1`

	t.Run("AllIterator", func(t *testing.T) {
		allocs := testing.AllocsPerRun(1000, func() {
			count := 0
			for svc := range altsvc.All(header) {
				if svc.ProtocolID == "" || svc.Port == 0 {
					t.Fatal("invalid service")
				}
				count++
			}
			if count != 2 {
				t.Fatalf("expected 2 services, got %d", count)
			}
		})
		if allocs > 15 {
			t.Errorf("expected minimal allocs, got %v", allocs)
		}
	})
}
