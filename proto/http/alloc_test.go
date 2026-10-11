package http_test

import (
	"bufio"
	"bytes"
	"testing"

	"github.com/lemon4ksan/foundation/silicon/bytesconv"
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
		r.Header.Set("X-Custom-Auth", "bearer-token-12345")
		if string(r.Header.Peek("X-Custom-Auth")) != "bearer-token-12345" {
			t.Fatalf("unexpected header value: %s", r.Header.Peek("X-Custom-Auth"))
		}
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
		r.Header.Set("X-Custom-Server", "mach-edge/2.0")
		if string(r.Header.Peek("X-Custom-Server")) != "mach-edge/2.0" {
			t.Fatalf("unexpected header value: %s", r.Header.Peek("X-Custom-Server"))
		}
		r.SetBodyString(`{"status":"ok"}`)
		http.ReleaseResponse(r)
	})

	if allocs > 0 {
		t.Fatalf("expected 0 allocations on Response acquire/release/reuse hot path, got %v", allocs)
	}
}

func TestRequestStream_AcquireRelease_ZeroAlloc(t *testing.T) {
	buf := &bytesconv.ByteBuffer{B: []byte("test body")}
	reqHdr := http.AcquireRequestHeader()
	defer http.ReleaseRequestHeader(reqHdr)
	r := bufio.NewReader(bytes.NewReader(buf.B))

	// Prime the pool
	rs := http.AcquireRequestStream(buf, r, reqHdr)
	http.ReleaseRequestStream(rs)

	allocs := testing.AllocsPerRun(1000, func() {
		s := http.AcquireRequestStream(buf, r, reqHdr)
		http.ReleaseRequestStream(s)
	})

	if allocs > 0 {
		t.Fatalf("expected 0 allocations on RequestStream acquire/release, got %v", allocs)
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
