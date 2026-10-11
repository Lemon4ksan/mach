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

	t.Run("ForEach", func(t *testing.T) {
		allocs := testing.AllocsPerRun(1000, func() {
			count := 0
			altsvc.ForEach(header, func(svc altsvc.Service) bool {
				if svc.ProtocolID == "" || svc.Port == 0 {
					t.Fatal("invalid service")
				}
				count++
				return true
			})
			if count != 2 {
				t.Fatalf("expected 2 services, got %d", count)
			}
		})
		if allocs != 0 {
			t.Errorf("expected 0 allocs for ForEach, got %v", allocs)
		}
	})

	t.Run("AppendFormat", func(t *testing.T) {
		services := []altsvc.Service{
			{ProtocolID: "h3", Host: "", Port: 443},
			{ProtocolID: "h2", Host: "alt.example.com", Port: 8443, Persist: true},
		}
		var buf [128]byte
		allocs := testing.AllocsPerRun(1000, func() {
			_ = altsvc.AppendFormat(buf[:0], services...)
		})
		if allocs != 0 {
			t.Errorf("expected 0 allocs for AppendFormat, got %v", allocs)
		}
	})
}
