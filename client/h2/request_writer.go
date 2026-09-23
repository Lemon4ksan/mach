// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h2

import (
	"bytes"
	"context"
	"time"

	coreh2 "github.com/lemon4ksan/mach/proto/h2"
	h1 "github.com/lemon4ksan/mach/proto/http"
)

func isExpectContinue(req *h1.Request) bool {
	expect := req.Header.Peek("Expect")
	return bytes.EqualFold(expect, []byte("100-continue"))
}

func (c *Conn) waitExpectContinue(ctx *Context) {
	t := time.NewTimer(1 * time.Second)
	defer t.Stop()

	select {
	case <-t.C:
		// ExpectContinueTimeout elapsed; proceed to transmit body payload
	case <-ctx.Err:
		// Early response arrived (100 Continue or 4xx/5xx error rejection)
	}
}

func (c *Conn) writeRequest(ctx *Context) error {
	if ctx.State() == streamClosed {
		return context.Canceled
	}

	if !c.CanOpenStream() {
		return coreh2.ErrNoAvailableStreams
	}

	// RFC 9113 §5.1.1: Streams initiated by a client MUST use odd-numbered stream identifiers (1, 3, 5, ...).
	id := c.nextID.Add(2) - 2
	if id >= (1<<31 - 1) {
		// RFC 9113 §5.1.1: Stream identifiers must be 31-bit unsigned integers.
		// When reaching 2^31-1, stream identifiers cannot be reused and connection must close.
		_ = c.Close()
		return coreh2.ErrStreamClosed
	}

	req := ctx.Request
	hasBody := len(req.Body()) != 0

	ctx.StreamID.Store(id)
	ctx.SetState(streamOpen)

	maxWin := c.serverS.MaxWindowSize()
	initWin := int32(65535)

	if maxWin > 0 && maxWin <= 0x7fffffff {
		initWin = int32(maxWin)
	}

	ctx.streamWindow.Store(initWin)
	ctx.streamRxWindow.Store(6291456)

	fr := coreh2.AcquireFrameHeader()
	defer coreh2.ReleaseFrameHeader(fr)

	fr.SetStream(id)

	h := coreh2.AcquireFrame(coreh2.FrameHeaders).(*coreh2.Headers)
	fr.SetBody(h)

	c.encodeRequestHeaders(h, req)

	h.SetPadding(false)
	h.SetEndStream(!hasBody)
	h.SetEndHeaders(true)

	if !hasBody {
		ctx.SetState(streamHalfClosed)
	}

	c.storeStream(ctx)

	c.writeMu.Lock()

	_, err := fr.WriteTo(c.bw)
	if err == nil && !hasBody {
		err = c.bw.Flush()
	}

	c.writeMu.Unlock()

	if err != nil {
		c.lastErr = err
		c.deleteStream(id)
		ctx.SetState(streamClosed)

		return err
	}

	if hasBody {
		if isExpectContinue(req) {
			c.waitExpectContinue(ctx)
		}

		if ctx.State() != streamClosed {
			err = c.writeData(fr, ctx, req.Body())
		}
	}

	if err == nil {
		c.openStreams.Add(1)
	} else {
		c.lastErr = err
		c.deleteStream(id)
		ctx.SetState(streamClosed)
	}

	return err
}

func (c *Conn) writeData(fh *coreh2.FrameHeader, ctx *Context, body []byte) error {
	data := coreh2.AcquireFrame(coreh2.FrameData).(*coreh2.Data)
	fh.SetBody(data)

	offset := 0
	bodyLen := len(body)

	for offset < bodyLen {
		if c.Closed() || ctx.State() == streamClosed {
			return coreh2.ErrStreamClosed
		}

		remaining := bodyLen - offset
		chunkSize := c.calculateChunkSize(ctx, remaining)

		if chunkSize <= 0 {
			c.waitForWindowUpdate(ctx, remaining)
			continue
		}

		end := offset + chunkSize
		data.SetEndStream(end == bodyLen)
		data.SetPadding(false)
		data.SetData(body[offset:end])

		c.writeMu.Lock()

		_, wErr := fh.WriteTo(c.bw)
		if wErr == nil {
			wErr = c.bw.Flush()
		}

		c.writeMu.Unlock()

		if wErr != nil {
			return wErr
		}

		c.serverWindow.Add(-int32(chunkSize))   //nolint:gosec // chunkSize bounded by H2 frame size <= 16MB
		ctx.streamWindow.Add(-int32(chunkSize)) //nolint:gosec // chunkSize bounded by H2 frame size <= 16MB

		offset = end
	}

	if bodyLen > 0 {
		ctx.State() // half-closed
	}

	return nil
}

func (c *Conn) finish(r *Context, stream uint32, err error) {
	c.openStreams.Add(-1)

	select {
	case r.Err <- err:
	default:
	}

	c.deleteStream(stream)
	close(r.Err)
}
