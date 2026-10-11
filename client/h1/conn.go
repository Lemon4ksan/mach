// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h1

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/url"
	"time"

	"github.com/lemon4ksan/mach"
	"github.com/lemon4ksan/mach/net/proxy"
	"github.com/lemon4ksan/mach/proto/http"
	"github.com/lemon4ksan/mach/proto/semantic/status"
)

var (
	_ mach.Engine = (*Engine)(nil)
	_ mach.Conn   = (*ClientConn)(nil)
	_ mach.Stream = (*Stream)(nil)
	_ mach.Frame  = (*Frame)(nil)

	// ErrConnClosed is returned when Do is called on a closed ClientConn.
	ErrConnClosed = errors.New("h1: client connection is closed")

	// ErrConnUpgraded is returned when Do is called on an upgraded ClientConn (RFC 9112 §9.3).
	ErrConnUpgraded = errors.New("h1: client connection has been upgraded (RFC 9112 §9.3)")
)

// Engine implements mach.Engine for HTTP/1.1 connections.
type Engine struct {
	Dialer      net.Dialer
	TLSConfig   *tls.Config
	ProxyDialer proxy.Dialer
}

// EngineOption configures an HTTP/1.1 Engine instance.
type EngineOption func(*Engine)

// WithProxy configures the Engine to route outbound connections through a proxy.
// The parameter p accepts a proxy.Dialer, *url.URL, proxy URL string, or client.ProxyOption.
func WithProxy(p any) EngineOption {
	return func(e *Engine) {
		switch v := p.(type) {
		case proxy.Dialer:
			e.ProxyDialer = v
		case *url.URL:
			d, err := proxy.FromURL(v, proxy.Direct)
			if err == nil {
				e.ProxyDialer = d
			}
		case string:
			u, err := proxy.Parse(v)
			if err == nil {
				d, err := proxy.FromURL(u, proxy.Direct)
				if err == nil {
					e.ProxyDialer = d
				}
			}

		case proxy.Option:
			cfg := &proxy.Config{Dialer: proxy.Direct}
			v(cfg)
			e.ProxyDialer = cfg.Dialer
		}
	}
}

// WithTLSConfig configures TLS on the Engine.
func WithTLSConfig(conf *tls.Config) EngineOption {
	return func(e *Engine) {
		e.TLSConfig = conf
	}
}

// NewEngine creates an HTTP/1.1 Engine configured with the provided options.
// Accepts EngineOption (e.g. h1.WithProxy, h1.WithTLSConfig) or proxy.Option (e.g. proxy.WithProxy).
func NewEngine(opts ...any) *Engine {
	e := &Engine{}
	for _, opt := range opts {
		switch v := opt.(type) {
		case EngineOption:
			if v != nil {
				v(e)
			}

		case proxy.Option:
			if v != nil {
				cfg := &proxy.Config{Dialer: proxy.Direct}
				v(cfg)
				e.ProxyDialer = cfg.Dialer
			}

		case func(*Engine):
			if v != nil {
				v(e)
			}
		}
	}

	return e
}

// Dial establishes an HTTP/1.1 connection to addr and returns a mach.Conn.
func (e *Engine) Dial(ctx context.Context, addr string) (mach.Conn, error) {
	var (
		c   net.Conn
		err error
	)

	if e.ProxyDialer != nil {
		if cd, ok := e.ProxyDialer.(proxy.ContextDialer); ok {
			c, err = cd.DialContext(ctx, "tcp", addr)
		} else {
			c, err = e.ProxyDialer.Dial("tcp", addr)
		}
	} else {
		c, err = e.Dialer.DialContext(ctx, "tcp", addr)
	}

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
	conn       net.Conn
	bw         *bufio.Writer
	br         *bufio.Reader
	stream     Stream
	isClosed   bool
	isUpgraded bool
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
	if cc.isClosed {
		return ErrConnClosed
	}

	if cc.isUpgraded {
		return ErrConnUpgraded
	}

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

	// RFC 9112 §9.3: If response signaled Connection: close, close and mark connection non-reusable
	if res.Header.ConnectionClose() {
		cc.isClosed = true
		_ = cc.conn.Close()
	}

	// RFC 9112 §9.3: 101 Switching Protocols upgrades connection away from HTTP/1.1
	if res.StatusCode() == status.SwitchingProtocols {
		cc.isUpgraded = true
	}

	return nil
}

// Close terminates the HTTP/1.1 client connection and releases the underlying socket (RFC 9112 §9.6).
// Any active or pending Do operation on cc will immediately fail with a closed connection error.
func (cc *ClientConn) Close() error {
	cc.isClosed = true
	return cc.conn.Close()
}

// IsClosed reports whether the client connection has closed or is non-reusable.
// If the connection is idle, it performs a non-destructive probe to detect peer EOF (RFC 9112 §9.3).
func (cc *ClientConn) IsClosed() bool {
	if cc.isClosed {
		return true
	}

	if cc.conn != nil && cc.br != nil && cc.br.Buffered() == 0 {
		_ = cc.conn.SetReadDeadline(time.Now())
		_, err := cc.br.Peek(1)
		_ = cc.conn.SetReadDeadline(time.Time{})
		if err == io.EOF || errors.Is(err, net.ErrClosed) {
			cc.isClosed = true
			_ = cc.conn.Close()

			return true
		}
	}

	return false
}
