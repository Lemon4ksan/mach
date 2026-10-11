package http_test

import (
	"testing"

	"github.com/lemon4ksan/mach/proto/http"
)

func TestRequest_AcquireRelease_ZeroAlloc(t *testing.T) {
	// Prime the pool
	req := http.AcquireRequest()
	http.ReleaseRequest(req)

	allocs := testing.AllocsPerRun(1000, func() {
		r := http.AcquireRequest()
		r.Header.SetMethod("GET")
		r.SetRequestURI("/api/v1/test")
		r.Header.Set("User-Agent", "mach-client/1.0")
		r.SetBodyString("ping")
		http.ReleaseRequest(r)
	})

	if allocs > 0 {
		t.Fatalf("expected 0 allocations on Request acquire/release/reuse hot path, got %v", allocs)
	}
}

func TestResponse_AcquireRelease_ZeroAlloc(t *testing.T) {
	// Prime the pool
	resp := http.AcquireResponse()
	http.ReleaseResponse(resp)

	allocs := testing.AllocsPerRun(1000, func() {
		r := http.AcquireResponse()
		r.SetStatusCode(200)
		r.Header.Set("Content-Type", "application/json")
		r.SetBodyString(`{"status":"ok"}`)
		http.ReleaseResponse(r)
	})

	if allocs > 0 {
		t.Fatalf("expected 0 allocations on Response acquire/release/reuse hot path, got %v", allocs)
	}
}

func BenchmarkAcquireReleaseRequest(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		req := http.AcquireRequest()
		http.ReleaseRequest(req)
	}
}

func BenchmarkAcquireReleaseRequest_Parallel(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			req := http.AcquireRequest()
			http.ReleaseRequest(req)
		}
	})
}

func BenchmarkAcquireReleaseResponse(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		resp := http.AcquireResponse()
		http.ReleaseResponse(resp)
	}
}

func BenchmarkAcquireReleaseResponse_Parallel(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			resp := http.AcquireResponse()
			http.ReleaseResponse(resp)
		}
	})
}
