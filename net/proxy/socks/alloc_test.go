// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package socks_test

import (
	"context"
	"io"
	"net"
	"testing"
	"time"

	"github.com/lemon4ksan/mach/net/proxy/socks"
)

type mockLoopConn struct {
	readBuf  []byte
	readOff  int
	writeBuf [256]byte
	writeOff int
}

func (m *mockLoopConn) Read(b []byte) (n int, err error) {
	if m.readOff >= len(m.readBuf) {
		return 0, io.EOF
	}
	n = copy(b, m.readBuf[m.readOff:])
	m.readOff += n
	return n, nil
}

func (m *mockLoopConn) Write(b []byte) (n int, err error) {
	n = copy(m.writeBuf[m.writeOff:], b)
	m.writeOff += n
	return n, nil
}

func (m *mockLoopConn) Close() error { return nil }
func (m *mockLoopConn) LocalAddr() net.Addr {
	return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 1080}
}

func (m *mockLoopConn) RemoteAddr() net.Addr {
	return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 1080}
}
func (m *mockLoopConn) SetDeadline(t time.Time) error      { return nil }
func (m *mockLoopConn) SetReadDeadline(t time.Time) error  { return nil }
func (m *mockLoopConn) SetWriteDeadline(t time.Time) error { return nil }

func (m *mockLoopConn) reset(resp []byte) {
	m.readBuf = resp
	m.readOff = 0
	m.writeOff = 0
}

func TestSOCKS5_AllocBudget(t *testing.T) {
	// Standard response: Auth (0x05, 0x00) + Connect Reply IPv4 (10 bytes)
	resp := []byte{
		0x05, 0x00,
		0x05, 0x00, 0x00, 0x01, 127, 0, 0, 1, 0x00, 0x50,
	}

	mock := &mockLoopConn{}
	d := socks.NewDialer("tcp", "127.0.0.1:1080")
	ctx := context.Background()

	allocs := testing.AllocsPerRun(100, func() {
		mock.reset(resp)
		_, err := d.DialWithConn(ctx, mock, "tcp", "127.0.0.1:80")
		if err != nil {
			t.Fatalf("dial failed: %v", err)
		}
	})

	t.Logf("SOCKS5 DialWithConn allocs per run: %v", allocs)
	// We document the allocation count here. In a zero-alloc SOCKS5 implementation,
	// this should be 0 allocations.
}
