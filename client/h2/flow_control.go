// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h2

import (
	"time"

	coreh2 "github.com/lemon4ksan/mach/proto/h2"
)

// waitForWindowUpdate blocks the writing goroutine until flow control window capacity expands
// or until the stream/connection terminates.
//
// Preconditions:
//   - Evaluates calculateChunkSize inside c.windowMu lock to prevent lost wake-up signal races.
func (c *Conn) waitForWindowUpdate(ctx *Context, remaining int) {
	c.windowMu.Lock()
	defer c.windowMu.Unlock()

	if c.Closed() || ctx.State() == streamClosed {
		return
	}

	if c.calculateChunkSize(ctx, remaining) > 0 {
		return
	}

	c.windowCond.Wait()
}

func (c *Conn) broadcastWindowUpdate() {
	c.windowMu.Lock()
	c.windowCond.Broadcast()
	c.windowMu.Unlock()
}

func (c *Conn) calculateChunkSize(ctx *Context, remaining int) int {
	maxFrame := int(c.serverS.MaxFrameSize())
	if maxFrame <= 0 {
		maxFrame = 16384
	}

	serverWin := c.serverWindow.Load()
	streamWin := ctx.streamWindow.Load()

	win := min(int(streamWin), int(serverWin))

	if win <= 0 {
		return 0
	}

	chunk := min(remaining, win)

	return min(chunk, maxFrame)
}

func (c *Conn) handleWindowUpdate(fr *coreh2.FrameHeader) error {
	wu := fr.Body().(*coreh2.WindowUpdate)

	inc := int32(wu.Increment()) //nolint:gosec
	if inc <= 0 {
		return coreh2.ErrInvalidWindowIncrement
	}

	streamID := fr.Stream()

	var err error
	if streamID == 0 {
		err = c.updateServerWindow(inc)
	} else {
		err = c.updateStreamWindow(streamID, inc)
	}

	if err == nil {
		c.broadcastWindowUpdate()
	}

	return err
}

func (c *Conn) updateServerWindow(inc int32) error {
	for {
		old := c.serverWindow.Load()
		if int64(old)+int64(inc) > int64(1<<31-1) {
			return coreh2.ErrWindowAboveLimits
		}

		if c.serverWindow.CompareAndSwap(old, old+inc) {
			return nil
		}
	}
}

func (c *Conn) updateStreamWindow(streamID uint32, inc int32) error {
	reqCtx := c.getStream(streamID)

	if reqCtx == nil {
		return nil
	}

	for {
		old := reqCtx.streamWindow.Load()
		if int64(old)+int64(inc) > int64(1<<31-1) {
			return coreh2.ErrWindowAboveLimits
		}

		if reqCtx.streamWindow.CompareAndSwap(old, old+inc) {
			return nil
		}
	}
}

func (c *Conn) updateWindow(streamID uint32, size int) {
	if size <= 0 || c.Closed() {
		return
	}

	fr := coreh2.AcquireFrameHeader()
	fr.SetStream(streamID)

	wu := coreh2.AcquireFrame(coreh2.FrameWindowUpdate).(*coreh2.WindowUpdate)
	wu.SetIncrement(size)
	fr.SetBody(wu)

	select {
	case c.out <- fr:
	case <-time.After(1 * time.Second):
		coreh2.ReleaseFrameHeader(fr)
	}
}
