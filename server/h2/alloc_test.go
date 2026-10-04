package h2_test

import (
	"bytes"
	"net"
	"testing"
	"time"

	"github.com/lemon4ksan/mach/server/h2"
)

type mockAddr struct{}

func (m mockAddr) Network() string { return "tcp" }
func (m mockAddr) String() string  { return "127.0.0.1:1234" }

type mockConn struct {
	*bytes.Reader
	writeBuf []byte
}

func (m *mockConn) Write(p []byte) (n int, err error) {
	m.writeBuf = append(m.writeBuf, p...)
	return len(p), nil
}

func (m *mockConn) Close() error                       { return nil }
func (m *mockConn) LocalAddr() net.Addr                { return mockAddr{} }
func (m *mockConn) RemoteAddr() net.Addr               { return mockAddr{} }
func (m *mockConn) SetDeadline(t time.Time) error      { return nil }
func (m *mockConn) SetReadDeadline(t time.Time) error  { return nil }
func (m *mockConn) SetWriteDeadline(t time.Time) error { return nil }

func TestServeConnAllocations(t *testing.T) {
	// Send client preface so that server doesn't immediately fail.
	reqData := []byte("PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n")

	handler := func(req *h2.ServerRequest, res *h2.ServerResponse) error {
		return nil
	}

	var conn mockConn
	conn.Reader = bytes.NewReader(reqData)

	allocs := testing.AllocsPerRun(100, func() {
		conn.Reader.Reset(reqData)
		conn.writeBuf = conn.writeBuf[:0]
		sc := h2.NewServerConn(&conn, handler)
		_ = sc.Serve()
	})

	if allocs > 17 {
		t.Errorf("Serve() allocated %v times, expected <= 17", allocs)
	}
}
