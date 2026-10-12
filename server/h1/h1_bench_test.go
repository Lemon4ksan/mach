// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h1_test

import (
	"bufio"
	"bytes"
	"fmt"
	"net"
	"testing"

	"github.com/lemon4ksan/foundation/silicon/bytesconv"

	"github.com/lemon4ksan/mach/proto/semantic/status"
	"github.com/lemon4ksan/mach/server/h1"
)

func BenchmarkRequest_ReadRequest(b *testing.B) {
	raw := []byte("GET /hello HTTP/1.1\r\nHost: example.com\r\nUser-Agent: bench\r\n\r\n")
	rd := bytes.NewReader(raw)
	br := bufio.NewReaderSize(rd, 4096)

	bw := bytesconv.AcquireByteBuffer()
	defer bytesconv.ReleaseByteBuffer(bw)

	var req h1.Request

	b.ReportAllocs()

	for b.Loop() {
		req.Reset()
		rd.Reset(raw)
		br.Reset(rd)

		if err := req.ReadRequest(br, bw, 1024*1024); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkResponse_WriteTo(b *testing.B) {
	var res h1.Response

	res.StatusCode = status.OK
	res.Headers.Set("Content-Type", "text/plain")
	res.Body = []byte("hello world")

	bw := bytesconv.AcquireByteBuffer()
	defer bytesconv.ReleaseByteBuffer(bw)

	b.ReportAllocs()

	for b.Loop() {
		bw.Reset()

		if err := res.WriteTo(bw, true, false); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkConnHandler_ServeConn_Pipeline(b *testing.B) {
	reqData := []byte("GET /bench HTTP/1.1\r\nHost: example.com\r\n\r\n")

	okBody := []byte("ok")
	ch := &h1.ConnHandler{
		Handler: func(req *h1.Request, res *h1.Response) error {
			res.StatusCode = status.OK
			res.Body = okBody

			return nil
		},
	}

	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	go func() {
		_ = ch.ServeConn(serverConn)
	}()

	br := bufio.NewReader(clientConn)

	var respBuf [256]byte

	b.ReportAllocs()

	for b.Loop() {
		if _, err := clientConn.Write(reqData); err != nil {
			b.Fatal(err)
		}

		// Read until blank line (\r\n\r\n) + 2 bytes body
		n, err := br.Read(respBuf[:])
		if err != nil || n == 0 {
			b.Fatalf("read failed: %v", err)
		}
	}
}

func BenchmarkH1Pipeline_Throughput(b *testing.B) {
	singleReq := []byte("GET /bench HTTP/1.1\r\nHost: example.com\r\nUser-Agent: bench\r\n\r\n")
	okBody := []byte("ok")

	ch := &h1.ConnHandler{
		Handler: func(req *h1.Request, res *h1.Response) error {
			res.StatusCode = status.OK
			res.Body = okBody

			return nil
		},
	}

	for _, batchSize := range []int{16, 32, 64} {
		batchData := bytes.Repeat(singleReq, batchSize)
		b.Run(fmt.Sprintf("batch=%d", batchSize), func(b *testing.B) {
			var conn mockConn
			conn.Reader = bytes.NewReader(batchData)
			conn.writeBuf = make([]byte, 0, 64*1024)

			b.ReportAllocs()
			b.SetBytes(int64(len(batchData)))
			b.ResetTimer()

			for b.Loop() {
				conn.Reader.Reset(batchData)
				conn.writeBuf = conn.writeBuf[:0]

				if err := ch.ServeConn(&conn); err != nil {
					b.Fatal(err)
				}
			}

			elapsed := b.Elapsed().Seconds()
			totalReqs := float64(b.N * batchSize)
			b.ReportMetric(totalReqs/elapsed, "req/s")
		})
	}
}
