// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package client

import (
	"context"
	"sync/atomic"

	"github.com/lemon4ksan/mach"
	h1client "github.com/lemon4ksan/mach/client/h1"
	h2client "github.com/lemon4ksan/mach/client/h2"
	h3client "github.com/lemon4ksan/mach/client/h3"
	"github.com/lemon4ksan/mach/proto/http"
)

// Conn represents an intelligent multi-protocol client connection (HTTP/1.1, HTTP/2, or HTTP/3).
// It satisfies mach.Conn and provides protocol introspection, health checks, and request dispatch.
type Conn interface {
	mach.Conn

	// Do executes an HTTP request/response exchange over the connection.
	Do(ctx context.Context, req *http.Request, res *http.Response) error

	// Protocol yields the negotiated protocol identifier ("http/1.1", "h2", "h3").
	Protocol() string

	// IsClosed reports whether the connection or multiplexer has closed.
	IsClosed() bool

	// CloseRaw terminates the underlying physical socket without returning to pool.
	CloseRaw() error
}

type h1Conn struct {
	cc   *h1client.ClientConn
	addr string
}

func newH1Conn(cc *h1client.ClientConn, addr string) *h1Conn {
	return &h1Conn{cc: cc, addr: addr}
}

func (c *h1Conn) OpenStream(ctx context.Context) (mach.Stream, error) {
	return c.cc.OpenStream(ctx)
}

func (c *h1Conn) Do(ctx context.Context, req *http.Request, res *http.Response) error {
	return c.cc.Do(ctx, req, res)
}

func (c *h1Conn) Protocol() string {
	return "http/1.1"
}

func (c *h1Conn) IsClosed() bool {
	return c.cc.IsClosed()
}

func (c *h1Conn) Close() error {
	return c.cc.Close()
}

func (c *h1Conn) CloseRaw() error {
	return c.cc.Close()
}

type h2Conn struct {
	c    *h2client.Conn
	addr string
}

func newH2Conn(c *h2client.Conn, addr string) *h2Conn {
	return &h2Conn{c: c, addr: addr}
}

func (c *h2Conn) OpenStream(ctx context.Context) (mach.Stream, error) {
	return c.c.OpenStream(ctx)
}

func (c *h2Conn) Do(ctx context.Context, req *http.Request, res *http.Response) error {
	return c.c.Do(ctx, req, res)
}

func (c *h2Conn) Protocol() string {
	return "h2"
}

func (c *h2Conn) IsClosed() bool {
	return c.c.Closed()
}

func (c *h2Conn) CanOpenStream() bool {
	return c.c.CanOpenStream()
}

func (c *h2Conn) Close() error {
	return c.c.Close()
}

func (c *h2Conn) CloseRaw() error {
	return c.c.Close()
}

type h3Conn struct {
	cc   *h3client.ClientConn
	addr string
}

func newH3Conn(cc *h3client.ClientConn, addr string) *h3Conn {
	return &h3Conn{cc: cc, addr: addr}
}

func (c *h3Conn) OpenStream(ctx context.Context) (mach.Stream, error) {
	return c.cc.OpenStream(ctx)
}

func (c *h3Conn) Do(ctx context.Context, req *http.Request, res *http.Response) error {
	_, err := c.cc.Do(ctx, req, res, nil)

	return err
}

func (c *h3Conn) Protocol() string {
	return "h3"
}

func (c *h3Conn) IsClosed() bool {
	return c.cc.IsClosed()
}

func (c *h3Conn) Close() error {
	return c.cc.Close()
}

func (c *h3Conn) CloseRaw() error {
	return c.cc.Close()
}

type pooledConn struct {
	Conn
	engine *Engine
	addr   string
	closed atomic.Bool
}

func newPooledConn(engine *Engine, addr string, conn Conn) *pooledConn {
	return &pooledConn{
		Conn:   conn,
		engine: engine,
		addr:   addr,
	}
}

func (p *pooledConn) Close() error {
	if !p.closed.CompareAndSwap(false, true) {
		return nil
	}

	if p.engine != nil {
		if !p.IsClosed() {
			if p.Protocol() == "http/1.1" {
				p.engine.PoolManager.Put(p.addr, p.Conn)

				return nil
			}

			return nil
		}

		p.engine.PoolManager.Discard(p.addr, p.Conn)

		return p.CloseRaw()
	}

	return p.CloseRaw()
}
