// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h2

import (
	"github.com/lemon4ksan/foundation/net/hpack"

	coreh2 "github.com/lemon4ksan/mach/proto/h2"
	h1 "github.com/lemon4ksan/mach/proto/http"
)

// handlePushPromise processes incoming server push promises (RFC 9113 §6.6 & §8.4).
func (c *Conn) handlePushPromise(pp *coreh2.PushPromise) error {
	if !c.current.Push() {
		return nil
	}

	promisedID := pp.PromisedStream()
	// RFC 9113 §5.1.1: Server-initiated streams MUST use even-numbered stream identifiers.
	if promisedID == 0 || (promisedID%2 != 0) {
		return coreh2.NewGoAwayError(coreh2.ProtocolError, "invalid promised stream id (RFC 9113 §5.1.1)")
	}

	pushReq := h1.AcquireRequest()
	if err := c.decodePushHeaders(pp.Headers(), pushReq); err != nil {
		h1.ReleaseRequest(pushReq)
		return err
	}

	method := string(pushReq.Header.Method())
	if method != "GET" && method != "HEAD" {
		h1.ReleaseRequest(pushReq)
		c.resetStream(promisedID, coreh2.StreamCanceled)

		return nil
	}

	pushResp := h1.AcquireResponse()
	errCh := make(chan error, 1)

	ctx := &Context{
		Request:  pushReq,
		Response: pushResp,
		Err:      errCh,
	}
	ctx.StreamID.Store(promisedID)

	ctx.SetState(streamOpen)
	c.storeStream(ctx)

	go c.awaitPushedResponse(ctx, pushReq, pushResp)

	return nil
}

func (c *Conn) decodePushHeaders(headerBlock []byte, pushReq *h1.Request) error {
	hf := hpack.AcquireHeaderField()
	defer hpack.ReleaseHeaderField(hf)

	b := headerBlock
	for len(b) > 0 {
		var err error

		b, err = c.dec.Next(hf, b)
		if err != nil {
			return err
		}

		key := hf.Key()
		val := hf.Value()

		switch key {
		case ":method":
			pushReq.Header.SetMethod(val)
		case ":authority":
			pushReq.Header.SetHost(val)
		case ":scheme":
			pushReq.URI().SetScheme(val)
		case ":path":
			pushReq.SetRequestURI(val)
		default:
			if !hf.IsPseudo() {
				pushReq.Header.Add(key, val)
			}
		}
	}

	return nil
}

func (c *Conn) awaitPushedResponse(ctx *Context, pushReq *h1.Request, pushResp *h1.Response) {
	err := <-ctx.Err
	if err == nil && c.onPushPromise != nil {
		c.onPushPromise(pushReq, pushResp)
	}

	h1.ReleaseRequest(pushReq)
	h1.ReleaseResponse(pushResp)
}

func (c *Conn) resetStream(streamID uint32, code coreh2.ErrorCode) {
	fr := coreh2.AcquireFrameHeader()
	fr.SetStream(streamID)

	rst := coreh2.AcquireFrame(coreh2.FrameResetStream).(*coreh2.RstStream)
	rst.SetCode(code)
	fr.SetBody(rst)

	select {
	case c.out <- fr:
	default:
		coreh2.ReleaseFrameHeader(fr)
	}
}
