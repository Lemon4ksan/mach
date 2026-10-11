package h3_test

import (
	"bytes"
	"testing"

	coreh3 "github.com/lemon4ksan/mach/proto/h3"
)

type mockStream struct {
	buf bytes.Buffer
}

func (s *mockStream) Read(p []byte) (n int, err error)  { return s.buf.Read(p) }
func (s *mockStream) Write(p []byte) (n int, err error) { return s.buf.Write(p) }
func (s *mockStream) Close() error                     { return nil }

var _ coreh3.Stream = (*mockStream)(nil)
var _ coreh3.ReceiveStream = (*mockStream)(nil)
var _ coreh3.SendStream = (*mockStream)(nil)

func TestAllocations(t *testing.T) {
	headerBytes := coreh3.AppendHeadersHeader(nil, 128)
	br := bytes.NewReader(headerBytes)

	allocs := testing.AllocsPerRun(1000, func() {
		br.Reset(headerBytes)
		_, _, _ = coreh3.ReadFrameHeader(br)
	})
	if allocs > 0 {
		t.Errorf("got %v allocs, want 0", allocs)
	}
}

func TestAppendHeadersAlloc(t *testing.T) {
	buf := make([]byte, 0, 16)

	allocs := testing.AllocsPerRun(1000, func() {
		b := coreh3.AppendHeadersHeader(buf[:0], 256)
		_ = coreh3.AppendDataHeader(b[:0], 1024)
	})
	if allocs != 0 {
		t.Errorf("got %v allocs, want 0", allocs)
	}
}
