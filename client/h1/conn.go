// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h1

import (
	"bufio"
	"context"
	"crypto/tls"
	"net"
	"time"

	"github.com/lemon4ksan/mach"
	"github.com/lemon4ksan/mach/proto/http"
)

var (
	_ mach.Engine = (*Engine)(nil)
	_ mach.Conn   = (*ClientConn)(nil)
	_ mach.Stream = (*Stream)(nil)
	_ mach.Frame  = (*Frame)(nil)
)

// Engine implements mach.Engine for HTTP/1.1 connections.
type Engine struct {
	Dialer    net.Dialer
	TLSConfig *tls.Config
}

// Dial establishes an HTTP/1.1 connection to addr and returns a mach.Conn.
func (e *Engine) Dial(ctx context.Context, addr string) (mach.Conn, error) {
	c, err := e.Dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}

	if e.TLSConfig != nil {
		tlsConn := tls.Client(c, e.TLSConfig)
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			_ = c.Close()
			return nil, err
		}

		c = tlsConn
	}

	return NewClientConn(c), nil
}

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
	conn   net.Conn
	bw     *bufio.Writer
	br     *bufio.Reader
	stream Stream
}

// NewClientConn creates an HTTP/1.1 client connection wrapping the provided network socket c.
// It initializes dedicated buffered readers and writers for pipelined wire I/O.
func NewClientConn(c net.Conn) *ClientConn {
	cc := &ClientConn{
		conn: c,
		bw:   bufio.NewWriter(c),
		br:   bufio.NewReader(c),
	}
	cc.stream.cc = cc

	return cc
}

// OpenStream opens a new stream on cc, satisfying mach.Conn (single-stream carrier for H1).
func (cc *ClientConn) OpenStream(ctx context.Context) (mach.Stream, error) {
	if cc.stream.cc == nil {
		cc.stream.cc = cc
	}

	cc.stream.ctx = ctx

	return &cc.stream, nil
}

// Stream represents an HTTP/1.1 client stream satisfying mach.Stream.
type Stream struct {
	cc    *ClientConn
	ctx   context.Context
	frame Frame
}

func (s *Stream) Read(p []byte) (int, error) {
	return s.cc.br.Read(p)
}

func (s *Stream) Write(p []byte) (int, error) {
	n, err := s.cc.bw.Write(p)
	if err != nil {
		return n, err
	}

	return n, s.cc.bw.Flush()
}

func (s *Stream) Close() error {
	return s.cc.bw.Flush()
}

func (s *Stream) ReadFrame() (mach.Frame, error) {
	line, err := s.cc.br.ReadSlice('\n')
	if err != nil {
		return nil, err
	}

	s.frame.frameType = mach.FrameData
	s.frame.payload = line

	return &s.frame, nil
}

// Frame implements mach.Frame for HTTP/1.1 wire frames.
type Frame struct {
	frameType uint8
	payload   []byte
}

func (f *Frame) Type() uint8 {
	return f.frameType
}

func (f *Frame) Payload() []byte {
	return f.payload
}

// Do transmits req over the wire and parses the server response into res (RFC 9112 §3.1, RFC 9110 §9).
//
// Protocol Invariants:
//   - Request wire framing adheres to RFC 9112 §3, serializing the request-line, header section, and body.
//   - Content length and chunked transfer encoding are handled per RFC 9112 §6 and §7.1.
//   - Response parsing populates res using zero-copy byte slices referencing the internal read buffer.
//   - If ctx is cancelled before or during response receipt, the underlying connection is closed
//     to unblock pending network I/O and ctx.Err() is returned without goroutine sprawl.
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

	var stop func() bool
	if ctx.Done() != nil {
		if dl, ok := ctx.Deadline(); ok {
			_ = cc.conn.SetDeadline(dl)
		}

		stop = context.AfterFunc(ctx, func() {
			_ = cc.conn.SetDeadline(time.Unix(1, 0))
			_ = cc.Close()
		})
	}

	err := res.Read(cc.br)

	if stop != nil {
		stop()

		_ = cc.conn.SetDeadline(time.Time{})
	}

	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			_ = cc.Close()
			return ctxErr
		}

		if dl, ok := ctx.Deadline(); ok && !time.Now().Before(dl) {
			_ = cc.Close()
			return context.DeadlineExceeded
		}

		return err
	}

	if ctxErr := ctx.Err(); ctxErr != nil {
		_ = cc.Close()
		return ctxErr
	}

	return nil
}

// Close terminates the HTTP/1.1 client connection and releases the underlying socket (RFC 9112 §9.6).
// Any active or pending Do operation on cc will immediately fail with a closed connection error.
func (cc *ClientConn) Close() error {
	return cc.conn.Close()
}
