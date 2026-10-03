package h1_test

import (
	"bytes"
	"net"
	"testing"
	"time"

	"github.com/lemon4ksan/mach/server/h1"
)

type mockConn struct {
	*bytes.Reader
	writeBuf []byte
}

func (m *mockConn) Write(p []byte) (n int, err error) {
	m.writeBuf = append(m.writeBuf, p...)
	return len(p), nil
}

type mockAddr struct{}

func (m mockAddr) Network() string { return "tcp" }
func (m mockAddr) String() string  { return "127.0.0.1:1234" }

func (m *mockConn) Close() error                       { return nil }
func (m *mockConn) LocalAddr() net.Addr                { return mockAddr{} }
func (m *mockConn) RemoteAddr() net.Addr               { return mockAddr{} }
func (m *mockConn) SetDeadline(t time.Time) error      { return nil }
func (m *mockConn) SetReadDeadline(t time.Time) error  { return nil }
func (m *mockConn) SetWriteDeadline(t time.Time) error { return nil }

func TestServeConnAllocations(t *testing.T) {
	reqData := []byte("GET / HTTP/1.1\r\nHost: localhost\r\n\r\n")

	handler := &h1.ConnHandler{
		Handler: func(req *h1.Request, res *h1.Response) error {
			res.StatusCode = 200
			res.Body = append(res.Body[:0], []byte("OK")...)
			return nil
		},
	}

	var conn mockConn
	conn.Reader = bytes.NewReader(reqData)

	allocs := testing.AllocsPerRun(1000, func() {
		conn.Reader.Reset(reqData)
		conn.writeBuf = conn.writeBuf[:0]
		_ = handler.ServeConn(&conn)
	})

	if allocs != 3 {
		t.Errorf("ServeConn allocated %v times, expected 3", allocs)
	}
}
