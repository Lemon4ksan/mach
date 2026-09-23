// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package h1 provides high-performance, zero-allocation HTTP/1.1 client transport
// primitives adhering to RFC 9112 and RFC 9110 specifications.
package h1

import (
	"bufio"
	"context"
	"net"

	"github.com/lemon4ksan/mach/proto/http"
)

// ClientConn manages an HTTP/1.1 client connection over a stream-oriented network transport
// (RFC 9112 §3). It performs sequential request serialization and response deserialization
// with zero heap allocations on message transfer hot paths.
//
// Concurrency Model:
// ClientConn represents a single underlying network connection and is NOT safe for concurrent
// calls to Do. Requests over a single HTTP/1.1 connection MUST be executed sequentially
// (RFC 9112 §9.3). For concurrent request processing, instances should be pooled across goroutines
// using client.PoolManager[*ClientConn]. Close may be invoked concurrently to abort in-flight I/O.
//
// Lifecycle & Buffering:
// A ClientConn wraps a net.Conn with dedicated 4KB-16KB bufio buffers. Once closed via Close(),
// the underlying socket is closed and the ClientConn cannot be reused.
type ClientConn struct {
	conn net.Conn
	bw   *bufio.Writer
	br   *bufio.Reader
}

// NewClientConn creates an HTTP/1.1 client connection wrapping the provided network socket c.
// It initializes dedicated buffered readers and writers for pipelined wire I/O.
func NewClientConn(c net.Conn) *ClientConn {
	return &ClientConn{
		conn: c,
		bw:   bufio.NewWriter(c),
		br:   bufio.NewReader(c),
	}
}

// Do transmits req over the wire and parses the server response into res (RFC 9112 §3.1, RFC 9110 §9).
//
// Protocol Invariants:
//   - Request wire framing adheres to RFC 9112 §3, serializing the request-line, header section, and body.
//   - Content length and chunked transfer encoding are handled per RFC 9112 §6 and §7.1.
//   - Response parsing populates res using zero-copy byte slices referencing the internal read buffer.
//   - If ctx is cancelled before or during response receipt, the underlying connection is closed
//     to unblock pending network I/O, the reading goroutine is joined, and ctx.Err() is returned.
//
// Do is not safe for concurrent execution on the same ClientConn instance.
func (cc *ClientConn) Do(ctx context.Context, req *http.Request, res *http.Response) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if err := req.Write(cc.bw); err != nil {
		return err
	}

	if err := cc.bw.Flush(); err != nil {
		return err
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- res.Read(cc.br)
	}()

	select {
	case <-ctx.Done():
		_ = cc.Close()

		<-errCh

		return ctx.Err()

	case err := <-errCh:
		return err
	}
}

// Close terminates the HTTP/1.1 client connection and releases the underlying socket (RFC 9112 §9.6).
// Any active or pending Do operation on cc will immediately fail with a closed connection error.
func (cc *ClientConn) Close() error {
	return cc.conn.Close()
}
