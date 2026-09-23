// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h2

import (
	"sync/atomic"
	"time"

	coreh2 "github.com/lemon4ksan/mach/proto/h2"
	h1 "github.com/lemon4ksan/mach/proto/http"
)

// DefaultPingInterval specifies the default 10-second period between keepalive PING frames (RFC 9113 §6.7).
const DefaultPingInterval = time.Second * 10

type streamState int32

const (
	streamIdle streamState = iota
	streamOpen
	streamHalfClosed
	streamClosed
)

// ClientOpts defines legacy connection configuration options retained for backward compatibility.
// Use ConnOpts in conn.go as the primary connection options type.
type ClientOpts struct {
	PingInterval  time.Duration
	OnRTT         func(time.Duration)
	OnPushPromise func(pushReq *h1.Request, pushResp *h1.Response)
	Settings      *coreh2.Settings
}

// Context encapsulates stream-level lifecycle state, request/response message envelopes,
// and stream flow-control windows for an active HTTP/2 exchange (RFC 9113 §5.1).
//
// Concurrency:
// StreamID and state are managed via atomic primitives to eliminate data races between
// request cancellation (CancelStream) and egress request writing (writeRequest).
type Context struct {
	Request        *h1.Request
	Response       *h1.Response
	Err            chan error
	Trailers       map[string][]string
	StreamID       atomic.Uint32
	streamWindow   atomic.Int32
	streamRxWindow atomic.Int32
	state          atomic.Int32
	headersParsed  bool
}

// ID returns the active stream identifier (RFC 9113 §5.1.1).
func (ctx *Context) ID() uint32 {
	return ctx.StreamID.Load()
}

// SetID sets the active stream identifier.
func (ctx *Context) SetID(id uint32) {
	ctx.StreamID.Store(id)
}

// State returns the current stream state (RFC 9113 §5.1).
func (ctx *Context) State() streamState {
	return streamState(ctx.state.Load())
}

// SetState updates the stream state.
func (ctx *Context) SetState(s streamState) {
	ctx.state.Store(int32(s))
}
