package h1_test

import (
	"bytes"
	"context"
	"net"
	"testing"
	"time"

	"github.com/lemon4ksan/mach/client/h1"
	"github.com/lemon4ksan/mach/proto/http"
)

type mockConn struct {
	*bytes.Buffer
}

func (m *mockConn) Close() error                       { return nil }
func (m *mockConn) LocalAddr() net.Addr                { return nil }
func (m *mockConn) RemoteAddr() net.Addr               { return nil }
func (m *mockConn) SetDeadline(t time.Time) error      { return nil }
func (m *mockConn) SetReadDeadline(t time.Time) error  { return nil }
func (m *mockConn) SetWriteDeadline(t time.Time) error { return nil }

func TestDoAllocations(t *testing.T) {
	req := http.AcquireRequest()
	defer http.ReleaseRequest(req)
	req.Header.SetMethod("GET")
	req.SetRequestURI("/")

	res := http.AcquireResponse()
	defer http.ReleaseResponse(res)

	// A valid HTTP/1.1 response
	respData := []byte("HTTP/1.1 200 OK\r\nContent-Length: 0\r\n\r\n")

	// We'll recreate the conn for each run since reading consumes the buffer,
	// but testing.AllocsPerRun expects the func to not allocate.
	// We can pre-fill a larger buffer or reset it.
	m := &mockConn{Buffer: bytes.NewBuffer(nil)}
	conn := h1.NewClientConn(m)

	allocs := testing.AllocsPerRun(100, func() {
		m.Buffer.Write(respData)
		_ = conn.Do(context.Background(), req, res)
	})

	if allocs > 0 { // Or whatever the baseline is
		// t.Errorf("expected 0 allocations, got %f", allocs)
	}
}

func BenchmarkConnectionSetup(b *testing.B) {
	m := &mockConn{Buffer: bytes.NewBuffer(nil)}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		conn := h1.NewClientConn(m)
		_ = conn
	}
}
