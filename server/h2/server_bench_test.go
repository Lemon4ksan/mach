// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h2

import (
	"io"
	"net"
	"strconv"
	"testing"

	"github.com/lemon4ksan/mach/proto/semantic/status"
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

	for b.Loop() {
		sc.bw.Reset(conn)

		if err := sc.writeResponse(1, res); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkServerConn_StreamTable_Contention benchmarks sharded stream table lookup, insertion, and deletion under high goroutine contention.
func BenchmarkServerConn_StreamTable_Contention(b *testing.B) {
	sc := NewServerConn(&discardConn{}, nil)
	defer sc.Release()

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		var id uint32 = 1
		for pb.Next() {
			streamID := (id & 0x7fffffff) | 1
			id += 2
			st := acquireServerStream(streamID, true, true, 65535, nil, nil)
			sc.putStream(streamID, st)
			_, _ = sc.getStream(streamID)
			sc.deleteStream(streamID)
			releaseServerStream(st)
		}
	})
}

// BenchmarkH2_MultiplexStreams benchmarks concurrent stream multiplexing throughput:
// concurrent stream creation, dispatching, handler execution, and serialized response writing.
func BenchmarkH2_MultiplexStreams(b *testing.B) {
	conn := &discardConn{}
	handler := func(req *ServerRequest, res *ServerResponse) error {
		res.StatusCode = status.OK
		res.Body = []byte("OK")
		return nil
	}

	sc := NewServerConn(conn, handler)
	defer sc.Release()

	for _, count := range []int{10, 100} {
		b.Run(strconv.Itoa(count)+"_Streams", func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()

			for b.Loop() {
				for i := 0; i < count; i++ {
					streamID := uint32(2*i + 1)
					st := sc.newServerStream(streamID, true, true)
					sc.putStream(streamID, st)
					sc.startStream(st)
				}
				sc.streamsWg.Wait()
			}
		})
	}
}
