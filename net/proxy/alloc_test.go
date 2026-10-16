// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package proxy_test

import (
	"bytes"
	"context"
	"net"
	"testing"
	"time"

	"github.com/lemon4ksan/foundation/testing/assert"
	"github.com/lemon4ksan/foundation/testing/require"

	"github.com/lemon4ksan/mach/net/proxy"
)

type mockDuplexConn struct {
	reader   bytes.Reader
	writeBuf [2048]byte
	writeN   int
}

func (m *mockDuplexConn) Read(b []byte) (int, error) {
	return m.reader.Read(b)
}

func (m *mockDuplexConn) Write(b []byte) (int, error) {
	n := copy(m.writeBuf[m.writeN:], b)
	m.writeN += n
	return len(b), nil
}

func (m *mockDuplexConn) Close() error                       { return nil }
func (m *mockDuplexConn) LocalAddr() net.Addr                { return nil }
func (m *mockDuplexConn) RemoteAddr() net.Addr               { return nil }
func (m *mockDuplexConn) SetDeadline(t time.Time) error      { return nil }
func (m *mockDuplexConn) SetReadDeadline(t time.Time) error  { return nil }
func (m *mockDuplexConn) SetWriteDeadline(t time.Time) error { return nil }

func TestSOCKS5_ZeroAllocations(t *testing.T) {
	// Greeting response: VER=5, METHOD=0 (No Auth)
	// CONNECT response: VER=5, REP=0, RSV=0, ATYP=1 (IPv4), BND.ADDR=127.0.0.1, BND.PORT=8080
	respBytes := []byte{
		0x05, 0x00,
		0x05, 0x00, 0x00, 0x01, 127, 0, 0, 1, 0x1F, 0x90,
	}

	d := proxy.NewSOCKS5Dialer("tcp", "127.0.0.1:1080", nil, nil)
	m := &mockDuplexConn{}
	ctx := context.Background()

	// Warmup
	m.reader.Reset(respBytes)
	m.writeN = 0
	err := d.Handshake(ctx, m, "target.example.com:443")
	require.NoError(t, err)

	allocs := testing.AllocsPerRun(100, func() {
		m.reader.Reset(respBytes)
		m.writeN = 0
		_ = d.Handshake(ctx, m, "target.example.com:443")
	})

	assert.Equal(t, 0.0, allocs, "SOCKS5 handshake must incur 0 allocations")
}

func TestSOCKS5_IPv4_ZeroAllocations(t *testing.T) {
	respBytes := []byte{
		0x05, 0x00,
		0x05, 0x00, 0x00, 0x01, 127, 0, 0, 1, 0x1F, 0x90,
	}

	d := proxy.NewSOCKS5Dialer("tcp", "127.0.0.1:1080", nil, nil)
	m := &mockDuplexConn{}
	ctx := context.Background()

	allocs := testing.AllocsPerRun(100, func() {
		m.reader.Reset(respBytes)
		m.writeN = 0
		_ = d.Handshake(ctx, m, "93.184.216.34:80")
	})

	assert.Equal(t, 0.0, allocs, "SOCKS5 IPv4 handshake must incur 0 allocations")
}

func TestHTTPConnect_ZeroAllocations(t *testing.T) {
	respBytes := []byte("HTTP/1.1 200 Connection established\r\nProxy-Agent: mach\r\n\r\n")

	d := proxy.NewHTTPConnectDialer("tcp", "127.0.0.1:8080", nil, nil)
	m := &mockDuplexConn{}
	ctx := context.Background()

	// Warmup
	m.reader.Reset(respBytes)
	m.writeN = 0
	_, err := d.Handshake(ctx, m, "target.example.com:443")
	require.NoError(t, err)

	allocs := testing.AllocsPerRun(100, func() {
		m.reader.Reset(respBytes)
		m.writeN = 0
		_, _ = d.Handshake(ctx, m, "target.example.com:443")
	})

	assert.Equal(t, 0.0, allocs, "HTTP CONNECT handshake must incur 0 allocations")
}

func TestHTTPConnect_BasicAuth_ZeroAllocations(t *testing.T) {
	respBytes := []byte("HTTP/1.1 200 Connection established\r\n\r\n")

	auth := &proxy.Auth{User: "alice", Password: "secretpassword"}
	d := proxy.NewHTTPConnectDialer("tcp", "127.0.0.1:8080", auth, nil)
	m := &mockDuplexConn{}
	ctx := context.Background()

	allocs := testing.AllocsPerRun(100, func() {
		m.reader.Reset(respBytes)
		m.writeN = 0
		_, _ = d.Handshake(ctx, m, "target.example.com:443")
	})

	assert.Equal(t, 0.0, allocs, "HTTP CONNECT with Basic Auth must incur 0 allocations")
}

func BenchmarkSOCKS5_Handshake(b *testing.B) {
	respBytes := []byte{
		0x05, 0x00,
		0x05, 0x00, 0x00, 0x01, 127, 0, 0, 1, 0x1F, 0x90,
	}

	d := proxy.NewSOCKS5Dialer("tcp", "127.0.0.1:1080", nil, nil)
	m := &mockDuplexConn{}
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		m.reader.Reset(respBytes)
		m.writeN = 0
		_ = d.Handshake(ctx, m, "target.example.com:443")
	}
}

func BenchmarkHTTPConnect_Handshake(b *testing.B) {
	respBytes := []byte("HTTP/1.1 200 Connection established\r\n\r\n")

	d := proxy.NewHTTPConnectDialer("tcp", "127.0.0.1:8080", nil, nil)
	m := &mockDuplexConn{}
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		m.reader.Reset(respBytes)
		m.writeN = 0
		_, _ = d.Handshake(ctx, m, "target.example.com:443")
	}
}

func BenchmarkHTTPConnect_BasicAuth_Handshake(b *testing.B) {
	respBytes := []byte("HTTP/1.1 200 Connection established\r\n\r\n")

	auth := &proxy.Auth{User: "alice", Password: "secretpassword"}
	d := proxy.NewHTTPConnectDialer("tcp", "127.0.0.1:8080", auth, nil)
	m := &mockDuplexConn{}
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		m.reader.Reset(respBytes)
		m.writeN = 0
		_, _ = d.Handshake(ctx, m, "target.example.com:443")
	}
}
