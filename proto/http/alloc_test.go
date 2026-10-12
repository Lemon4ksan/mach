package http_test

import (
	"bufio"
	"bytes"
	"testing"

	"github.com/lemon4ksan/foundation/silicon/bytesconv"

	"github.com/lemon4ksan/mach/proto/http"
	"github.com/lemon4ksan/mach/proto/semantic/cookie"
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

func TestRequestHeader_MethodAndFields_ZeroAlloc(t *testing.T) {
	h := http.AcquireRequestHeader()
	defer http.ReleaseRequestHeader(h)

	// Prime the header
	_ = h.Method()
	_ = h.IsGet()

	allocs := testing.AllocsPerRun(100, func() {
		h.Reset()
		if !h.IsGet() {
			t.Fatal("expected IsGet to be true by default")
		}
		if string(h.Method()) != "GET" {
			t.Fatal("expected GET method by default")
		}

		h.SetMethodBytes([]byte("POST"))
		if !h.IsPost() {
			t.Fatal("expected IsPost to be true")
		}
		if h.IsGet() {
			t.Fatal("expected IsGet to be false")
		}

		h.SetRequestURIBytes([]byte("/index.html"))
		h.SetHostBytes([]byte("example.com"))
		h.SetUserAgentBytes([]byte("mach-agent/1.0"))
		h.SetContentTypeBytes([]byte("application/json"))
		h.SetContentLength(42)

		if string(h.Host()) != "example.com" {
			t.Fatal("host mismatch")
		}
		if string(h.UserAgent()) != "mach-agent/1.0" {
			t.Fatal("ua mismatch")
		}
		if string(h.ContentType()) != "application/json" {
			t.Fatal("content-type mismatch")
		}
		if h.ContentLength() != 42 {
			t.Fatal("content-length mismatch")
		}

		// Test disabled special header path
		h.DisableSpecialHeader()
		if string(h.Host()) != "" {
			t.Fatal("expected empty host when special disabled")
		}
		if string(h.ContentType()) != "" {
			t.Fatal("expected empty content-type when special disabled")
		}
		if string(h.UserAgent()) != "" {
			t.Fatal("expected empty user-agent when special disabled")
		}
		h.EnableSpecialHeader()
	})

	if allocs > 0 {
		t.Fatalf("expected 0 allocations on RequestHeader method and fields hot path, got %v", allocs)
	}
}

func TestHeader_Cookies_ZeroAlloc(t *testing.T) {
	reqHdr := http.AcquireRequestHeader()
	defer http.ReleaseRequestHeader(reqHdr)

	respHdr := http.AcquireResponseHeader()
	defer http.ReleaseResponseHeader(respHdr)

	ckKey := []byte("session_id")
	ckVal := []byte("abcd1234xyz")

	// Prime
	reqHdr.SetCookieBytesKV(ckKey, ckVal)
	_ = reqHdr.CookieBytes(ckKey)
	reqHdr.DelCookieBytes(ckKey)

	c := cookie.AcquireCookie()
	c.SetKeyBytes(ckKey)
	c.SetValueBytes(ckVal)
	respHdr.SetCookie(c)
	respHdr.DelClientCookieBytes(ckKey)
	cookie.ReleaseCookie(c)

	allocs := testing.AllocsPerRun(100, func() {
		// RequestHeader cookie hot path
		reqHdr.Reset()
		reqHdr.SetCookieBytesKV(ckKey, ckVal)
		val := reqHdr.CookieBytes(ckKey)
		if !bytes.Equal(val, ckVal) {
			t.Fatalf("expected cookie %s, got %s", ckVal, val)
		}
		count := 0
		for k, v := range reqHdr.Cookies() {
			if len(k) > 0 && len(v) > 0 {
				count++
			}
		}
		if count != 1 {
			t.Fatalf("expected 1 cookie, got %d", count)
		}
		reqHdr.DelCookieBytes(ckKey)
		if reqHdr.CookieBytes(ckKey) != nil {
			t.Fatal("cookie should be deleted")
		}

		// ResponseHeader cookie hot path
		respHdr.Reset()
		ck := cookie.AcquireCookie()
		ck.SetKeyBytes(ckKey)
		ck.SetValueBytes(ckVal)
		respHdr.SetCookie(ck)
		cookie.ReleaseCookie(ck)

		peekVal := respHdr.PeekCookie("session_id")
		if len(peekVal) == 0 {
			t.Fatal("expected non-empty cookie")
		}
		respHdr.DelClientCookieBytes(ckKey)
		respHdr.DelCookieBytes(ckKey)
	})

	if allocs > 0 {
		t.Fatalf("expected 0 allocations on Cookie operations, got %v", allocs)
	}
}

func TestHeader_TrailersAndSerialization_ZeroAlloc(t *testing.T) {
	reqHdr := http.AcquireRequestHeader()
	defer http.ReleaseRequestHeader(reqHdr)

	respHdr := http.AcquireResponseHeader()
	defer http.ReleaseResponseHeader(respHdr)

	trailerKey := []byte("X-Checksum")

	// Prime
	_ = reqHdr.SetTrailerBytes(trailerKey)
	_ = reqHdr.Header()
	_ = respHdr.SetTrailerBytes(trailerKey)
	_ = respHdr.Header()

	dst := make([]byte, 0, 1024)

	allocs := testing.AllocsPerRun(100, func() {
		reqHdr.Reset()
		reqHdr.SetMethod("GET")
		reqHdr.SetRequestURI("/test")
		reqHdr.SetHost("example.com")
		_ = reqHdr.SetTrailerBytes(trailerKey)
		reqHdr.Set("X-Checksum", "sha256-abcdef")

		// Peek trailer
		tBytes := reqHdr.Peek(http.HeaderTrailer)
		if len(tBytes) == 0 {
			t.Fatal("expected trailer header")
		}

		// Iterate All()
		for k, v := range reqHdr.All() {
			_ = k
			_ = v
		}

		// Serialization
		dst = reqHdr.AppendBytes(dst[:0])
		if len(dst) == 0 {
			t.Fatal("expected serialized header")
		}

		// Response serialization
		respHdr.Reset()
		respHdr.SetStatusCode(200)
		_ = respHdr.SetTrailerBytes(trailerKey)
		respHdr.Set("X-Checksum", "sha256-abcdef")

		dst = respHdr.AppendBytes(dst[:0])
		if len(dst) == 0 {
			t.Fatal("expected serialized response header")
		}
	})

	if allocs > 0 {
		t.Fatalf("expected 0 allocations on Trailer and Serialization hot path, got %v", allocs)
	}
}

func TestHeader_ParseRead_ZeroAlloc(t *testing.T) {
	rawReq := []byte(
		"GET /api/v1/test HTTP/1.1\r\nHost: example.com\r\nUser-Agent: mach-client/1.0\r\nContent-Type: application/json\r\nContent-Length: 18\r\nConnection: keep-alive\r\n\r\n",
	)
	rawResp := []byte(
		"HTTP/1.1 200 OK\r\nServer: mach-edge\r\nContent-Type: text/plain\r\nContent-Length: 5\r\nConnection: keep-alive\r\n\r\n",
	)

	reqHdr := http.AcquireRequestHeader()
	defer http.ReleaseRequestHeader(reqHdr)

	respHdr := http.AcquireResponseHeader()
	defer http.ReleaseResponseHeader(respHdr)

	bufReader := bufio.NewReader(bytes.NewReader(rawReq))

	// Prime
	_ = reqHdr.Read(bufReader)
	bufReader.Reset(bytes.NewReader(rawResp))
	_ = respHdr.Read(bufReader)

	byteReader := bytes.NewReader(nil)

	allocs := testing.AllocsPerRun(100, func() {
		byteReader.Reset(rawReq)
		bufReader.Reset(byteReader)

		if err := reqHdr.Read(bufReader); err != nil {
			t.Fatalf("unexpected error reading request: %v", err)
		}
		if !reqHdr.IsGet() {
			t.Fatal("expected GET")
		}
		if string(reqHdr.Host()) != "example.com" {
			t.Fatal("host mismatch")
		}
		if reqHdr.ContentLength() != 18 {
			t.Fatal("content length mismatch")
		}

		byteReader.Reset(rawResp)
		bufReader.Reset(byteReader)

		if err := respHdr.Read(bufReader); err != nil {
			t.Fatalf("unexpected error reading response: %v", err)
		}
		if respHdr.StatusCode() != 200 {
			t.Fatal("status code mismatch")
		}
		if string(respHdr.Server()) != "mach-edge" {
			t.Fatal("server mismatch")
		}
	})

	if allocs > 0 {
		t.Fatalf("expected 0 allocations on RequestHeader/ResponseHeader Read hot path, got %v", allocs)
	}
}

func TestResponseHeader_AltSvcAndVary_ZeroAlloc(t *testing.T) {
	respHdr := http.AcquireResponseHeader()
	defer http.ReleaseResponseHeader(respHdr)

	// Prime
	_ = respHdr.AltSvc()

	allocs := testing.AllocsPerRun(100, func() {
		respHdr.Reset()
		if services := respHdr.AltSvc(); services != nil {
			t.Fatal("expected nil Alt-Svc")
		}

		respHdr.Set("Vary", "Accept-Encoding")
		respHdr.Set("Vary", "User-Agent")
		v := respHdr.Peek("Vary")
		if len(v) == 0 {
			t.Fatal("expected Vary header")
		}
	})

	if allocs > 0 {
		t.Fatalf("expected 0 allocations on AltSvc and Vary hot path, got %v", allocs)
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
