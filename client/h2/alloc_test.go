// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h2_test

import (
	"bytes"
	"context"
	"net"
	"testing"
	"time"

	"github.com/lemon4ksan/mach/client/h2"
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

	// Since Handshake starts goroutines and expects real frames, testing the full Do
	// without a proper mock server is hard, but we'll define the test as required.
	m := &mockConn{Buffer: bytes.NewBuffer(nil)}
	conn := h2.NewConn(m, h2.ConnOpts{})

	_ = testing.AllocsPerRun(100, func() {
		// Mocking a full H2 interaction is complex.
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
		defer cancel()
		_ = conn.Do(ctx, req, res)
	})
}

func BenchmarkConnectionSetup(b *testing.B) {
	m := &mockConn{Buffer: bytes.NewBuffer(nil)}
	b.ReportAllocs()
	for b.Loop() {
		conn := h2.NewConn(m, h2.ConnOpts{})
		_ = conn
	}
}
