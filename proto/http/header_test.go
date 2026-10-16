// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package http_test

import (
	"testing"

	"github.com/lemon4ksan/mach/proto/headkit"
	"github.com/lemon4ksan/mach/proto/http"
)

func TestRequestHeader_AcquireRelease_ZeroAlloc(t *testing.T) {
	// Prime the pool
	h := http.AcquireRequestHeader()
	http.ReleaseRequestHeader(h)

	allocs := testing.AllocsPerRun(1000, func() {
		reqHdr := http.AcquireRequestHeader()
		reqHdr.SetMethod("GET")
		reqHdr.SetRequestURI("/api/v2/resource")
		reqHdr.Set("User-Agent", "mach-client/2.0")
		reqHdr.Set("X-Custom-Token", "supersecret123")
		if string(reqHdr.Peek("X-Custom-Token")) != "supersecret123" {
			t.Fatalf("unexpected header value: %s", reqHdr.Peek("X-Custom-Token"))
		}
		http.ReleaseRequestHeader(reqHdr)
	})

	if allocs > 0 {
		t.Fatalf("expected 0 allocations on RequestHeader acquire/release hot path, got %v", allocs)
	}
}

func TestResponseHeader_AcquireRelease_ZeroAlloc(t *testing.T) {
	// Prime the pool
	h := http.AcquireResponseHeader()
	http.ReleaseResponseHeader(h)

	allocs := testing.AllocsPerRun(1000, func() {
		respHdr := http.AcquireResponseHeader()
		respHdr.SetStatusCode(200)
		respHdr.SetContentType("application/json")
		respHdr.Set("X-Server-Version", "v2.0.0")
		if string(respHdr.Peek("X-Server-Version")) != "v2.0.0" {
			t.Fatalf("unexpected header value: %s", respHdr.Peek("X-Server-Version"))
		}
		http.ReleaseResponseHeader(respHdr)
	})

	if allocs > 0 {
		t.Fatalf("expected 0 allocations on ResponseHeader acquire/release hot path, got %v", allocs)
	}
}

func TestRequestHeader_HeadkitIntegration(t *testing.T) {
	h := http.AcquireRequestHeader()
	defer http.ReleaseRequestHeader(h)

	hk := headkit.NewWithCapacity(8)
	hk.Set("X-Forwarded-For", "192.168.1.1")
	hk.Set("X-Request-ID", "req-xyz-987")

	h.SetFromHeadkit(&hk)

	if string(h.Peek("X-Forwarded-For")) != "192.168.1.1" {
		t.Fatalf("expected 192.168.1.1, got %s", h.Peek("X-Forwarded-For"))
	}
	if string(h.Peek("X-Request-ID")) != "req-xyz-987" {
		t.Fatalf("expected req-xyz-987, got %s", h.Peek("X-Request-ID"))
	}

	rawHK := h.Headkit()
	if rawHK == nil {
		t.Fatal("expected non-nil raw Headkit")
	}
	if rawHK.Get("X-Request-ID") != "req-xyz-987" {
		t.Fatalf("expected req-xyz-987 from direct Headkit, got %s", rawHK.Get("X-Request-ID"))
	}
}

func TestResponseHeader_HeadkitIntegration(t *testing.T) {
	h := http.AcquireResponseHeader()
	defer http.ReleaseResponseHeader(h)

	hk := headkit.NewWithCapacity(8)
	hk.Set("Cache-Control", "no-cache, no-store")
	hk.Set("ETag", "W/\"abcd1234\"")

	h.SetFromHeadkit(&hk)

	if string(h.Peek("Cache-Control")) != "no-cache, no-store" {
		t.Fatalf("expected no-cache, no-store, got %s", h.Peek("Cache-Control"))
	}
	if string(h.Peek("ETag")) != "W/\"abcd1234\"" {
		t.Fatalf("expected W/\"abcd1234\", got %s", h.Peek("ETag"))
	}

	rawHK := h.Headkit()
	if rawHK == nil {
		t.Fatal("expected non-nil raw Headkit")
	}
	if rawHK.Get("ETag") != "W/\"abcd1234\"" {
		t.Fatalf("expected W/\"abcd1234\" from direct Headkit, got %s", rawHK.Get("ETag"))
	}
}

func TestRequestHeader_All_ZeroAlloc(t *testing.T) {
	h := http.AcquireRequestHeader()
	defer http.ReleaseRequestHeader(h)

	h.Set("Host", "example.com")
	h.Set("Content-Type", "application/json")
	h.Set("User-Agent", "mach/1.0")
	h.Set("X-Custom", "custom-value")

	allocs := testing.AllocsPerRun(1000, func() {
		count := 0
		for k, v := range h.All() {
			if len(k) > 0 && len(v) > 0 {
				count++
			}
		}
		if count < 4 {
			t.Fatalf("expected at least 4 headers, got %d", count)
		}
	})

	if allocs > 0 {
		t.Fatalf("expected 0 allocations during h.All() iteration, got %v", allocs)
	}
}
