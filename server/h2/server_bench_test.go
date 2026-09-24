// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h2

import (
	"io"
	"net"
	"testing"

	"github.com/lemon4ksan/mach/proto/http/status"
)

type discardConn struct {
	net.Conn
}

func (d *discardConn) Read(b []byte) (int, error)  { return 0, io.EOF }
func (d *discardConn) Write(b []byte) (int, error) { return len(b), nil }
func (d *discardConn) Close() error                { return nil }
func (d *discardConn) LocalAddr() net.Addr         { return &net.TCPAddr{} }
func (d *discardConn) RemoteAddr() net.Addr        { return &net.TCPAddr{} }

// BenchmarkServerConn_WriteResponse benchmarks HTTP/2 server response serialization and framing (RFC 9113 §6.1, §6.2).
func BenchmarkServerConn_WriteResponse(b *testing.B) {
	conn := &discardConn{}

	sc := NewServerConn(conn, nil)
	defer sc.Release()

	res := &ServerResponse{
		StatusCode: status.OK,
		Body:       []byte("Hello World"),
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		sc.bw.Reset(conn)

		if err := sc.writeResponse(1, res); err != nil {
			b.Fatal(err)
		}
	}
}
