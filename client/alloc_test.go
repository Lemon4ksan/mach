// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package client_test

import (
	"testing"
	"time"

	"github.com/lemon4ksan/mach/client"
	"github.com/lemon4ksan/mach/proto/semantic/altsvc"
)

func TestAlloc(t *testing.T) {
	cache := client.NewAltSvcCache()
	cache.Set("fast.origin:443", altsvc.Service{
		ProtocolID: "h3",
		Port:       443,
		MaxAge:     time.Hour,
	})

	allocs := testing.AllocsPerRun(1000, func() {
		_, _ = cache.GetH3("fast.origin:443")
	})

	if allocs > 0 {
		t.Fatalf("expected 0 allocations in AltSvcCache.GetH3 hit, got %v", allocs)
	}
}

func BenchmarkAltSvcCache_GetH3(b *testing.B) {
	cache := client.NewAltSvcCache()
	cache.Set("fast.origin:443", altsvc.Service{
		ProtocolID: "h3",
		Port:       443,
		MaxAge:     time.Hour,
	})

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = cache.GetH3("fast.origin:443")
	}
}
