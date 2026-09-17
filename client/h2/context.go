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

const DefaultPingInterval = time.Second * 10

type streamState int32

const (
	streamIdle streamState = iota
	streamOpen
	streamHalfClosed
	streamClosed
)

type ClientOpts struct {
	PingInterval  time.Duration
	OnRTT         func(time.Duration)
	OnPushPromise func(pushReq *h1.Request, pushResp *h1.Response)
	Settings      *coreh2.Settings
}

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

func (ctx *Context) State() streamState {
	return streamState(ctx.state.Load())
}

func (ctx *Context) SetState(s streamState) {
	ctx.state.Store(int32(s))
}
