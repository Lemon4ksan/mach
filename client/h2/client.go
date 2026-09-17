// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// package h2 provides an HTTP/2 client multiplexer.
package h2

import (
	"container/list"
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lemon4ksan/mach/client/h1"
	coreh2 "github.com/lemon4ksan/mach/proto/h2"
)

const DefaultPingInterval = 15 * time.Second

type streamState int32

const (
	streamIdle streamState = iota
	streamOpen
	streamHalfClosed
	streamClosed
)

// ClientOpts configures the HTTP/2 client multiplexer.
type ClientOpts struct {
	PingInterval  time.Duration
	OnRTT         func(time.Duration)
	OnPushPromise func(pushReq *h1.Request, pushResp *h1.Response)
	Settings      *coreh2.Settings
}

// Context maps a fasthttp request/response pair to an asynchronous stream execution.
type Context struct {
	Request        *h1.Request
	Response       *h1.Response
	Err            chan error
	Trailers       map[string][]string
	StreamID       uint32
	streamWindow   atomic.Int32
	streamRxWindow atomic.Int32
	state          atomic.Int32
	headersParsed  bool
}

// State yields current lifecycle state of the HTTP/2 stream.
func (ctx *Context) State() streamState {
	return streamState(ctx.state.Load())
}

// SetState updates lifecycle state of the HTTP/2 stream.
func (ctx *Context) SetState(s streamState) {
	ctx.state.Store(int32(s))
}

// Client manages connection pooling and stream allocation for HTTP/2.
type Client struct {
	d             *Dialer
	onRTT         func(time.Duration)
	onPushPromise func(pushReq *h1.Request, pushResp *h1.Response)
	lck           sync.Mutex
	conns         list.List
	orderedKeys   []string
	settings      *coreh2.Settings
}

// NewClient constructs an HTTP/2 Client instance using dialer and options.
func NewClient(d *Dialer, opts ClientOpts) *Client {
	return &Client{
		d:             d,
		onRTT:         opts.OnRTT,
		onPushPromise: opts.OnPushPromise,
		settings:      opts.Settings,
	}
}

// SetOrderedHeaders configures custom HPACK header ordering.
func (cl *Client) SetOrderedHeaders(keys []string) {
	cl.orderedKeys = keys
}

func (cl *Client) onConnectionDropped(ctx context.Context, c *Conn) {
	cl.lck.Lock()
	defer cl.lck.Unlock()

	for e := cl.conns.Front(); e != nil; e = e.Next() {
		if e.Value.(*Conn) == c {
			cl.conns.Remove(e)

			if newConn, err := cl.createConn(ctx); err == nil && newConn != nil {
				cl.conns.PushFront(newConn)
			}

			break
		}
	}
}

func (cl *Client) createConn(ctx context.Context) (*Conn, error) {
	c, err := cl.d.DialContext(ctx, ConnOpts{
		PingInterval:  cl.d.PingInterval,
		OnDisconnect:  cl.onConnectionDropped,
		OnRTT:         cl.onRTT,
		OnPushPromise: cl.onPushPromise,
		Settings:      cl.settings,
	})
	if err != nil {
		return nil, err
	}

	if len(cl.orderedKeys) > 0 {
		c.SetOrderedHeaders(cl.orderedKeys)
	}

	return c, nil
}

// Do executes req over an available HTTP/2 stream, automatically retrying
// on a fresh connection if affected by a graceful GOAWAY frame.
//
// Postconditions:
//   - Retries transparently up to 3 times on new connections when GOAWAY is received.
func (cl *Client) Do(ctx context.Context, req *h1.Request, res *h1.Response) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	for range 3 {
		err := cl.doOnce(ctx, req, res)
		if errors.Is(err, coreh2.ErrGoAwayRetryable) {
			continue
		}

		return err
	}

	return coreh2.ErrGoAwayRetryable
}

// DoBatch executes a batch of requests concurrently over the multiplexed HTTP/2 connection.
func (cl *Client) DoBatch(ctx context.Context, reqs []*h1.Request, resps []*h1.Response) error {
	if len(reqs) == 0 {
		return nil
	}

	if len(reqs) != len(resps) {
		return errors.New("h2engine: length of reqs and resps must match")
	}

	conn, err := cl.selectConn(ctx)
	if err != nil {
		return err
	}

	type result struct {
		idx int
		err error
	}

	resCh := make(chan result, len(reqs))
	for i := range reqs {
		reqCtx := &Context{
			Request:  reqs[i],
			Response: resps[i],
			Err:      make(chan error, 1),
		}

		if err := conn.Write(reqCtx); err != nil {
			return coreh2.ErrGoAwayRetryable
		}

		go func(idx int, rCtx *Context) {
			select {
			case <-ctx.Done():
				conn.CancelStream(rCtx)

				resCh <- result{idx: idx, err: ctx.Err()}
			case err := <-rCtx.Err:
				resCh <- result{idx: idx, err: err}
			}
		}(i, reqCtx)
	}

	var firstErr error
	for range reqs {
		res := <-resCh
		if res.err != nil && firstErr == nil {
			firstErr = res.err
		}
	}

	return firstErr
}

// DoWithTrailers executes req over an available HTTP/2 stream and returns captured response trailers.
func (cl *Client) DoWithTrailers(
	ctx context.Context,
	req *h1.Request,
	res *h1.Response,
) (map[string][]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	for range 3 {
		trailers, err := cl.doOnceWithTrailers(ctx, req, res)
		if errors.Is(err, coreh2.ErrGoAwayRetryable) {
			continue
		}

		return trailers, err
	}

	return nil, coreh2.ErrGoAwayRetryable
}

func (cl *Client) doOnceWithTrailers(
	ctx context.Context,
	req *h1.Request,
	res *h1.Response,
) (map[string][]string, error) {
	conn, err := cl.selectConn(ctx)
	if err != nil {
		return nil, err
	}

	errCh := make(chan error, 1)
	reqCtx := &Context{
		Request:  req,
		Response: res,
		Err:      errCh,
	}

	if err := conn.Write(reqCtx); err != nil {
		return nil, coreh2.ErrGoAwayRetryable
	}

	select {
	case <-ctx.Done():
		conn.CancelStream(reqCtx)
		return nil, ctx.Err()

	case err := <-errCh:
		return reqCtx.Trailers, err
	}
}

func (cl *Client) doOnce(ctx context.Context, req *h1.Request, res *h1.Response) error {
	conn, err := cl.selectConn(ctx)
	if err != nil {
		return err
	}

	errCh := make(chan error, 1)
	reqCtx := &Context{
		Request:  req,
		Response: res,
		Err:      errCh,
	}

	if err := conn.Write(reqCtx); err != nil {
		return coreh2.ErrGoAwayRetryable
	}

	select {
	case <-ctx.Done():
		conn.CancelStream(reqCtx)
		return ctx.Err()

	case err := <-errCh:
		return err
	}
}

// selectConn selects an available connection from pool with Late-Binding optimization.
func (cl *Client) selectConn(ctx context.Context) (*Conn, error) {
	cl.lck.Lock()
	defer cl.lck.Unlock()

	for {
		if conn := cl.findAvailableConnLocked(); conn != nil {
			return conn, nil
		}

		c, err := cl.dialOrWaitLateBindingLocked(ctx)
		if err != nil {
			return nil, err
		}

		if c != nil {
			return c, nil
		}
	}
}

func (cl *Client) findAvailableConnLocked() *Conn {
	var next *list.Element

	for e := cl.conns.Front(); e != nil; e = next {
		c := e.Value.(*Conn)
		next = e.Next()

		if c.Closed() {
			cl.conns.Remove(e)
			continue
		}

		if c.CanOpenStream() {
			return c
		}
	}

	return nil
}

func (cl *Client) dialOrWaitLateBindingLocked(ctx context.Context) (*Conn, error) {
	cl.lck.Unlock()
	c, err := cl.createConn(ctx)
	cl.lck.Lock()

	if err != nil {
		return nil, err
	}

	cl.conns.PushFront(c)

	if existing := cl.findAvailableConnLocked(); existing != nil && existing != c {
		return existing, nil
	}

	return c, nil
}
